package torrential

import (
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	uuid "github.com/satori/go.uuid"
)

type Event struct {
	Type    EventType
	Torrent *torrent.Torrent
	File    *torrent.File
}

type EventType int

const (
	EventAdded EventType = iota
	EventGotInfo
	EventFileDone
	EventDownloadDone
	EventSeedingDone
	EventClosed
)

var _ Eventer = &TorrentsEventer{}
var _ Eventer = &TorrentEventer{}

type Eventer interface {
	Events(done <-chan struct{}) <-chan Event
}

type ClosedEventer interface {
	Eventer
	Closed() <-chan struct{}
}

type TorrentsEventer struct {
	eventerChans map[string]chan Eventer
	eventerMap   map[string]Eventer
	numActive    int
	mutex        sync.RWMutex
}

func NewTorrentsEventer() *TorrentsEventer {
	return &TorrentsEventer{
		eventerChans: make(map[string]chan Eventer),
		eventerMap:   make(map[string]Eventer),
	}
}

func (ae *TorrentsEventer) Events(done <-chan struct{}) <-chan Event {
	events := make(chan Event)
	eventerChan := make(chan Eventer)

	id := uuid.NewV4().String()
	ae.mutex.Lock()
	ae.eventerChans[id] = eventerChan
	ae.mutex.Unlock()

	go func() {
		ae.mutex.RLock()
		defer ae.mutex.RUnlock()
		for _, eventer := range ae.eventerMap {
			eventerChan <- eventer
		}
	}()
	go func() {
		defer func() {
			ae.mutex.Lock()
			delete(ae.eventerChans, id)
			ae.mutex.Unlock()
			close(events)
		}()
		for {
			select {
			case eventer, ok := <-eventerChan:
				if !ok {
					return
				}
				go func() {
					for event := range eventer.Events(done) {
						events <- event
					}
				}()
			case <-done:
				return
			}
		}
	}()
	return events
}

func (ae *TorrentsEventer) AddEventer(e ClosedEventer) {
	ae.mutex.RLock()
	defer ae.mutex.RUnlock()

	id := uuid.NewV4().String()
	ae.eventerMap[id] = e
	go func() {
		<-e.Closed()
		ae.mutex.Lock()
		delete(ae.eventerMap, id)
		ae.mutex.Unlock()
	}()
	for _, eventerChan := range ae.eventerChans {
		eventerChan <- e
	}
}

type TorrentEventer struct {
	seedRatio float64

	torrent *torrent.Torrent

	added        chan struct{}
	gotInfo      chan struct{}
	fileDone     map[string]chan struct{}
	downloadDone chan struct{}
	seedingDone  chan struct{}
	closed       chan struct{}

	fdMutex    sync.RWMutex
	filesReady chan struct{}
}

type TorrentEventerOptionFunc func(e *TorrentEventer)

func NewTorrentEventer(t *torrent.Torrent, options ...TorrentEventerOptionFunc) *TorrentEventer {
	e := TorrentEventer{
		torrent:      t,
		added:        make(chan struct{}),
		gotInfo:      make(chan struct{}),
		fileDone:     make(map[string]chan struct{}),
		downloadDone: make(chan struct{}),
		seedingDone:  make(chan struct{}),
		closed:       make(chan struct{}),

		filesReady: make(chan struct{}),
	}
	for _, opt := range options {
		opt(&e)
	}
	go e.run()

	return &e
}

// SetSeedRatio sets the monitored seed ratio for the torrent. If the channel
// returned by SeedingDone() has already been closed, this will have no effect.
func (e *TorrentEventer) SetSeedRatio(seedRatio float64) {
	e.seedRatio = seedRatio
}

// SeedRatio returns an OptionFunc that sets the given seed ratio when the
// TorrentEventer is initialized.
func SeedRatio(seedRatio float64) TorrentEventerOptionFunc {
	return func(e *TorrentEventer) {
		e.seedRatio = seedRatio
	}
}

// Added returns a channel that will be closed when the torrent is added.
func (e *TorrentEventer) Added() <-chan struct{} {
	return e.added
}

// GotInfo returns a channel that will be closed when the torrent info is ready.
func (e *TorrentEventer) GotInfo() <-chan struct{} {
	return e.gotInfo
}

// FileDone returns a channel that will be closed when the file at the given
// path has completed downloading.
func (e *TorrentEventer) FileDone(filePath string) (<-chan struct{}, bool) {
	e.fdMutex.RLock()
	defer e.fdMutex.RUnlock()
	c, ok := e.fileDone[filePath]
	return c, ok
}

// DownloadDone returns a channel that will be closed when the torrent download
// is complete.
func (e *TorrentEventer) DownloadDone() <-chan struct{} {
	return e.downloadDone
}

// SeedingDone returns a channel that will be closed when the torrent seeding
// is complete, based on the TorrentEventer's configured seed ratio. Changes to the
// seed ratio after the returned channel is closed will have no effect.
func (e *TorrentEventer) SeedingDone() <-chan struct{} {
	return e.seedingDone
}

// Closed returns a channel that will be closed when the torrent is closed.
func (e *TorrentEventer) Closed() <-chan struct{} {
	return e.closed
}

// Events returns a channel on which all of the events will be sent. The channel
// will be closed after the closed event is sent.
func (e *TorrentEventer) Events(done <-chan struct{}) <-chan Event {
	events := make(chan Event)

	go func() {
		defer func() {
			close(events)
		}()
		select {
		case <-e.Added():
			events <- Event{Type: EventAdded, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			return
		case <-done:
			return
		}

		select {
		case <-e.GotInfo():
			events <- Event{Type: EventGotInfo, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			return
		case <-done:
			return
		}

		// We're using a sync.WaitGroup to make sure all of the fileDone events
		// have been sent before we send the downloadDone event
		var wg sync.WaitGroup
		wg.Add(len(e.torrent.Files()))

		select {
		case <-e.filesReady:
			go func() {
				for _, file := range e.torrent.Files() {
					fileDone, _ := e.FileDone(file.Path())
					go func(f torrent.File) {
						defer wg.Done()
						select {
						case <-e.Closed():
							return
						case <-done:
							return
						case <-fileDone:
							events <- Event{Type: EventFileDone, Torrent: e.torrent, File: &f}
						}
					}(file)
				}
			}()
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			return
		case <-done:
			return
		}

		// Make sure all of the fileDone events have been sent before we send
		// the downloadDone event
		wg.Wait()

		select {
		case <-e.DownloadDone():
			events <- Event{Type: EventDownloadDone, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			return
		case <-done:
			return
		}

		select {
		case <-e.SeedingDone():
			events <- Event{Type: EventSeedingDone, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			return
		case <-done:
			return
		}

		select {
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
		case <-done:
			return
		}
	}()

	return events
}

func (e *TorrentEventer) run() {
	// Immediately subscribe to piece changes so we won't miss them while we
	// check file completion and setup state for incomplete files.
	sub := e.torrent.SubscribePieceStateChanges()
	defer sub.Close()

	// Closed the added channel immediately. The fact that we know about this
	// torrent means it has been added.
	close(e.added)

	// We need to wait for the torrent info to be ready so that we know the
	// files contained in the torrent. If the torrent gets closed before the
	// info is ready, return immediately without closing the other event
	// channels.
	select {
	case <-e.torrent.GotInfo():
		close(e.gotInfo)
	case <-e.torrent.Closed():
		close(e.closed)
		return
	}

	// For each file, create a set of incomplete pieces for the file
	// For each piece, create a set of incomplete files for the piece
	//
	// When we get a piece update, we'll use these maps to quickly check to see
	// if files have been completed.
	incompleteFilePieces := make(map[string]map[int]struct{})
	incompletePieceFiles := make(map[int]map[string]struct{})

	// For each file, setup a new fileDone channel and a new map to store a set
	// of incomplete pieces
	for _, file := range e.torrent.Files() {
		e.fdMutex.Lock()
		e.fileDone[file.Path()] = make(chan struct{})
		e.fdMutex.Unlock()

		incompleteFilePieces[file.Path()] = make(map[int]struct{})
	}

	// Now that all of the fileDone channels have been created, close the
	// filesReady channel to initialize the processing to send FileDone events
	// in the Events() function
	close(e.filesReady)

	// Iterate through each piece, setting up the incomplete file and piece maps
	fileIndex := 0
	for i := 0; i < e.torrent.NumPieces(); i++ {
		piece := e.torrent.Piece(i)
		pieceState := e.torrent.PieceState(i)

		if piece.Info().Length() == 0 || pieceState.Complete {
			continue
		}

		incompletePieceFiles[i] = make(map[string]struct{})

		pieceBegin := piece.Info().Offset()
		pieceEnd := pieceBegin + piece.Info().Length() - 1

		for j, file := range e.torrent.Files()[fileIndex:] {
			if file.Length() == 0 {
				continue
			}

			fileBegin := file.Offset()
			fileEnd := fileBegin + file.Length() - 1

			if pieceEnd >= fileBegin && fileEnd >= pieceBegin {
				// This file has bytes in the piece. Add it to the maps and
				// check the same file in the next piece
				incompletePieceFiles[i][file.Path()] = struct{}{}
				incompleteFilePieces[file.Path()][i] = struct{}{}
				fileIndex = j
			}
		}
	}

	// Check for files that are already complete
	for file, pieces := range incompleteFilePieces {
		// If the set of incomplete pieces for a file is empty, the file is
		// complete, so close the fileDone channel for the file and delete the
		// file from the filePieces set
		if len(pieces) == 0 {
			e.fdMutex.RLock()
			close(e.fileDone[file])
			e.fdMutex.RUnlock()

			delete(incompleteFilePieces, file)
			for p := range pieces {
				delete(incompletePieceFiles[p], file)
			}
		}
	}

	// If there are no bytes missing, it means all the files and the entire
	// torrent are done downloading, so short circuit the rest of the fileDone
	// and downloadDone processing.
	//
	// Otherwise, monitor the piece state changes for completed pieces.
	if e.torrent.BytesMissing() == 0 {
		for file, pieces := range incompleteFilePieces {
			e.fdMutex.RLock()
			close(e.fileDone[file])
			e.fdMutex.RUnlock()

			delete(incompleteFilePieces, file)
			for p := range pieces {
				delete(incompletePieceFiles[p], file)
			}
		}
		close(e.downloadDone)
	} else {
		for {
			piece, open := <-sub.Values
			if !open {
				// If sub.Values is closed, the torrent has been closed, so
				// close the e.closed channel and return
				close(e.closed)
				return
			}

			psc := piece.(torrent.PieceStateChange)

			// If the piece is complete:
			//   1. find the set of files it contains data for
			//   2. for each of those files, remove the piece from that file's
			//      set of incomplete pieces
			//   3. if the set of pieces is now empty, the file is down
			//      downloading, so close its fileDone channel and remove the
			//      file from both incomplete maps
			//   4. if no bytes are missing from the torrent, close the
			//      downloadDone channel and break the loop
			if psc.Complete {
				files := incompletePieceFiles[psc.Index]

				for f := range files {
					delete(incompleteFilePieces[f], psc.Index)
					if len(incompleteFilePieces[f]) == 0 {
						e.fdMutex.RLock()
						close(e.fileDone[f])
						e.fdMutex.RUnlock()

						delete(incompleteFilePieces, f)
						delete(incompletePieceFiles[psc.Index], f)
					}
				}

				if e.torrent.BytesMissing() == 0 {
					close(e.downloadDone)
					break
				}
			}
		}
	}

	// At this point, the torrent has completed downloading, so we switch to
	// monitoring the seed ratio.

	// If the seed ratio is 0 or the torrent is set to not seed, close the
	// seedingDone channel immediately.  Otherwise check the ratio periodically.
	if e.seedRatio <= 0.0 || !e.torrent.Seeding() {
		close(e.seedingDone)
	} else {
	seedRatioLoop:
		for {
			select {
			// If the torrent is closed before the seed ratio is met, close the
			// e.closed channel and return
			case <-e.torrent.Closed():
				close(e.closed)
				return
			case <-time.After(e.seedWait()):
				if float64(e.torrent.Stats().DataBytesWritten)/float64(e.torrent.BytesCompleted()) >= e.seedRatio {
					close(e.seedingDone)
					break seedRatioLoop
				}
			}
		}
	}

	// The only event left is the torrent close event. Wait for that and close
	// the e.closed channel before returning.
	<-e.torrent.Closed()
	close(e.closed)
	return
}

// seedWait returns a duration inversely propotional to the seed ratio itself,
// so the closer to the seed ratio we are, the shorter the wait duration.
func (e *TorrentEventer) seedWait() time.Duration {
	percentSeedRatio := float64(e.torrent.Stats().DataBytesWritten) / float64(e.torrent.Length()) / e.seedRatio
	if percentSeedRatio > 1 {
		return 0
	}
	return (time.Millisecond * time.Duration((1-percentSeedRatio)*1000.0) * 15) + time.Second
}
package torrential

import (
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"
)

type Eventer struct {
	seedRatio float64

	torrent      *atorrent.Torrent
	added        chan struct{}
	gotInfo      <-chan struct{}
	fileDone     map[string]chan struct{}
	downloadDone chan struct{}
	seedingDone  chan struct{}
	closed       <-chan struct{}

	fdMutex    sync.RWMutex
	filesReady chan struct{}
}

type EventerOptionFunc func(e *Eventer)

func NewEventer(t *atorrent.Torrent, options ...EventerOptionFunc) *Eventer {
	e := Eventer{
		torrent:      t,
		added:        make(chan struct{}),
		gotInfo:      t.GotInfo(),
		fileDone:     make(map[string]chan struct{}),
		downloadDone: make(chan struct{}),
		seedingDone:  make(chan struct{}),
		closed:       t.Closed(),

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
func (e *Eventer) SetSeedRatio(seedRatio float64) {
	e.seedRatio = seedRatio
}

// SeedRatio returns an OptionFunc that sets the given seed ratio when the
// Eventer is initialized.
func SeedRatio(seedRatio float64) EventerOptionFunc {
	return func(e *Eventer) {
		e.seedRatio = seedRatio
	}
}

// Added returns a channel that will be closed when the torrent is added.
func (e *Eventer) Added() <-chan struct{} {
	return e.added
}

// GotInfo returns a channel that will be closed when the torrent info is ready.
func (e *Eventer) GotInfo() <-chan struct{} {
	return e.gotInfo
}

// FileDone returns a channel that will be closed when the file at the given
// path has completed downloading.
func (e *Eventer) FileDone(filePath string) (chan struct{}, bool) {
	e.fdMutex.RLock()
	defer e.fdMutex.RUnlock()
	c, ok := e.fileDone[filePath]
	return c, ok
}

// DownloadDone returns a channel that will be closed when the torrent download
// is complete.
func (e *Eventer) DownloadDone() chan struct{} {
	return e.downloadDone
}

// SeedingDone returns a channel that will be closed when the torrent seeding
// is complete, based on the Eventer's configured seed ratio. Changes to the
// seed ratio after the returned channel is closed will have no effect.
func (e *Eventer) SeedingDone() chan struct{} {
	return e.seedingDone
}

// Closed returns a channel that will be closed when the torrent is closed.
func (e *Eventer) Closed() <-chan struct{} {
	return e.closed
}

// Events returns a channel on which all of the events will be sent. The channel
// will be closed after the closed event is sent.
func (e *Eventer) Events() <-chan Event {
	events := make(chan Event)

	go func() {

		select {
		case <-e.Added():
			events <- Event{Type: EventAdded, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			close(events)
			return
		}

		select {
		case <-e.GotInfo():
			events <- Event{Type: EventGotInfo, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			close(events)
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
					go func(f atorrent.File) {
						defer wg.Done()
						select {
						case <-e.torrent.Closed():
							return
						case <-fileDone:
							events <- Event{Type: EventFileDone, Torrent: e.torrent, File: &f}
						}
					}(file)
				}
			}()
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			close(events)
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
			close(events)
			return
		}

		select {
		case <-e.SeedingDone():
			events <- Event{Type: EventSeedingDone, Torrent: e.torrent}
		case <-e.Closed():
			events <- Event{Type: EventClosed, Torrent: e.torrent}
			close(events)
			return
		}

		<-e.Closed()
		events <- Event{Type: EventClosed, Torrent: e.torrent}
		close(events)
		return

	}()

	return events
}

func (e *Eventer) run() {
	// Closed the added channel immediately. The fact that we know about this
	// torrent means it has been added.
	close(e.added)

	// We need to wait for the torrent info to be ready so that we know the
	// files contained in the torrent. If the torrent gets closed before the
	// info is ready, return immediately without closing the other event
	// channels.
	select {
	case <-e.torrent.GotInfo():
	case <-e.torrent.Closed():
		return
	}

	// Immediately subscribe to piece changes to we miss as few as possible
	// while we check file completion and setup state for incomplete files.
	sub := e.torrent.SubscribePieceStateChanges()
	defer sub.Close()

	// For each file, setup a map of incomplete pieces. Once a file map of
	// incomplete pieces is empty, we'll know the file is complete. We also
	// create our fileDone channel for each file.
	incompleteFiles := make(map[string]map[int]struct{})
	for _, f := range e.torrent.Files() {
		fileDone := make(chan struct{})
		complete := true

		incompleteFiles[f.DisplayPath()] = make(map[int]struct{})
		for i, piece := range f.State() {
			if !piece.Complete {
				complete = false
				incompleteFiles[f.DisplayPath()][int(f.Offset())+i] = struct{}{}
			}
		}

		if complete {
			delete(incompleteFiles, f.DisplayPath())
			close(fileDone)
		}

		e.fdMutex.Lock()
		e.fileDone[f.Path()] = fileDone
		e.fdMutex.Unlock()
	}

	close(e.filesReady)

	if e.torrent.BytesMissing() == 0 {
		close(e.downloadDone)
	} else {
		for {
			piece, open := <-sub.Values
			if !open {
				// If sub.Values is closed, the torrent
				// has been closed, so just return
				return
			}

			psc := piece.(atorrent.PieceStateChange)

			// If the piece is complete:
			//   find the file it's in
			//   remove the piece from the file's incomplete piece map
			//   close the fileDone channel if the incomplete piece map is empty
			//   close the downloadDone channel if no more bytes are missing
			if psc.Complete {
				var file *atorrent.File
				for _, f := range e.torrent.Files() {
					i := int64(psc.Index)
					if i >= f.Offset() && i < f.Offset()+f.Length() {
						file = &f
					}
				}
				if file != nil {
					delete(incompleteFiles[file.Path()], psc.Index)
					if len(incompleteFiles[file.Path()]) == 0 {
						delete(incompleteFiles, file.Path())

						e.fdMutex.RLock()
						close(e.fileDone[file.Path()])
						e.fdMutex.RUnlock()
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
		for {
			select {
			// If the torrent is closed before the seed ratio is met, return
			case <-e.torrent.Closed():
				return
			case <-time.After(e.seedWait()):
				if float64(e.torrent.Stats().DataBytesWritten)/float64(e.torrent.BytesCompleted()) >= e.seedRatio {
					close(e.seedingDone)
					return
				}
			}
		}
	}
}

// seedWait returns a duration inversely propotional to the seed ratio itself,
// so the closer to the seed ratio we are, the shorter the wait duration.
func (e *Eventer) seedWait() time.Duration {
	percentSeedRatio := float64(e.torrent.Stats().DataBytesWritten) / float64(e.torrent.Length()) / e.seedRatio
	if percentSeedRatio > 1 {
		return 0
	}
	return (time.Millisecond * time.Duration((1-percentSeedRatio)*1000.0) * 15) + time.Second
}

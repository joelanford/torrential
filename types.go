package torrential

import (
	"encoding/json"

	"github.com/anacrolix/torrent"
)

type Torrent struct {
	*torrent.Torrent
}

func (t Torrent) MarshalJSON() ([]byte, error) {
	if t.Torrent == nil {
		return []byte("null"), nil
	}
	mi := t.Metainfo()
	torrent := struct {
		BytesCompleted int    `json:"bytesCompleted"` // Number of bytes completed
		BytesMissing   int    `json:"bytesMissing"`   // Number of bytes missing
		Files          []File `json:"files"`          // Files contained in the torrent
		InfoHash       string `json:"infoHash"`       // Torrent info hash
		Length         int    `json:"length"`         // Total number of bytes in torrent
		MagnetLink     string `json:"magnetLink"`     // Torrent magnet link
		Name           string `json:"name"`           // Torrent name
		NumPieces      int    `json:"numPieces"`      // Total number of pieces in torrent
		Seeding        bool   `json:"seeding"`        // Whether torrent is currently seeding
		Stats          stats  `json:"stats"`          // Torrent stats
		HasInfo        bool   `json:"hasInfo"`        // Whether the torrent info has been received)
	}{
		BytesCompleted: int(t.BytesCompleted()),
		BytesMissing:   0,
		Files:          make([]File, 0),
		InfoHash:       t.InfoHash().String(),
		Length:         0,
		MagnetLink:     mi.Magnet(t.Name(), t.InfoHash()).String(),
		Name:           t.Name(),
		NumPieces:      0,
		Seeding:        t.Seeding(),
		Stats:          stats{},
		HasInfo:        false,
	}
	select {
	case <-t.GotInfo():
		torrent.BytesMissing = int(t.BytesMissing())
		torrent.Length = int(t.Length())
		torrent.NumPieces = t.NumPieces()
		torrent.HasInfo = true

		s := t.Stats()
		torrent.Stats = stats{
			BytesRead:        int(s.BytesRead),
			BytesWritten:     int(s.BytesWritten),
			ChunksRead:       int(s.ChunksRead),
			ChunksWritten:    int(s.ChunksWritten),
			DataBytesRead:    int(s.DataBytesRead),
			DataBytesWritten: int(s.DataBytesWritten),
			TotalPeers:       s.TotalPeers,
			ActivePeers:      s.ActivePeers,
			PendingPeers:     s.PendingPeers,
			HalfOpenPeers:    s.HalfOpenPeers,
		}
		files := t.Files()
		for i := range files {
			torrent.Files = append(torrent.Files, File{&files[i]})
		}

	default:
	}
	return json.Marshal(torrent)
}

type File struct {
	*torrent.File
}

func (f File) MarshalJSON() ([]byte, error) {
	if f.File == nil {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		DisplayPath string `json:"displayPath"`
		Length      int    `json:"length"`
		Offset      int    `json:"offset"`
		Path        string `json:"path"`
	}{
		Path:        f.Path(),
		DisplayPath: f.DisplayPath(),
		Length:      int(f.Length()),
		Offset:      int(f.Offset()),
	})
}

type stats struct {
	ActivePeers      int `json:"activePeers"`
	BytesRead        int `json:"bytesRead"`
	BytesWritten     int `json:"bytesWritten"`
	ChunksRead       int `json:"chunksRead"`
	ChunksWritten    int `json:"chunksWritten"`
	DataBytesRead    int `json:"dataBytesRead"`
	DataBytesWritten int `json:"dataBytesWritten"`
	HalfOpenPeers    int `json:"halfOpenPeers"`
	PendingPeers     int `json:"pendingPeers"`
	TotalPeers       int `json:"totalPeers"`
}

type Eventer interface {
	Events(done <-chan struct{}) <-chan Event
}

type Event struct {
	Type    EventType `json:"type"`
	Torrent Torrent   `json:"torrent"`
	File    *File     `json:"file,omitempty"`
	Piece   *int      `json:"piece,omitempty"`
}

type EventType int

const (
	Added EventType = iota
	GotInfo
	PieceDone
	FileDone
	DownloadDone
	SeedingDone
	Closed
)

func (t EventType) String() string {
	switch t {
	case Added:
		return "added"
	case GotInfo:
		return "gotInfo"
	case PieceDone:
		return "pieceDone"
	case FileDone:
		return "fileDone"
	case DownloadDone:
		return "downloadDone"
	case SeedingDone:
		return "seedingDone"
	case Closed:
		return "closed"
	default:
		return "unknown"
	}
}

func (t EventType) MarshalJSON() (data []byte, err error) {
	return []byte("\"" + t.String() + "\""), nil
}

type torrentResult struct {
	Torrent *Torrent `json:"torrent"`
}

type torrentsResult struct {
	Torrents []Torrent `json:"torrents"`
}

type eventResult struct {
	Event Event `json:"event"`
}

type errorResult struct {
	Error string `json:"error"`
}

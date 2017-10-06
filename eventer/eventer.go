package eventer

import (
	"github.com/anacrolix/torrent"
)

type Eventer interface {
	Events(done <-chan struct{}) <-chan Event
}

type Event struct {
	Type    EventType
	Torrent *torrent.Torrent
	File    *torrent.File
}

type EventType int

const (
	Added EventType = iota
	GotInfo
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

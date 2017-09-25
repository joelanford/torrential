package torrential

import atorrent "github.com/anacrolix/torrent"

type Event struct {
	Type    EventType
	Torrent *atorrent.Torrent
	File    *atorrent.File
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

package convert

import (
	"github.com/joelanford/torrential/eventer"
	"github.com/joelanford/torrential/internal/torrential"
)

func Event(e eventer.Event) torrential.Event {
	var file *torrential.File
	if e.File != nil {
		file = &torrential.File{}
		*file = File(*e.File)
	}
	return torrential.Event{
		Type:    e.Type.String(),
		Torrent: Torrent(e.Torrent),
		File:    file,
	}
}

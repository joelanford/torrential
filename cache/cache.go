package cache

import (
	"github.com/anacrolix/torrent"
)

type Cache interface {
	SaveTorrent(*torrent.Torrent) error
	LoadTorrents() ([]torrent.TorrentSpec, error)
	DeleteTorrent(*torrent.Torrent) error
}

package convert

import (
	"io"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/joelanford/torrential/internal/torrential"
)

func ReaderToTorrentSpec(r io.Reader) (*torrent.TorrentSpec, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return nil, err
	}
	return torrent.TorrentSpecFromMetaInfo(mi), nil
}

func Torrent(t *torrent.Torrent) torrential.Torrent {
	metainfo := t.Metainfo()
	res := torrential.Torrent{
		BytesCompleted: int(t.BytesCompleted()),
		BytesMissing:   0,
		Files:          Files(t.Files()),
		InfoHash:       t.InfoHash().String(),
		Length:         0,
		MagnetLink:     metainfo.Magnet(t.Name(), t.InfoHash()).String(),
		Name:           t.Name(),
		NumPieces:      0,
		Seeding:        t.Seeding(),
		Stats:          Stats(t.Stats()),
		HasInfo:        false,
	}
	select {
	case <-t.GotInfo():
		res.BytesMissing = int(t.BytesMissing())
		res.Length = int(t.Length())
		res.NumPieces = t.NumPieces()
		res.HasInfo = true
	default:
	}
	return res
}

func File(f torrent.File) torrential.File {
	return torrential.File{
		Path:        f.Path(),
		DisplayPath: f.DisplayPath(),
		Length:      int(f.Length()),
		Offset:      int(f.Offset()),
	}
}

func Files(v []torrent.File) []torrential.File {
	res := make([]torrential.File, 0)
	for _, f := range v {
		res = append(res, File(f))
	}
	return res
}

func Stats(v torrent.TorrentStats) torrential.Stats {
	return torrential.Stats{
		BytesRead:        int(v.BytesRead),
		BytesWritten:     int(v.BytesWritten),
		ChunksRead:       int(v.ChunksRead),
		ChunksWritten:    int(v.ChunksWritten),
		DataBytesRead:    int(v.DataBytesRead),
		DataBytesWritten: int(v.DataBytesWritten),
		TotalPeers:       v.TotalPeers,
		ActivePeers:      v.ActivePeers,
		PendingPeers:     v.PendingPeers,
		HalfOpenPeers:    v.HalfOpenPeers,
	}
}

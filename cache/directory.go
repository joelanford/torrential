package cache

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/pkg/errors"
)

type Directory struct {
	Directory string
}

func NewDirectory(dir string) *Directory {
	return &Directory{
		Directory: dir,
	}
}

func (c *Directory) SaveTorrent(t *torrent.Torrent) error {
	select {
	case <-t.GotInfo():
		filename := filepath.Join(c.Directory, fmt.Sprintf("%s.torrent", t.InfoHash().HexString()))
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0660)
		if err != nil {
			return err
		}
		defer f.Close()
		return t.Metainfo().Write(f)
	case <-t.Closed():
		return errors.New("torrent closed before info ready")
	}
}

func (c *Directory) LoadTorrents() ([]torrent.TorrentSpec, error) {
	err := os.MkdirAll(c.Directory, 0750)
	if err != nil {
		return nil, err
	}

	entries, err := ioutil.ReadDir(c.Directory)
	if err != nil {
		return nil, err
	}
	var specs []torrent.TorrentSpec
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".torrent") && !e.IsDir() {
			f, err := os.Open(filepath.Join(c.Directory, e.Name()))
			if err != nil {
				return nil, err
			}
			defer f.Close()

			mi, err := metainfo.Load(f)
			if err != nil {
				return nil, err
			}
			spec := torrent.TorrentSpecFromMetaInfo(mi)
			specs = append(specs, *spec)
		}
	}
	return specs, nil
}
func (c *Directory) DeleteTorrent(t *torrent.Torrent) error {
	filename := filepath.Join(c.Directory, fmt.Sprintf("%s.torrent", t.InfoHash().HexString()))
	return os.Remove(filename)
}

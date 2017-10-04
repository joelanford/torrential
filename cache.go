package torrential

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/pkg/errors"
)

type Cache interface {
	SaveTorrent(*torrent.Torrent) error
	LoadTorrents() ([]torrent.TorrentSpec, error)
	DeleteTorrent(*torrent.Torrent) error
}

type FileCache struct {
	Directory string
}

func NewFileCache(dir string) *FileCache {
	return &FileCache{
		Directory: dir,
	}
}

func (c *FileCache) SaveTorrent(t *torrent.Torrent) error {
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

func (c *FileCache) LoadTorrents() ([]torrent.TorrentSpec, error) {
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
			spec, err := specFromTorrentReader(f)
			if err != nil {
				return nil, err
			}
			specs = append(specs, *spec)
		}
	}
	return specs, nil
}
func (c *FileCache) DeleteTorrent(t *torrent.Torrent) error {
	filename := filepath.Join(c.Directory, fmt.Sprintf("%s.torrent", t.InfoHash().HexString()))
	return os.Remove(filename)
}

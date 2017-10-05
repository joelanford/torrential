package torrential

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent"
	minio "github.com/minio/minio-go"
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

type MinioCache struct {
	Client *minio.Client
	Region string
	Bucket string
}

func NewMinioCache(client *minio.Client, bucket string) *MinioCache {
	return &MinioCache{
		Client: client,
		Region: "",
		Bucket: bucket,
	}
}

func NewMinioCacheWithRegion(client *minio.Client, bucket, region string) *MinioCache {
	return &MinioCache{
		Client: client,
		Region: region,
		Bucket: bucket,
	}
}

func (c *MinioCache) SaveTorrent(t *torrent.Torrent) error {
	select {
	case <-t.GotInfo():
		exists, err := c.Client.BucketExists(c.Bucket)
		if err != nil {
			return err
		}

		if !exists {
			if err := c.Client.MakeBucket(c.Bucket, c.Region); err != nil {
				return err
			}
		}

		var buf bytes.Buffer
		if err := t.Metainfo().Write(&buf); err != nil {
			return err
		}

		filename := fmt.Sprintf("%s.torrent", t.InfoHash().HexString())
		_, err = c.Client.PutObject(c.Bucket, filename, &buf, int64(buf.Len()), minio.PutObjectOptions{})
		return err
	case <-t.Closed():
		return errors.New("torrent closed before info ready")
	}
}

func (c *MinioCache) LoadTorrents() ([]torrent.TorrentSpec, error) {
	exists, err := c.Client.BucketExists(c.Bucket)
	if err != nil {
		return nil, err
	}

	var specs []torrent.TorrentSpec
	if !exists {
		return specs, nil
	}

	doneCh := make(chan struct{})
	defer close(doneCh)

	objectsChan := c.Client.ListObjectsV2(c.Bucket, "", false, doneCh)
	for info := range objectsChan {
		if info.Err != nil {
			return nil, info.Err
		}
		if !strings.HasSuffix(info.Key, ".torrent") {
			continue
		}
		obj, err := c.Client.GetObject(c.Bucket, info.Key, minio.GetObjectOptions{})
		if err != nil {
			return nil, err
		}
		spec, err := specFromTorrentReader(obj)
		if err != nil {
			return nil, err
		}
		specs = append(specs, *spec)
	}
	return specs, nil
}

func (c *MinioCache) DeleteTorrent(t *torrent.Torrent) error {
	filename := fmt.Sprintf("%s.torrent", t.InfoHash().HexString())
	return c.Client.RemoveObject(c.Bucket, filename)
}

package cache

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/joelanford/torrential/internal/convert"

	"github.com/anacrolix/torrent"
	minio "github.com/minio/minio-go"
	"github.com/pkg/errors"
)

type Minio struct {
	client *minio.Client
	region string
	bucket string
}

func NewMinio(client *minio.Client, bucket string) *Minio {
	return &Minio{
		client: client,
		region: "",
		bucket: bucket,
	}
}

func NewMinioWithRegion(client *minio.Client, bucket, region string) *Minio {
	return &Minio{
		client: client,
		region: region,
		bucket: bucket,
	}
}

func (c *Minio) SaveTorrent(t *torrent.Torrent) error {
	select {
	case <-t.GotInfo():
		exists, err := c.client.BucketExists(c.bucket)
		if err != nil {
			return err
		}

		if !exists {
			if err := c.client.MakeBucket(c.bucket, c.region); err != nil {
				return err
			}
		}

		var buf bytes.Buffer
		if err := t.Metainfo().Write(&buf); err != nil {
			return err
		}

		filename := fmt.Sprintf("%s.torrent", t.InfoHash().HexString())
		_, err = c.client.PutObject(c.bucket, filename, &buf, int64(buf.Len()), minio.PutObjectOptions{})
		return err
	case <-t.Closed():
		return errors.New("torrent closed before info ready")
	}
}

func (c *Minio) LoadTorrents() ([]torrent.TorrentSpec, error) {
	exists, err := c.client.BucketExists(c.bucket)
	if err != nil {
		return nil, err
	}

	var specs []torrent.TorrentSpec
	if !exists {
		return specs, nil
	}

	doneCh := make(chan struct{})
	defer close(doneCh)

	objectsChan := c.client.ListObjectsV2(c.bucket, "", false, doneCh)
	for info := range objectsChan {
		if info.Err != nil {
			return nil, info.Err
		}
		if !strings.HasSuffix(info.Key, ".torrent") {
			continue
		}
		obj, err := c.client.GetObject(c.bucket, info.Key, minio.GetObjectOptions{})
		if err != nil {
			return nil, err
		}
		spec, err := convert.ReaderToTorrentSpec(obj)
		if err != nil {
			return nil, err
		}
		specs = append(specs, *spec)
	}
	return specs, nil
}

func (c *Minio) DeleteTorrent(t *torrent.Torrent) error {
	filename := fmt.Sprintf("%s.torrent", t.InfoHash().HexString())
	return c.client.RemoveObject(c.bucket, filename)
}

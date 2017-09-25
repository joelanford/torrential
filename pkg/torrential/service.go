package torrential

import (
	"io"
	"net/http"
	"os"
	"sync"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/pkg/errors"
)

type Service struct {
	client   *atorrent.Client
	cache    Cache
	eventers map[string]*Eventer

	defaultSeedRatio float64

	eventerMu sync.RWMutex
}

type Config struct {
	ClientConfig *atorrent.Config
	Cache        Cache
	SeedRatio    float64
}

func NewService(conf *Config) (*Service, error) {
	if conf == nil {
		conf = &Config{}
	}
	if conf.ClientConfig == nil {
		conf.ClientConfig = &atorrent.Config{}
	}
	if conf.SeedRatio > 0 {
		conf.ClientConfig.Seed = true
	}

	client, err := atorrent.NewClient(conf.ClientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "could not create client")
	}
	svc := &Service{
		client:           client,
		cache:            conf.Cache,
		defaultSeedRatio: conf.SeedRatio,
		eventers:         make(map[string]*Eventer),
	}
	if svc.cache != nil {
		specs, err := svc.cache.LoadTorrents()
		if err != nil {
			return nil, errors.Wrap(err, "could not load cache")
		}
		for _, spec := range specs {
			if _, _, err := svc.client.AddTorrentSpec(&spec); err != nil {
				return nil, err
			}
		}
	}
	return svc, nil
}

func (svc *Service) Torrents() []*atorrent.Torrent {
	return svc.client.Torrents()
}

func (svc *Service) Torrent(infoHash string) (*atorrent.Torrent, error) {
	var h metainfo.Hash
	if err := h.FromHexString(infoHash); err != nil {
		return nil, errors.Wrap(parseErr{err}, "bad torrent hash")
	}
	torrent, ok := svc.client.Torrent(h)
	if !ok {
		return nil, notFoundErr{errors.New("torrent not found")}
	}
	return torrent, nil
}

func (svc *Service) AddTorrentReader(torrentReader io.Reader) (*atorrent.Torrent, error) {
	spec, err := specFromTorrentReader(torrentReader)
	if err != nil {
		return nil, errors.Wrap(parseErr{err}, "could not parse spec from torrent")
	}
	return svc.addTorrentSpec(spec)
}

func (svc *Service) AddTorrentURL(torrentURL string) (*atorrent.Torrent, error) {
	resp, err := http.Get(torrentURL)
	if err != nil {
		return nil, errors.Wrap(fetchErr{err}, "could not fetch torrent")
	}
	spec, err := specFromTorrentReader(resp.Body)
	if err != nil {
		return nil, errors.Wrap(parseErr{err}, "could not parse spec from torrent")
	}
	return svc.addTorrentSpec(spec)
}

func (svc *Service) AddMagnetURI(magnetURI string) (*atorrent.Torrent, error) {
	spec, err := atorrent.TorrentSpecFromMagnetURI(magnetURI)
	if err != nil {
		return nil, errors.Wrap(parseErr{err}, "could not parse spec from magnet URI")
	}
	return svc.addTorrentSpec(spec)
}

func (svc *Service) Eventer(infoHash string) (*Eventer, error) {
	svc.eventerMu.RLock()
	defer svc.eventerMu.RUnlock()
	eventer, ok := svc.eventers[infoHash]
	if !ok {
		return nil, notFoundErr{errors.New("torrent not found")}
	}
	return eventer, nil
}

func (svc *Service) Drop(infoHash string, deleteFiles bool) error {
	var h metainfo.Hash
	if err := h.FromHexString(infoHash); err != nil {
		return errors.Wrap(parseErr{err}, "bad infoHash")
	}
	t, ok := svc.client.Torrent(h)
	if !ok {
		return notFoundErr{errors.New("torrent not found")}
	}
	t.Drop()

	svc.eventerMu.Lock()
	delete(svc.eventers, infoHash)
	svc.eventerMu.Unlock()

	if svc.cache != nil {
		if err := svc.cache.DeleteTorrent(t); err != nil {
			return errors.Wrap(deleteErr{err}, "could not delete cached torrent metadata")
		}
	}
	if deleteFiles {
		for _, f := range t.Files() {
			if err := os.RemoveAll(f.Path()); err != nil {
				return errors.Wrap(deleteErr{err}, "could not delete torrent files")
			}
		}
	}
	return nil
}

func (svc *Service) addTorrentSpec(spec *atorrent.TorrentSpec) (*atorrent.Torrent, error) {
	t, new, err := svc.client.AddTorrentSpec(spec)
	if !new {
		return nil, existsErr{errors.New("torrent already exists")}
	}
	if err != nil {
		return nil, errors.Wrap(addTorrentErr{err}, "could not add torrent")
	}

	svc.eventerMu.Lock()
	svc.eventers[spec.InfoHash.String()] = NewEventer(t, SeedRatio(svc.defaultSeedRatio))
	svc.eventerMu.Unlock()

	if svc.cache != nil {
		if err := svc.cache.SaveTorrent(t); err != nil {
			return nil, errors.Wrap(cacheErr{err}, "could not save torrent metadata")
		}
	}
	go func() {
		select {
		case <-t.Closed():
		case <-t.GotInfo():
			t.DownloadAll()
		}
	}()
	return t, nil
}

func specFromTorrentReader(r io.Reader) (*atorrent.TorrentSpec, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return nil, err
	}
	return atorrent.TorrentSpecFromMetaInfo(mi), nil
}

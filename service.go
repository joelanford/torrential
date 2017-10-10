package torrential

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/pkg/errors"

	"github.com/joelanford/torrential/cache"
)

type Service struct {
	client       *torrent.Client
	multiEventer *MultiEventer
	eventers     map[string]*TorrentEventer
	conf         *Config
	eventerMu    sync.RWMutex
}

func NewService(conf *Config) (*Service, error) {
	if conf == nil {
		conf = &Config{}
	}
	if conf.ClientConfig == nil {
		conf.ClientConfig = &torrent.Config{}
	}
	if conf.SeedRatio > 0 {
		conf.ClientConfig.Seed = true
	}

	client, err := torrent.NewClient(conf.ClientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "could not create client")
	}
	svc := &Service{
		client:       client,
		conf:         conf,
		multiEventer: newMultiEventer(),
		eventers:     make(map[string]*TorrentEventer),
	}
	if svc.conf.Cache != nil {
		specs, err := svc.conf.Cache.LoadTorrents()
		if err != nil {
			return nil, errors.Wrap(err, "could not load cache")
		}
		for _, spec := range specs {
			if _, err := svc.addTorrentSpec(&spec); err != nil {
				return nil, err
			}
		}
	}
	return svc, nil
}

func (svc *Service) Torrents() (torrents []Torrent) {
	for _, torrent := range svc.client.Torrents() {
		torrents = append(torrents, Torrent{torrent})
	}
	return
}

func (svc *Service) Torrent(infoHash string) (*Torrent, error) {
	var h metainfo.Hash
	if err := h.FromHexString(infoHash); err != nil {
		return nil, errors.Wrap(parseErr{err}, "bad torrent hash")
	}
	torrent, ok := svc.client.Torrent(h)
	if !ok {
		return nil, notFoundErr{errors.New("torrent not found")}
	}
	return &Torrent{torrent}, nil
}

func (svc *Service) AddTorrentReader(torrentReader io.Reader) (*Torrent, error) {
	mi, err := metainfo.Load(torrentReader)
	if err != nil {
		return nil, errors.Wrap(parseErr{err}, "could not parse spec from torrent")
	}
	return svc.addTorrentSpec(torrent.TorrentSpecFromMetaInfo(mi))
}

func (svc *Service) AddTorrentURL(torrentURL string) (*Torrent, error) {
	resp, err := http.Get(torrentURL)
	if err != nil {
		return nil, errors.Wrap(fetchErr{err}, "could not fetch torrent")
	}
	mi, err := metainfo.Load(resp.Body)
	if err != nil {
		return nil, errors.Wrap(parseErr{err}, "could not parse spec from torrent")
	}
	return svc.addTorrentSpec(torrent.TorrentSpecFromMetaInfo(mi))
}

func (svc *Service) AddMagnetURI(magnetURI string) (*Torrent, error) {
	spec, err := torrent.TorrentSpecFromMagnetURI(magnetURI)
	if err != nil {
		return nil, errors.Wrap(parseErr{err}, "could not parse spec from magnet URI")
	}
	return svc.addTorrentSpec(spec)
}

func (svc *Service) Eventer(infoHash string) (*TorrentEventer, error) {
	svc.eventerMu.RLock()
	defer svc.eventerMu.RUnlock()
	e, ok := svc.eventers[infoHash]
	if !ok {
		return nil, notFoundErr{errors.New("torrent not found")}
	}
	return e, nil
}

func (svc *Service) MultiEventer() *MultiEventer {
	return svc.multiEventer
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

	if svc.conf.Cache != nil {
		if err := svc.conf.Cache.DeleteTorrent(t); err != nil {
			return errors.Wrap(deleteErr{err}, "could not delete cached torrent metadata")
		}
	}
	if deleteFiles {
		directories := make(map[string]struct{})
		for _, f := range t.Files() {
			dirs := strings.Split(f.Path(), string(os.PathSeparator))
			if len(dirs) > 1 {
				directories[dirs[0]] = struct{}{}
			}
			if err := os.RemoveAll(filepath.Join(svc.conf.ClientConfig.DataDir, f.Path())); err != nil {
				return errors.Wrap(deleteErr{err}, "could not delete torrent files")
			}
		}
		for d := range directories {
			if err := os.RemoveAll(filepath.Join(svc.conf.ClientConfig.DataDir, d)); err != nil {
				return errors.Wrap(deleteErr{err}, "could not delete torrent files")
			}
		}
	}
	return nil
}

func (svc *Service) addTorrentSpec(spec *torrent.TorrentSpec) (*Torrent, error) {
	t, new, err := svc.client.AddTorrentSpec(spec)
	if !new {
		return nil, existsErr{errors.New("torrent already exists")}
	}
	if err != nil {
		return nil, errors.Wrap(addTorrentErr{err}, "could not add torrent")
	}

	torrent := Torrent{t}

	e := newTorrentEventer(torrent, SeedRatio(svc.conf.SeedRatio))
	svc.multiEventer.add(e)

	svc.eventerMu.Lock()
	svc.eventers[spec.InfoHash.String()] = e
	svc.eventerMu.Unlock()

	if svc.conf.Cache != nil {
		if err := svc.conf.Cache.SaveTorrent(t); err != nil {
			return nil, errors.Wrap(cacheErr{err}, "could not save torrent metadata")
		}
	}
	go func() {
		select {
		case <-e.Closed():
		case <-e.GotInfo():
			t.DownloadAll()
		}
	}()
	go func() {
		background := make(chan struct{})
		for event := range e.Events(background) {
			if svc.conf.WebhookURL != "" {
				if err := invokeWebhook(event, svc.conf.WebhookURL); err != nil {
					log.Printf("error invoking webhook %s for %s event for torrent %s: %s", svc.conf.WebhookURL, event.Type, event.Torrent.InfoHash().String(), err)
				}
			}
			if event.Type == SeedingDone && svc.conf.DropWhenDone {
				event.Torrent.Drop()
			}
		}
	}()
	return &torrent, nil
}

type Config struct {
	ClientConfig *torrent.Config
	Cache        cache.Cache
	WebhookURL   string
	SeedRatio    float64
	DropWhenDone bool
}

func invokeWebhook(e Event, url string) error {
	jsonData, err := json.Marshal(eventResult{e})
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return errors.New(resp.Status)
	}
	return nil
}

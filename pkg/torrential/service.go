package torrential

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/pkg/errors"
)

type Service struct {
	client    *atorrent.Client
	eventers  map[string]*Eventer
	conf      *Config
	eventerMu sync.RWMutex
}

type Config struct {
	ClientConfig *atorrent.Config
	Cache        Cache
	Webhooks     Webhooks
	SeedRatio    float64
}

type Webhooks struct {
	Added        string
	GotInfo      string
	FileDone     string
	DownloadDone string
	SeedingDone  string
	Closed       string
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
		client:   client,
		conf:     conf,
		eventers: make(map[string]*Eventer),
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
			if err := os.RemoveAll(f.Path()); err != nil {
				return errors.Wrap(deleteErr{err}, "could not delete torrent files")
			}
		}
		for d := range directories {
			if err := os.RemoveAll(d); err != nil {
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

	eventer := NewEventer(t, SeedRatio(svc.conf.SeedRatio))

	svc.eventerMu.Lock()
	svc.eventers[spec.InfoHash.String()] = eventer
	svc.eventerMu.Unlock()

	if svc.conf.Cache != nil {
		if err := svc.conf.Cache.SaveTorrent(t); err != nil {
			return nil, errors.Wrap(cacheErr{err}, "could not save torrent metadata")
		}
	}
	go func() {
		select {
		case <-eventer.Closed():
		case <-eventer.GotInfo():
			t.DownloadAll()
		}
	}()
	go func() {
		for e := range eventer.Events() {
			switch e.Type {
			case EventAdded:
				if err := invokeWebhook(&e, svc.conf.Webhooks.Added); err != nil {
					log.Printf("error invoking %s webhook %s for torrent %s: %s", eventTypeString(e.Type), svc.conf.Webhooks.Added, e.Torrent.InfoHash().String(), err)
				}
			case EventGotInfo:
				if err := invokeWebhook(&e, svc.conf.Webhooks.GotInfo); err != nil {
					log.Printf("error invoking %s webhook %s for torrent %s: %s", eventTypeString(e.Type), svc.conf.Webhooks.GotInfo, e.Torrent.InfoHash().String(), err)
				}
			case EventFileDone:
				if err := invokeWebhook(&e, svc.conf.Webhooks.FileDone); err != nil {
					log.Printf("error invoking %s webhook %s for torrent %s: %s", eventTypeString(e.Type), svc.conf.Webhooks.FileDone, e.Torrent.InfoHash().String(), err)
				}
			case EventDownloadDone:
				if err := invokeWebhook(&e, svc.conf.Webhooks.DownloadDone); err != nil {
					log.Printf("error invoking %s webhook %s for torrent %s: %s", eventTypeString(e.Type), svc.conf.Webhooks.DownloadDone, e.Torrent.InfoHash().String(), err)
				}
			case EventSeedingDone:
				if err := invokeWebhook(&e, svc.conf.Webhooks.SeedingDone); err != nil {
					log.Printf("error invoking %s webhook %s for torrent %s: %s", eventTypeString(e.Type), svc.conf.Webhooks.SeedingDone, e.Torrent.InfoHash().String(), err)
				}
			case EventClosed:
				if err := invokeWebhook(&e, svc.conf.Webhooks.DownloadDone); err != nil {
					log.Printf("error invoking %s webhook %s for torrent %s: %s", eventTypeString(e.Type), svc.conf.Webhooks.Closed, e.Torrent.InfoHash().String(), err)
				}
			}
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

func invokeWebhook(e *Event, url string) error {
	if url != "" {
		var file *torrentFile
		if e.File != nil {
			file = toTorrentFile(e.File)
		}
		jsonData, err := json.Marshal(eventResult{Event: event{
			Type:    eventTypeString(e.Type),
			Torrent: toTorrent(e.Torrent),
			File:    file,
		}})
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
	}
	return nil
}

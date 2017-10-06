package torrential

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"

	"github.com/joelanford/torrential/eventer"
	"github.com/joelanford/torrential/internal/convert"
	t "github.com/joelanford/torrential/internal/torrential"
)

type Webhooks struct {
	Added        string
	GotInfo      string
	FileDone     string
	DownloadDone string
	SeedingDone  string
	Closed       string
}

func WebhookAll(webhookURL string) Webhooks {
	return Webhooks{
		Added:        webhookURL,
		GotInfo:      webhookURL,
		FileDone:     webhookURL,
		DownloadDone: webhookURL,
		SeedingDone:  webhookURL,
		Closed:       webhookURL,
	}
}

func invokeWebhook(e eventer.Event, url string) error {
	if url != "" {
		var file *t.File
		if e.File != nil {
			file = &t.File{}
			*file = convert.File(*e.File)
		}
		jsonData, err := json.Marshal(eventResult{Event: t.Event{
			Type:    e.Type.String(),
			Torrent: convert.Torrent(e.Torrent),
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

package torrential

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

type Webhooks struct {
	Added        string
	GotInfo      string
	FileDone     string
	DownloadDone string
	SeedingDone  string
	Closed       string
}

func SingleWebhook(webhookURL string) Webhooks {
	return Webhooks{
		Added:        webhookURL,
		GotInfo:      webhookURL,
		FileDone:     webhookURL,
		DownloadDone: webhookURL,
		SeedingDone:  webhookURL,
		Closed:       webhookURL,
	}
}

func PathWebhooks(webhookBaseURL string) Webhooks {
	webhookBaseURL = strings.TrimSuffix(webhookBaseURL, "/")
	return Webhooks{
		Added:        fmt.Sprintf("%s/added", webhookBaseURL),
		GotInfo:      fmt.Sprintf("%s/gotInfo", webhookBaseURL),
		FileDone:     fmt.Sprintf("%s/fileDone", webhookBaseURL),
		DownloadDone: fmt.Sprintf("%s/downloadDone", webhookBaseURL),
		SeedingDone:  fmt.Sprintf("%s/seedingDone", webhookBaseURL),
		Closed:       fmt.Sprintf("%s/closed", webhookBaseURL),
	}
}

func invokeWebhook(e Event, url string) error {
	if url != "" {
		var file *torrentFileJSON
		if e.File != nil {
			file = toTorrentFileJSON(e.File)
		}
		jsonData, err := json.Marshal(eventResult{Event: eventJSON{
			Type:    eventTypeString(e.Type),
			Torrent: toTorrentJSON(e.Torrent),
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

package main

import (
	"log"
	"net/http"

	"github.com/joelanford/torrential/pkg/torrential"
)

func main() {
	svc, err := torrential.NewService(&torrential.Config{
		Cache:     torrential.NewFileCache(".torrent"),
		SeedRatio: 0.00001,
		Webhooks: torrential.Webhooks{
			Added:        "http://localhost:8081/webhook/added",
			GotInfo:      "http://localhost:8081/webhook/gotInfo",
			FileDone:     "http://localhost:8081/webhook/fileDone",
			DownloadDone: "http://localhost:8081/webhook/downloadDone",
			SeedingDone:  "http://localhost:8081/webhook/seedingDone",
			Closed:       "http://localhost:8081/webhook/closed",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	h := torrential.NewHTTPHandler(svc)

	log.Fatal(http.ListenAndServe(":8080", h))
}

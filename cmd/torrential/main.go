package main

import (
	"log"
	"net/http"

	"github.com/joelanford/torrential/pkg/torrential"
)

func main() {
	svc, err := torrential.NewService(&torrential.Config{
		Cache:     torrential.NewFileCache(".torrent"),
		SeedRatio: 1.0,
	})
	if err != nil {
		log.Fatal(err)
	}

	h := torrential.NewHTTPHandler(svc)

	log.Fatal(http.ListenAndServe(":8080", h))
}

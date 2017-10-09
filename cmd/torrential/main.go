package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/anacrolix/torrent"
	"github.com/gorilla/mux"
	"github.com/joelanford/torrential"
	"github.com/joelanford/torrential/cache"
)

var (
	listenAddr   string
	downloadDir  string
	torrentsDir  string
	seedRatio    float64
	dropWhenDone bool
	webhookURL   string
	httpBasePath string
)

func main() {
	flag.StringVar(&listenAddr, "listen-addr", ":8080", "Address to listen on")
	flag.StringVar(&downloadDir, "download-dir", "torrential/downloads", "Directory in which to download torrent data")
	flag.StringVar(&torrentsDir, "torrents-dir", "torrential/torrents", "Directory in which to cache active torrent metadata files")
	flag.Float64Var(&seedRatio, "seed-ratio", 1.0, "Seed ratio of torrents that determines when seed ratio events and webhooks are invoked")
	flag.BoolVar(&dropWhenDone, "drop-done", true, "Drop the torrent when the download completes (or when the seed ratio is met, if enabled)")
	flag.StringVar(&webhookURL, "webhook-url", "http://localhost:8080/webhook", "Webhook to invoke for torrent events")
	flag.StringVar(&httpBasePath, "http-basepath", "/", "Base path of torrential HTTP handler")

	flag.Parse()

	svc, err := torrential.NewService(&torrential.Config{
		ClientConfig: &torrent.Config{
			DataDir: downloadDir,
		},
		Cache:        cache.NewDirectory(torrentsDir),
		SeedRatio:    seedRatio,
		DropWhenDone: dropWhenDone,
		WebhookURL:   webhookURL,
	})
	if err != nil {
		log.Fatal(err)
	}

	router := mux.NewRouter()
	router.PathPrefix(httpBasePath).Handler(torrential.Handler(httpBasePath, svc))

	log.Fatal(http.ListenAndServe(listenAddr, router))
}

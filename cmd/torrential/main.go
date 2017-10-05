package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/anacrolix/torrent"
	"github.com/gorilla/mux"
	"github.com/joelanford/torrential"
)

var (
	listenAddr   string
	downloadDir  string
	torrentsDir  string
	seedRatio    float64
	webhookURL   string
	httpBasePath string
)

func main() {
	flag.StringVar(&listenAddr, "listen-addr", ":8080", "Address to listen on")
	flag.StringVar(&downloadDir, "download-dir", "/tmp/torrential/downloads", "Directory in which to download torrent data")
	flag.StringVar(&torrentsDir, "torrents-dir", "/tmp/torrential/torrents", "Directory in which to cache active torrent metadata files")
	flag.Float64Var(&seedRatio, "seed-ratio", 0.0, "Seed ratio of torrents that determines when seed ratio events and webhooks are invoked")
	flag.StringVar(&webhookURL, "webhook-url", "http://localhost:8080/webhook", "Webhook to invoke for torrent events")
	flag.StringVar(&httpBasePath, "http-base-path", "/", "Base path of torrential HTTP handler")

	flag.Parse()

	svc, err := torrential.NewService(&torrential.Config{
		ClientConfig: &torrent.Config{
			DataDir: downloadDir,
		},
		Cache:     torrential.NewFileCache(torrentsDir),
		SeedRatio: seedRatio,
		Webhooks:  torrential.WebhookAll(webhookURL),
	})
	if err != nil {
		log.Fatal(err)
	}

	router := mux.NewRouter()
	router.PathPrefix("/webhook").HandlerFunc(dumpRequest)
	router.PathPrefix(httpBasePath).Handler(torrential.Handler(httpBasePath, svc))

	log.Fatal(http.ListenAndServe(listenAddr, router))
}

func dumpRequest(w http.ResponseWriter, r *http.Request) {
	output, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	fmt.Println(string(output))
}

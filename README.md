# torrential

[![GoDoc](https://godoc.org/github.com/joelanford/torrential?status.svg)](https://godoc.org/github.com/joelanford/torrential)
[![Build Status](https://travis-ci.org/joelanford/torrential.svg?branch=master)](https://travis-ci.org/joelanford/torrential)
[![Sourcegraph](https://sourcegraph.com/github.com/joelanford/torrential/-/badge.svg)](https://sourcegraph.com/github.com/joelanford/torrential?badge)

The `joelanford/torrential` package implements a conventient service and an HTTP handler for bittorrent downloading and monitoring. 

`torrential.Service` has methods for adding torrents from an `io.Reader` of the torrent file format, HTTP URLs to torrent files, and magnet links. It also has methods for retrieving all active torrents (or individual torrents by their info hash) and channels of events. It can also be configured to invoke a webhook on torrent events.

`torrential.Handler` wraps `torrential.Service` to expose the service methods via RESTful HTTP endpoints.

## Installation

Assuming a correctly configured go environment, one can run the following command to install `joelanford/torrential` in `$GOPATH`

```sh
go get -u github.com/joelanford/torrential
```

## Example

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/anacrolix/torrent"
	"github.com/gorilla/mux"
	"github.com/joelanford/torrential"
	"github.com/joelanford/torrential/cache"
)

func main() {
	svc, err := torrential.NewService(&torrential.Config{
		ClientConfig: &torrent.Config{
			DataDir: "torrential/downloads",
		},
		Cache:      cache.NewDirectory("torrential/cache"),
		SeedRatio:  1.0,
		WebhookURL: "http://localhost:8080/webhook",
	})
	if err != nil {
		log.Fatal(err)
	}

	router := mux.NewRouter()
	router.PathPrefix("/webhook").HandlerFunc(dumpRequest)
	router.PathPrefix("/torrential").Handler(torrential.Handler("/torrential", svc))

	log.Fatal(http.ListenAndServe(":8080", router))
}

func dumpRequest(w http.ResponseWriter, r *http.Request) {
	output, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	fmt.Println(string(output))
}
```

## Special Thanks

 Thanks to the maintainers and all of the contributors of the [anacrolix/torrent](https://github.com/anacrolix/torrent) project! This project is heavily dependent on it and wouldn't exist without it.

## License

GPLv3 licensed. See the LICENSE file for details.
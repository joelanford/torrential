package torrential_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/joelanford/torrential"
	"github.com/stretchr/testify/assert"
)

var (
	c *torrent.Client
)

func TestMain(m *testing.M) {
	var err error
	c, err = torrent.NewClient(&torrent.Config{
		DataDir: ".torrent-test-data",
		NoDHT:   true,
	})
	if err != nil {
		panic(err)
	}
	exitStatus := m.Run()

	c.Close()
	os.RemoveAll(".torrent-test-data")

	os.Exit(exitStatus)
}

func TestTorrentMarshalJSON(t *testing.T) {
	// Test empty Torrent
	data, err := json.Marshal(torrential.Torrent{})
	assert.NoError(t, err)
	assert.JSONEq(t, `null`, string(data))

	// Test non-empty Torrent
	tor, err := c.AddTorrentFromFile("testdata/sample.torrent")
	assert.NoError(t, err)
	defer tor.Drop()

	data, err = json.Marshal(torrential.Torrent{Torrent: tor})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"bytesCompleted":0,"bytesMissing":20,"files":[{"displayPath":"sample.txt","length":20,"offset":0,"path":"sample.txt"}],"infoHash":"d0d14c926e6e99761a2fdcff27b403d96376eff6","length":20,"magnetLink":"magnet:?xt=urn:btih:d0d14c926e6e99761a2fdcff27b403d96376eff6\u0026dn=sample.txt\u0026tr=udp%3A%2F%2Ftracker.openbittorrent.com%3A80","name":"sample.txt","numPieces":1,"seeding":false,"stats":{"activePeers":0,"bytesRead":0,"bytesWritten":0,"chunksRead":0,"chunksWritten":0,"dataBytesRead":0,"dataBytesWritten":0,"halfOpenPeers":0,"pendingPeers":0,"totalPeers":0},"hasInfo":true}`, string(data))
}

func TestFileMarshalJSON(t *testing.T) {
	// Test empty File
	data, err := json.Marshal(torrential.File{})
	assert.NoError(t, err)
	assert.JSONEq(t, `null`, string(data))

	// Test non-empty File
	tor, err := c.AddTorrentFromFile("testdata/sample.torrent")
	assert.NoError(t, err)
	defer tor.Drop()

	data, err = json.Marshal(torrential.File{File: &tor.Files()[0]})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"displayPath":"sample.txt","length":20,"offset":0,"path":"sample.txt"}`, string(data))
}

func TestEventTypeString(t *testing.T) {
	assert.Equal(t, "added", torrential.Added.String())
	assert.Equal(t, "gotInfo", torrential.GotInfo.String())
	assert.Equal(t, "pieceDone", torrential.PieceDone.String())
	assert.Equal(t, "fileDone", torrential.FileDone.String())
	assert.Equal(t, "downloadDone", torrential.DownloadDone.String())
	assert.Equal(t, "seedingDone", torrential.SeedingDone.String())
	assert.Equal(t, "closed", torrential.Closed.String())
	assert.Equal(t, "unknown", torrential.EventType(7).String())
}
func TestEventTypeMarshalJSON(t *testing.T) {
	actual, err := torrential.Added.MarshalJSON()
	assert.JSONEq(t, "\"added\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.GotInfo.MarshalJSON()
	assert.JSONEq(t, "\"gotInfo\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.PieceDone.MarshalJSON()
	assert.JSONEq(t, "\"pieceDone\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.FileDone.MarshalJSON()
	assert.JSONEq(t, "\"fileDone\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.DownloadDone.MarshalJSON()
	assert.JSONEq(t, "\"downloadDone\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.SeedingDone.MarshalJSON()
	assert.JSONEq(t, "\"seedingDone\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.Closed.MarshalJSON()
	assert.Equal(t, "\"closed\"", string(actual))
	assert.NoError(t, err)

	actual, err = torrential.EventType(7).MarshalJSON()
	assert.Equal(t, "\"unknown\"", string(actual))
	assert.NoError(t, err)
}

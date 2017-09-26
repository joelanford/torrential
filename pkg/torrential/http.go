package torrential

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"

	atorrent "github.com/anacrolix/torrent"
)

type handler struct {
	ts       *Service
	upgrader *websocket.Upgrader
}

func NewHTTPHandler(svc *Service) http.Handler {
	r := mux.NewRouter()
	h := handler{
		ts: svc,
		upgrader: &websocket.Upgrader{
			Error: func(w http.ResponseWriter, r *http.Request, code int, err error) {
				encodeError(w, code, err)
			},
		},
	}

	r.Path("/torrents").Methods("HEAD").HandlerFunc(h.headTorrents)
	r.Path("/torrents").Methods("GET").HandlerFunc(h.getTorrents)
	r.Path("/torrents").Methods("POST").Headers("Content-Type", "application/x-bittorrent").HandlerFunc(h.postTorrentData)
	r.Path("/torrents").Methods("POST").Headers("Content-Type", "application/x-url").HandlerFunc(h.postTorrentURL)
	r.Path("/torrents").Methods("POST").Headers("Content-Type", "x-scheme-handler/magnet").HandlerFunc(h.postMagnetURI)
	r.Path("/torrents").Methods("POST").HandlerFunc(h.badContentType)
	r.Path("/torrents").HandlerFunc(h.supportedMethods("HEAD", "GET", "POST"))

	r.Path("/torrents/{infoHash}").Methods("HEAD").HandlerFunc(h.headTorrent)
	r.Path("/torrents/{infoHash}").Methods("GET").HandlerFunc(h.getTorrent)
	r.Path("/torrents/{infoHash}").Methods("DELETE").HandlerFunc(h.deleteTorrent)
	r.Path("/torrents/{infoHash}/events").Methods("GET").HandlerFunc(h.getTorrentEvents)
	r.Path("/torrents").HandlerFunc(h.supportedMethods("HEAD", "GET", "DELETE"))

	return r
}

// headTorrents returns the headers and status code for torrents
func (h *handler) headTorrents(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// getTorrents returns all active torrents
func (h *handler) getTorrents(w http.ResponseWriter, r *http.Request) {
	torrents := h.ts.Torrents()
	encodeTorrents(w, http.StatusOK, torrents)
}

// postTorrentData adds a new torrent from torrent data
func (h *handler) postTorrentData(w http.ResponseWriter, r *http.Request) {
	torrent, err := h.ts.AddTorrentReader(r.Body)
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}
	encodeTorrent(w, http.StatusCreated, torrent)
}

// postTorrentURL adds a new torrent from a torrent URL
func (h *handler) postTorrentURL(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}

	torrent, err := h.ts.AddTorrentURL(string(data))
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}
	encodeTorrent(w, http.StatusCreated, torrent)
}

// postMagnetURI adds a new torrent from a magnet URI
func (h *handler) postMagnetURI(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}

	torrent, err := h.ts.AddMagnetURI(string(data))
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}
	encodeTorrent(w, http.StatusCreated, torrent)
}

// headTorrent returns the headers and status code given an info hash
func (h *handler) headTorrent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	infoHash, ok := vars["infoHash"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err := h.ts.Torrent(infoHash)
	if err != nil {
		w.WriteHeader(httpStatus(err))
		return
	}
}

// getTorrent returns a torrent given an info hash
func (h *handler) getTorrent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	infoHash, ok := vars["infoHash"]
	if !ok {
		encodeError(w, http.StatusNotFound, errors.New("torrent not found"))
		return
	}
	torrent, err := h.ts.Torrent(infoHash)
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}
	encodeTorrent(w, http.StatusOK, torrent)
}

// deleteTorrent drops a torrent given an info hash
func (h *handler) deleteTorrent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	infoHash, ok := vars["infoHash"]
	if !ok {
		encodeError(w, http.StatusNotFound, errors.New("torrent not found"))
		return
	}
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"
	err := h.ts.Drop(infoHash, deleteFiles)
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}
	encodeEmptyResult(w, http.StatusOK)
}

// getTorrentEvents opens a websocket and sends events about the given torrent.
func (h *handler) getTorrentEvents(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	infoHash, ok := vars["infoHash"]
	if !ok {
		encodeError(w, http.StatusNotFound, errors.New("torrent not found"))
		return
	}
	eventer, err := h.ts.Eventer(infoHash)
	if err != nil {
		encodeError(w, httpStatus(err), err)
		return
	}

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// err is handled by h.upgrader.Error, which calls encodeError
		return
	}
	defer ws.Close()

	for e := range eventer.Events() {
		var file *torrentFile
		if e.File != nil {
			file = toTorrentFile(e.File)
		}
		ws.WriteJSON(eventResult{Event: event{
			Type:    eventTypeString(e.Type),
			Torrent: toTorrent(e.Torrent),
			File:    file,
		}})
	}
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

func encodeTorrent(w http.ResponseWriter, code int, t *atorrent.Torrent) {
	result := toTorrent(t)
	writeHeader(w, code)
	json.NewEncoder(w).Encode(torrentResult{Torrent: &result})
}

func encodeTorrents(w http.ResponseWriter, code int, torrents []*atorrent.Torrent) {
	results := make([]torrent, 0)
	for _, t := range torrents {
		results = append(results, toTorrent(t))
	}
	writeHeader(w, code)
	json.NewEncoder(w).Encode(torrentsResult{Torrents: results})
}

func encodeEmptyResult(w http.ResponseWriter, code int) {
	writeHeader(w, code)
	w.Write([]byte("{}"))
}

func encodeError(w http.ResponseWriter, code int, err error) {
	writeHeader(w, code)
	json.NewEncoder(w).Encode(errorResult{Error: err.Error()})
}

func writeHeader(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
}

type torrentResult struct {
	Torrent *torrent `json:"torrent"`
}

type torrentsResult struct {
	Torrents []torrent `json:"torrents"`
}

type eventResult struct {
	Event event `json:"event"`
}

type errorResult struct {
	Error string `json:"error"`
}

func (h *handler) badContentType(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Content-Type not supported"))
}

func (h *handler) supportedMethods(supportedMethods ...string) http.HandlerFunc {
	methods := make(map[string]struct{})
	for _, m := range supportedMethods {
		methods[m] = struct{}{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := methods[r.Method]
		if ok {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Allowed method has no handler\n"))
			return
		}

		w.Header()["Allow"] = append(supportedMethods, http.MethodOptions)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type torrent struct {
	BytesCompleted int            `json:"bytesCompleted"` // Number of bytes completed
	BytesMissing   int            `json:"bytesMissing"`   // Number of bytes missing
	Files          []*torrentFile `json:"files"`          // Files contained in the torrent
	InfoHash       string         `json:"infoHash"`       // Torrent info hash
	Length         int            `json:"length"`         // Total number of bytes in torrent
	MagnetLink     string         `json:"magnetLink"`     // Torrent magnet link
	Name           string         `json:"name"`           // Torrent name
	NumPieces      int            `json:"numPieces"`      // Total number of pieces in torrent
	Seeding        bool           `json:"seeding"`        // Whether torrent is currently seeding
	Stats          torrentStats   `json:"stats"`          // Torrent stats
	HasInfo        bool           `json:"hasInfo"`        // Whether the torrent info has been received
}

type torrentFile struct {
	DisplayPath string `json:"displayPath"`
	Length      int    `json:"length"`
	Offset      int    `json:"offset"`
	Path        string `json:"path"`
}

type torrentStats struct {
	ActivePeers      int `json:"activePeers"`
	BytesRead        int `json:"bytesRead"`
	BytesWritten     int `json:"bytesWritten"`
	ChunksRead       int `json:"chunksRead"`
	ChunksWritten    int `json:"chunksWritten"`
	DataBytesRead    int `json:"dataBytesRead"`
	DataBytesWritten int `json:"dataBytesWritten"`
	HalfOpenPeers    int `json:"halfOpenPeers"`
	PendingPeers     int `json:"pendingPeers"`
	TotalPeers       int `json:"totalPeers"`
}

type event struct {
	Type    string       `json:"type"`
	Torrent torrent      `json:"torrent"`
	File    *torrentFile `json:"file,omitempty"`
}

func eventTypeString(t EventType) string {
	switch t {
	case EventAdded:
		return "added"
	case EventGotInfo:
		return "gotInfo"
	case EventFileDone:
		return "fileDone"
	case EventDownloadDone:
		return "downloadDone"
	case EventSeedingDone:
		return "seedingDone"
	case EventClosed:
		return "closed"
	default:
		return "unknown"
	}
}

func toTorrent(t *atorrent.Torrent) torrent {
	metainfo := t.Metainfo()
	res := torrent{
		BytesCompleted: int(t.BytesCompleted()),
		BytesMissing:   0,
		Files:          toTorrentFiles(t.Files()),
		InfoHash:       t.InfoHash().String(),
		Length:         0,
		MagnetLink:     metainfo.Magnet(t.Name(), t.InfoHash()).String(),
		Name:           t.Name(),
		NumPieces:      0,
		Seeding:        t.Seeding(),
		Stats:          statsToTorrentStats(t.Stats()),
		HasInfo:        false,
	}
	select {
	case <-t.GotInfo():
		res.BytesMissing = int(t.BytesMissing())
		res.Length = int(t.Length())
		res.NumPieces = t.NumPieces()
		res.HasInfo = true
	default:
	}
	return res
}

func toTorrentFile(f *atorrent.File) *torrentFile {
	return &torrentFile{
		Path:        f.Path(),
		DisplayPath: f.DisplayPath(),
		Length:      int(f.Length()),
		Offset:      int(f.Offset()),
	}
}

func toTorrentFiles(v []atorrent.File) []*torrentFile {
	res := make([]*torrentFile, 0)
	for _, f := range v {
		res = append(res, toTorrentFile(&f))
	}
	return res
}

func statsToTorrentStats(v atorrent.TorrentStats) torrentStats {
	return torrentStats{
		BytesRead:        int(v.BytesRead),
		BytesWritten:     int(v.BytesWritten),
		ChunksRead:       int(v.ChunksRead),
		ChunksWritten:    int(v.ChunksWritten),
		DataBytesRead:    int(v.DataBytesRead),
		DataBytesWritten: int(v.DataBytesWritten),
		TotalPeers:       v.TotalPeers,
		ActivePeers:      v.ActivePeers,
		PendingPeers:     v.PendingPeers,
		HalfOpenPeers:    v.HalfOpenPeers,
	}
}

func httpStatus(err error) int {
	if e, ok := err.(notFoundErr); ok && e.IsNotFound() {
		return http.StatusNotFound
	} else if e, ok := err.(existsErr); ok && e.IsExists() {
		return http.StatusConflict
	} else if e, ok := err.(readErr); ok && e.IsReadError() {
		return http.StatusBadRequest
	} else if e, ok := err.(parseErr); ok && e.IsParseError() {
		return http.StatusBadRequest
	} else if e, ok := err.(addTorrentErr); ok && e.IsAddTorrentError() {
		return http.StatusInternalServerError
	} else if e, ok := err.(fetchErr); ok && e.IsFetchError() {
		return http.StatusInternalServerError
	} else if e, ok := err.(deleteErr); ok && e.IsDeleteError() {
		return http.StatusInternalServerError
	}

	return http.StatusInternalServerError
}

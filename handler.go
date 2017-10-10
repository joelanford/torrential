package torrential

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type handler struct {
	ts       *Service
	upgrader *websocket.Upgrader
}

func Handler(basePath string, svc *Service) http.Handler {
	r := mux.NewRouter()
	sr := r.PathPrefix(basePath).Subrouter()

	h := handler{
		ts: svc,
		upgrader: &websocket.Upgrader{
			Error: func(w http.ResponseWriter, r *http.Request, code int, err error) {
				encodeError(w, code, err)
			},
		},
	}

	sr.Path("/torrents/events").Methods("GET").HandlerFunc(h.getTorrentsEvents)
	sr.Path("/torrents/{infoHash}/events").Methods("GET").HandlerFunc(h.getTorrentEvents)

	sr.Path("/torrents").Methods("HEAD").HandlerFunc(h.headTorrents)
	sr.Path("/torrents").Methods("GET").HandlerFunc(h.getTorrents)
	sr.Path("/torrents").Methods("POST").Headers("Content-Type", "application/x-bittorrent").HandlerFunc(h.postTorrentData)
	sr.Path("/torrents").Methods("POST").Headers("Content-Type", "application/x-url").HandlerFunc(h.postTorrentURL)
	sr.Path("/torrents").Methods("POST").Headers("Content-Type", "x-scheme-handler/magnet").HandlerFunc(h.postMagnetURI)
	sr.Path("/torrents").Methods("POST").HandlerFunc(h.badContentType)
	sr.Path("/torrents").HandlerFunc(h.supportedMethods("HEAD", "GET", "POST"))

	sr.Path("/torrents/{infoHash}").Methods("HEAD").HandlerFunc(h.headTorrent)
	sr.Path("/torrents/{infoHash}").Methods("GET").HandlerFunc(h.getTorrent)
	sr.Path("/torrents/{infoHash}").Methods("DELETE").HandlerFunc(h.deleteTorrent)
	sr.Path("/torrents/{infoHash}").HandlerFunc(h.supportedMethods("HEAD", "GET", "DELETE"))

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

// getTorrentsEvents opens a websocket and sends events about all torrents.
func (h *handler) getTorrentsEvents(w http.ResponseWriter, r *http.Request) {
	eventer := h.ts.MultiEventer()

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// err is handled by h.upgrader.Error, which calls encodeError
		return
	}
	defer ws.Close()

	for e := range eventer.Events(r.Context().Done()) {
		ws.WriteJSON(eventResult{e})
	}
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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

	for e := range eventer.Events(r.Context().Done()) {
		ws.WriteJSON(eventResult{e})
	}
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

func encodeTorrent(w http.ResponseWriter, code int, torrent *Torrent) {
	writeHeader(w, code)
	json.NewEncoder(w).Encode(torrentResult{torrent})
}

func encodeTorrents(w http.ResponseWriter, code int, torrents []Torrent) {
	writeHeader(w, code)
	json.NewEncoder(w).Encode(torrentsResult{torrents})
}

func encodeEmptyResult(w http.ResponseWriter, code int) {
	writeHeader(w, code)
	w.Write([]byte("{}"))
}

func encodeError(w http.ResponseWriter, code int, err error) {
	writeHeader(w, code)
	json.NewEncoder(w).Encode(errorResult{err.Error()})
}

func writeHeader(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
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

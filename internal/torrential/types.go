package torrential

type Torrent struct {
	BytesCompleted int    `json:"bytesCompleted"` // Number of bytes completed
	BytesMissing   int    `json:"bytesMissing"`   // Number of bytes missing
	Files          []File `json:"files"`          // Files contained in the torrent
	InfoHash       string `json:"infoHash"`       // Torrent info hash
	Length         int    `json:"length"`         // Total number of bytes in torrent
	MagnetLink     string `json:"magnetLink"`     // Torrent magnet link
	Name           string `json:"name"`           // Torrent name
	NumPieces      int    `json:"numPieces"`      // Total number of pieces in torrent
	Seeding        bool   `json:"seeding"`        // Whether torrent is currently seeding
	Stats          Stats  `json:"stats"`          // Torrent stats
	HasInfo        bool   `json:"hasInfo"`        // Whether the torrent info has been received
}

type File struct {
	DisplayPath string `json:"displayPath"`
	Length      int    `json:"length"`
	Offset      int    `json:"offset"`
	Path        string `json:"path"`
}

type Stats struct {
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

type Event struct {
	Type    string  `json:"type"`
	Torrent Torrent `json:"torrent"`
	File    *File   `json:"file,omitempty"`
}

type TorrentResult struct {
	Torrent *Torrent `json:"torrent"`
}

type TorrentsResult struct {
	Torrents []Torrent `json:"torrents"`
}

type EventResult struct {
	Event Event `json:"event"`
}

type ErrorResult struct {
	Error string `json:"error"`
}

func NewTorrentResult(torrent *Torrent) *TorrentResult {
	return &TorrentResult{Torrent: torrent}
}
func NewTorrentsResult(torrents []Torrent) *TorrentsResult {
	return &TorrentsResult{Torrents: torrents}
}
func NewEventResult(event Event) *EventResult {
	return &EventResult{Event: event}
}
func NewErrorResult(err error) *ErrorResult {
	return &ErrorResult{Error: err.Error()}
}

package torrential

type notFoundErr struct {
	error
}
type existsErr struct {
	error
}
type readErr struct {
	error
}
type parseErr struct {
	error
}
type addTorrentErr struct {
	error
}
type fetchErr struct {
	error
}
type deleteErr struct {
	error
}
type cacheErr struct {
	error
}

func (e notFoundErr) IsNotFound() bool {
	return true
}
func (e existsErr) IsExists() bool {
	return true
}
func (e readErr) IsReadError() bool {
	return true
}
func (e parseErr) IsParseError() bool {
	return true
}
func (e addTorrentErr) IsAddTorrentError() bool {
	return true
}
func (e fetchErr) IsFetchError() bool {
	return true
}
func (e deleteErr) IsDeleteError() bool {
	return true
}
func (e cacheErr) IsCacheError() bool {
	return true
}

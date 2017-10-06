package eventer

import (
	"sync"

	uuid "github.com/satori/go.uuid"
)

type Multi struct {
	eventerChans map[string]chan Eventer
	eventerMap   map[string]Eventer
	numActive    int
	mutex        sync.RWMutex
}

var _ Eventer = &Multi{}

func NewMulti() *Multi {
	return &Multi{
		eventerChans: make(map[string]chan Eventer),
		eventerMap:   make(map[string]Eventer),
	}
}

func (e *Multi) Events(done <-chan struct{}) <-chan Event {
	events := make(chan Event)
	eventerChan := make(chan Eventer)

	id := uuid.NewV4().String()
	e.mutex.Lock()
	e.eventerChans[id] = eventerChan
	e.mutex.Unlock()

	go func() {
		e.mutex.RLock()
		defer e.mutex.RUnlock()
		for _, eventer := range e.eventerMap {
			eventerChan <- eventer
		}
	}()
	go func() {
		defer func() {
			e.mutex.Lock()
			delete(e.eventerChans, id)
			e.mutex.Unlock()
			close(events)
		}()
		for {
			select {
			case t, ok := <-eventerChan:
				if !ok {
					return
				}
				go func() {
					for event := range t.Events(done) {
						events <- event
					}
				}()
			case <-done:
				return
			}
		}
	}()
	return events
}

func (e *Multi) Add(t *Torrent) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	id := uuid.NewV4().String()
	e.eventerMap[id] = t
	go func() {
		<-t.Closed()
		e.mutex.Lock()
		delete(e.eventerMap, id)
		e.mutex.Unlock()
	}()
	for _, eventerChan := range e.eventerChans {
		eventerChan <- t
	}
}

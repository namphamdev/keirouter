// Package usagehub provides a lightweight pub/sub hub that notifies
// subscribers when new usage records are inserted. The gateway uses this
// to push SSE events to the Quota and Usage pages for near-real-time
// dashboard updates without polling.
package usagehub

import (
	"sync"
)

// Event is a minimal usage event broadcast to subscribers after a usage
// record is persisted. It carries just enough data for the frontend to
// know which query keys to invalidate.
type Event struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	AccountID string `json:"account_id"`
	Tokens    int    `json:"tokens"`
}

// Listener receives usage events.
type Listener struct {
	OnEvent func(Event)
}

// Hub manages subscribers and broadcasts usage events.
type Hub struct {
	mu        sync.RWMutex
	listeners map[*Listener]struct{}
}

// New creates a Hub.
func New() *Hub {
	return &Hub{
		listeners: make(map[*Listener]struct{}),
	}
}

// Subscribe registers a listener. Call Unsubscribe when done.
func (h *Hub) Subscribe(l *Listener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners[l] = struct{}{}
}

// Unsubscribe removes a listener.
func (h *Hub) Unsubscribe(l *Listener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.listeners, l)
}

// Publish broadcasts an event to all subscribers. It copies the listener
// set under the read lock so callbacks run without holding the lock.
func (h *Hub) Publish(ev Event) {
	h.mu.RLock()
	snapshot := make([]*Listener, 0, len(h.listeners))
	for l := range h.listeners {
		snapshot = append(snapshot, l)
	}
	h.mu.RUnlock()

	for _, l := range snapshot {
		l.OnEvent(ev)
	}
}

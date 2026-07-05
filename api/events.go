package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"time"
)

// EventBus is a simple publish/subscribe hub for Docker-compatible events.
type EventBus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a channel that receives published events and an
// unsubscribe function. The channel is buffered (64) so slow consumers
// don't block publishers — events are dropped if the buffer is full.
func (b *EventBus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Publish fans out an event to all subscribers without blocking.
func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// publishEvent is a convenience method on Server to emit a Docker-compatible event.
func (s *Server) publishEvent(eventType, action, actorID string, attrs map[string]string) {
	now := time.Now()
	s.events.Publish(Event{
		Type:     eventType,
		Action:   action,
		Actor:    EventActor{ID: actorID, Attributes: attrs},
		Time:     now.Unix(),
		TimeNano: now.UnixNano(),
		Scope:    "local",
	})
}

// handleEvents streams Docker-compatible events as newline-delimited JSON.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	var filters map[string][]string
	if raw := r.URL.Query().Get("filters"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &filters)
	}

	ch, unsub := s.events.Subscribe()
	defer unsub()

	// Flush through ResponseController, not a w.(http.Flusher) assertion: the
	// logging middleware wraps w in loggingResponseWriter, whose method set
	// has no Flush, so the assertion fails and events sit buffered until the
	// 4KB chunk fills — clients hang. ResponseController follows Unwrap().
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()

	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if !matchFilters(evt, filters) {
				continue
			}
			_ = enc.Encode(evt)
			_ = rc.Flush()
		}
	}
}

// matchFilters checks whether an event passes the Docker-style filters.
// Supported filter keys: "type" (event Type) and "event" (event Action).
func matchFilters(e Event, filters map[string][]string) bool {
	if len(filters) == 0 {
		return true
	}
	if types, ok := filters["type"]; ok && len(types) > 0 {
		if !slices.Contains(types, e.Type) {
			return false
		}
	}
	if actions, ok := filters["event"]; ok && len(actions) > 0 {
		if !slices.Contains(actions, e.Action) {
			return false
		}
	}
	return true
}

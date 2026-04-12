package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEventBusPublishSubscribe(t *testing.T) {
	bus := NewEventBus()
	ch, unsub := bus.Subscribe()
	defer unsub()

	want := Event{Type: "container", Action: "start", Actor: EventActor{ID: "abc123"}}
	bus.Publish(want)

	select {
	case got := <-ch:
		if got.Type != want.Type || got.Action != want.Action || got.Actor.ID != want.Actor.ID {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1, unsub1 := bus.Subscribe()
	defer unsub1()
	ch2, unsub2 := bus.Subscribe()
	defer unsub2()

	evt := Event{Type: "image", Action: "pull"}
	bus.Publish(evt)

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Type != evt.Type || got.Action != evt.Action {
				t.Fatalf("subscriber %d: got %+v, want %+v", i, got, evt)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	ch, unsub := bus.Subscribe()
	unsub()

	bus.Publish(Event{Type: "container", Action: "stop"})

	select {
	case evt := <-ch:
		t.Fatalf("received event after unsubscribe: %+v", evt)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestEventBusNonBlocking(t *testing.T) {
	bus := NewEventBus()
	_, unsub := bus.Subscribe() // subscribe but never read
	defer unsub()

	// Should not block even though nobody is reading
	done := make(chan struct{})
	go func() {
		for i := range 100 {
			bus.Publish(Event{Type: "container", Action: "start", Actor: EventActor{ID: string(rune('0' + i%10))}})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on full subscriber channel")
	}
}

func TestMatchFilters(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		filters map[string][]string
		want    bool
	}{
		{"nil filters", Event{Type: "container", Action: "start"}, nil, true},
		{"empty filters", Event{Type: "container", Action: "start"}, map[string][]string{}, true},
		{"type match", Event{Type: "container", Action: "start"}, map[string][]string{"type": {"container"}}, true},
		{"type mismatch", Event{Type: "image", Action: "pull"}, map[string][]string{"type": {"container"}}, false},
		{"action match", Event{Type: "container", Action: "start"}, map[string][]string{"event": {"start", "stop"}}, true},
		{"action mismatch", Event{Type: "container", Action: "kill"}, map[string][]string{"event": {"start", "stop"}}, false},
		{"both match", Event{Type: "container", Action: "start"}, map[string][]string{"type": {"container"}, "event": {"start"}}, true},
		{"type match action mismatch", Event{Type: "container", Action: "kill"}, map[string][]string{"type": {"container"}, "event": {"start"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchFilters(tt.event, tt.filters); got != tt.want {
				t.Errorf("matchFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleEventsHTTP(t *testing.T) {
	bus := NewEventBus()
	s := &Server{events: bus, mux: http.NewServeMux()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/events", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	// Run handler in background
	done := make(chan struct{})
	go func() {
		s.handleEvents(rec, req)
		close(done)
	}()

	// Give the handler time to subscribe and flush headers
	time.Sleep(50 * time.Millisecond)

	// Publish two events
	bus.Publish(Event{
		Type: "container", Action: "start",
		Actor: EventActor{ID: "c1", Attributes: map[string]string{"name": "test"}},
		Time: 1000, TimeNano: 1000000000, Scope: "local",
	})
	bus.Publish(Event{
		Type: "container", Action: "stop",
		Actor: EventActor{ID: "c1"},
		Time: 1001, TimeNano: 1001000000, Scope: "local",
	})

	// Give time for events to be written
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Parse NDJSON output
	scanner := bufio.NewScanner(rec.Body)
	var events []Event
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("failed to parse event JSON: %v\nline: %s", err, scanner.Text())
		}
		events = append(events, e)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Action != "start" {
		t.Errorf("event[0].Action = %q, want %q", events[0].Action, "start")
	}
	if events[1].Action != "stop" {
		t.Errorf("event[1].Action = %q, want %q", events[1].Action, "stop")
	}
}

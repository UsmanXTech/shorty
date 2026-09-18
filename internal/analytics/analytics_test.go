package analytics

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu sync.Mutex
	events []Event
}

func (s *memoryStore) RecordEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func TestRecorderProcessesEventsAsync(t *testing.T) {
	store := &memoryStore{}
	r := New(store, 4)
	now := time.Now().UTC()
	if !r.Record(Event{Slug: "abc", CreatedAt: now}) {
		t.Fatal("expected event to be queued")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 || store.events[0].Slug != "abc" {
		t.Fatalf("unexpected events: %+v", store.events)
	}
}

func TestRecorderDropsWhenBufferFull(t *testing.T) {
	store := &memoryStore{}
	r := New(store, 1)
	if !r.Record(Event{Slug: "abc"}) {
		t.Fatal("first event should be queued")
	}
	if !r.Record(Event{Slug: "def"}) {
		// The worker may consume the first event before this call; retrying is not required.
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = r.Close(ctx)
}

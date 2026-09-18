package analytics

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	Slug      string
	CreatedAt time.Time
	Referrer  string
	UserAgent string
}

type Store interface {
	RecordEvent(Event) error
}

type Recorder struct {
	store Store
	queue chan Event
	wg    sync.WaitGroup
	once  sync.Once
}

func New(store Store, bufferSize int) *Recorder {
	if bufferSize < 1 {
		bufferSize = 256
	}
	r := &Recorder{store: store, queue: make(chan Event, bufferSize)}
	r.wg.Add(1)
	go r.worker()
	return r
}

func (r *Recorder) Record(event Event) bool {
	select {
	case r.queue <- event:
		return true
	default:
		return false
	}
}

func (r *Recorder) Close(ctx context.Context) error {
	r.once.Do(func() { close(r.queue) })

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Recorder) worker() {
	defer r.wg.Done()
	for event := range r.queue {
		_ = r.store.RecordEvent(event)
	}
}

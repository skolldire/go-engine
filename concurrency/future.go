package concurrency

import (
	"context"
	"sync"
	"time"
)

type Job[I any] struct {
	ID     string
	Key    string
	Weight int64
	Value  I
}

type Task[I, O any] func(context.Context, I) (O, error)

type Result[O any] struct {
	ID         string
	Key        string
	Sequence   uint64
	Value      O
	Err        error
	State      State
	AcceptedAt time.Time
	StartedAt  time.Time
	FinishedAt time.Time
}

type Future[O any] struct {
	mu     sync.Mutex
	done   chan struct{}
	result Result[O]
	cancel func(error)
}

func (f *Future[O]) Done() <-chan struct{} {
	if f == nil {
		return nil
	}
	return f.done
}

func (f *Future[O]) Wait(ctx context.Context) (Result[O], error) {
	var zero Result[O]
	if f == nil || f.done == nil || ctx == nil {
		return zero, invalid("future", "requires initialized future and non-nil context")
	}
	select {
	case <-f.done:
		return f.result, nil
	default:
	}
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-f.done:
		return f.result, nil
	}
}

func (f *Future[O]) Cancel(cause error) {
	if f == nil {
		return
	}
	if cause == nil {
		cause = context.Canceled
	}
	f.mu.Lock()
	cancel := f.cancel
	f.mu.Unlock()
	if cancel != nil {
		cancel(cause)
	}
}

// Package concurrency provides bounded, context-aware streaming primitives.
// It has no dependency on the engine or on third-party modules.
//
// Callers own input channels. Each primitive owns and closes its output.
// Consumers that stop reading must cancel the shared context. Functions passed
// to Map must honor context cancellation: Go cannot forcibly stop user code.
package concurrency

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidConfig = errors.New("invalid concurrency configuration")
	ErrPanic         = errors.New("task panicked")
)

// Config bounds execution and output buffering. Zero Buffer is unbuffered.
// Workers must be positive; durations and Buffer must be nonnegative.
// TaskTimeout requests cooperative cancellation; it never detaches a task.
type Config struct {
	Workers     int
	Buffer      int
	TaskTimeout time.Duration
}

// Result carries a task's value or error. Map emits results in completion order.
type Result[T any] struct {
	Value T
	Err   error
}

// Map distributes input across a fixed worker pool and merges the results.
// A slow consumer applies backpressure once Buffer results are queued. Task
// errors and recovered panics are emitted as results; other tasks continue.
// Cancellation stops admission and delivery; queued results may be discarded.
// The output closes only after all workers have returned.
func Map[I, O any](ctx context.Context, input <-chan I, cfg Config, fn func(context.Context, I) (O, error)) (<-chan Result[O], error) {
	if ctx == nil || input == nil || fn == nil || cfg.Workers <= 0 || cfg.Buffer < 0 || cfg.TaskTimeout < 0 {
		return nil, ErrInvalidConfig
	}
	output := make(chan Result[O], cfg.Buffer)
	var wg sync.WaitGroup
	wg.Add(cfg.Workers)
	for range cfg.Workers {
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				var value I
				var ok bool
				select {
				case <-ctx.Done():
					return
				case value, ok = <-input:
					if !ok {
						return
					}
				}
				if ctx.Err() != nil {
					return
				}
				result := invoke(ctx, cfg.TaskTimeout, value, fn)
				select {
				case <-ctx.Done():
					return
				case output <- result:
				}
			}
		}()
	}
	go func() { wg.Wait(); close(output) }()
	return output, nil
}

func invoke[I, O any](ctx context.Context, timeout time.Duration, value I, fn func(context.Context, I) (O, error)) (result Result[O]) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	defer func() {
		if p := recover(); p != nil {
			result.Err = fmt.Errorf("%w: %v", ErrPanic, p)
		}
	}()
	result.Value, result.Err = fn(ctx, value)
	if result.Err == nil {
		result.Err = ctx.Err()
	}
	return result
}

// From streams a finite slice without copying it. Do not mutate the slice until
// the output closes. Cancel ctx when abandoning the stream.
func From[T any](ctx context.Context, values ...T) <-chan T {
	output := make(chan T)
	go func() {
		defer close(output)
		for _, value := range values {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case output <- value:
			}
		}
	}()
	return output
}

// FanIn merges streams without imposing an ordering. Nil inputs are ignored.
// It uses one forwarding goroutine per non-nil input and a bounded output.
func FanIn[T any](ctx context.Context, buffer int, inputs ...<-chan T) (<-chan T, error) {
	if ctx == nil || buffer < 0 {
		return nil, ErrInvalidConfig
	}
	output := make(chan T, buffer)
	var wg sync.WaitGroup
	for _, input := range inputs {
		if input == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case value, ok := <-input:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case output <- value:
					}
				}
			}
		}()
	}
	go func() { wg.Wait(); close(output) }()
	return output, nil
}

// Limiter caps concurrent operations across multiple pools or callers.
// Construct it with NewLimiter; a Limiter must not be copied.
type Limiter struct{ slots chan struct{} }

func NewLimiter(limit int) (*Limiter, error) {
	if limit <= 0 {
		return nil, ErrInvalidConfig
	}
	return &Limiter{slots: make(chan struct{}, limit)}, nil
}

// Acquire blocks with backpressure until a slot is available or ctx ends.
// The returned release function is safe to call more than once.
func (l *Limiter) Acquire(ctx context.Context) (release func(), err error) {
	if ctx == nil || l == nil || l.slots == nil {
		return nil, ErrInvalidConfig
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.slots <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-l.slots
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { <-l.slots }) }, nil
}

// FanOut shares input items among n output streams, delivering each item once.
// This is work distribution, not broadcast. Each output buffers at most buffer
// items and has one forwarder; every output must be consumed or ctx cancelled.
func FanOut[T any](ctx context.Context, input <-chan T, n, buffer int) ([]<-chan T, error) {
	if ctx == nil || input == nil || n <= 0 || buffer < 0 {
		return nil, ErrInvalidConfig
	}
	outputs := make([]<-chan T, n)
	for i := range n {
		output := make(chan T, buffer)
		outputs[i] = output
		go func() {
			defer close(output)
			for {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case value, ok := <-input:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case output <- value:
					}
				}
			}
		}()
	}
	return outputs, nil
}

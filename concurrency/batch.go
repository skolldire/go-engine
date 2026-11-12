package concurrency

import (
	"context"
	"time"
)

// Batch groups up to size items and flushes the final partial batch on input
// closure. maxWait bounds the age of a partial batch before it is offered to
// the consumer; a slow consumer still applies backpressure. Zero disables the
// timer. Cancellation discards the pending batch. Every emitted slice has
// independent storage. Memory is bounded by size * (buffer + 1) items, excluding
// the caller's input buffer and batches already owned by the consumer.
func Batch[T any](ctx context.Context, input <-chan T, size int, maxWait time.Duration, buffer int) (<-chan []T, error) {
	if ctx == nil || input == nil || size <= 0 || maxWait < 0 || buffer < 0 {
		return nil, ErrInvalidConfig
	}
	output := make(chan []T, buffer)
	go func() {
		defer close(output)
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		var tick <-chan time.Time
		var batch []T
		flush := func() bool {
			select {
			case <-ctx.Done():
				return false
			case output <- batch:
			}
			batch = nil
			timer.Stop()
			tick = nil
			return true
		}
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-tick:
				if !flush() {
					return
				}
			case value, ok := <-input:
				if !ok {
					if len(batch) > 0 {
						flush()
					}
					return
				}
				if len(batch) == 0 {
					batch = make([]T, 0, size)
					if maxWait > 0 {
						timer.Reset(maxWait)
						tick = timer.C
					}
				}
				batch = append(batch, value)
				if len(batch) == size && !flush() {
					return
				}
			}
		}
	}()
	return output, nil
}

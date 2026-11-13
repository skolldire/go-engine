package concurrency

import (
	"context"
	"time"
)

type BatchConfig struct {
	Size           int           `json:"size" yaml:"size" mapstructure:"size"`
	MaxWait        time.Duration `json:"max_wait" yaml:"max_wait" mapstructure:"max_wait"`
	Buffer         int           `json:"buffer" yaml:"buffer" mapstructure:"buffer"`
	MaxBatchWeight int64         `json:"max_batch_weight" yaml:"max_batch_weight" mapstructure:"max_batch_weight"`
}

func Batch[T any](ctx context.Context, input <-chan T, cfg BatchConfig, weight func(T) int64) (<-chan StreamResult[[]T], error) {
	if ctx == nil || input == nil || cfg.Size <= 0 || cfg.MaxWait < 0 || cfg.Buffer < 0 || cfg.MaxBatchWeight <= 0 || weight == nil {
		return nil, invalid("batch", "invalid context, input, weight or limits")
	}
	out := make(chan StreamResult[[]T], cfg.Buffer)
	go func() {
		defer close(out)
		var timer *time.Timer
		var tick <-chan time.Time
		if cfg.MaxWait > 0 {
			timer = time.NewTimer(cfg.MaxWait)
			timer.Stop()
			defer timer.Stop()
		}
		var batch []T
		var total int64
		emit := func(result StreamResult[[]T]) bool {
			select {
			case <-ctx.Done():
				return false
			case out <- result:
				return true
			}
		}
		flush := func() bool {
			if !emit(StreamResult[[]T]{Value: batch}) {
				return false
			}
			batch = nil
			total = 0
			tick = nil
			if timer != nil {
				timer.Stop()
			}
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
			case v, ok := <-input:
				if !ok {
					if len(batch) > 0 {
						flush()
					}
					return
				}
				w := weight(v)
				if w <= 0 {
					emit(StreamResult[[]T]{Err: invalid("weight", "must be positive")})
					return
				}
				if w > cfg.MaxBatchWeight {
					emit(StreamResult[[]T]{Err: ErrJobTooLarge})
					return
				}
				if len(batch) > 0 && w > cfg.MaxBatchWeight-total {
					if !flush() {
						return
					}
				}
				if len(batch) == 0 {
					if timer != nil {
						timer.Reset(cfg.MaxWait)
						tick = timer.C
					}
				}
				batch = append(batch, v)
				total += w
				if len(batch) == cfg.Size || total == cfg.MaxBatchWeight {
					if !flush() {
						return
					}
				}
			}
		}
	}()
	return out, nil
}

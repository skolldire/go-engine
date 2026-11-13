package concurrency

import (
	"context"
	"errors"
	"sync"
	"time"
)

type StreamConfig struct {
	Workers     int           `json:"workers" yaml:"workers" mapstructure:"workers"`
	Buffer      int           `json:"buffer" yaml:"buffer" mapstructure:"buffer"`
	TaskTimeout time.Duration `json:"task_timeout" yaml:"task_timeout" mapstructure:"task_timeout"`
}

type StreamResult[T any] struct {
	Value T
	Err   error
}

func Map[I, O any](ctx context.Context, input <-chan I, cfg StreamConfig, task Task[I, O]) (<-chan StreamResult[O], error) {
	if ctx == nil || input == nil || task == nil || cfg.Workers <= 0 || cfg.Buffer < 0 || cfg.TaskTimeout < 0 {
		return nil, invalid("map", "invalid context, input, task or limits")
	}
	out := make(chan StreamResult[O], cfg.Buffer)
	var wg sync.WaitGroup
	wg.Add(cfg.Workers)
	for range cfg.Workers {
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				var in I
				var ok bool
				select {
				case <-ctx.Done():
					return
				case in, ok = <-input:
					if !ok {
						return
					}
				}
				if ctx.Err() != nil {
					return
				}
				result := streamInvoke(ctx, cfg.TaskTimeout, in, task)
				select {
				case <-ctx.Done():
					return
				case out <- result:
				}
			}
		}()
	}
	go func() { wg.Wait(); close(out) }()
	return out, nil
}
func streamInvoke[I, O any](ctx context.Context, timeout time.Duration, in I, task Task[I, O]) StreamResult[O] {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	out, pe, err := executeTask(ctx, in, task)
	if pe != nil {
		return StreamResult[O]{Value: out, Err: errors.Join(pe, context.Cause(ctx))}
	}
	return StreamResult[O]{Value: out, Err: errors.Join(err, context.Cause(ctx))}
}

func From[T any](ctx context.Context, values ...T) (<-chan T, error) {
	if ctx == nil {
		return nil, invalid("context", "must not be nil")
	}
	out := make(chan T)
	go func() {
		defer close(out)
		for _, v := range values {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- v:
			}
		}
	}()
	return out, nil
}

func FanIn[T any](ctx context.Context, buffer int, inputs ...<-chan T) (<-chan T, error) {
	if ctx == nil || buffer < 0 {
		return nil, invalid("fan_in", "non-nil context and nonnegative buffer required")
	}
	out := make(chan T, buffer)
	var wg sync.WaitGroup
	for _, in := range inputs {
		if in == nil {
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
				case v, ok := <-in:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
		}()
	}
	go func() { wg.Wait(); close(out) }()
	return out, nil
}

func FanOut[T any](ctx context.Context, input <-chan T, count, buffer int) ([]<-chan T, error) {
	if ctx == nil || input == nil || count <= 0 || buffer < 0 {
		return nil, invalid("fan_out", "invalid context, input or limits")
	}
	streams := make([]<-chan T, count)
	for i := range count {
		out := make(chan T, buffer)
		streams[i] = out
		go func() {
			defer close(out)
			for {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case v, ok := <-input:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
		}()
	}
	return streams, nil
}

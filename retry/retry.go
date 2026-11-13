package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

type Decision string

const (
	DecisionStop      Decision = "stop"
	DecisionRetry     Decision = "retry"
	DecisionReconcile Decision = "reconcile"
	JitterFull                 = "full"
	JitterNone                 = "none"
)

var (
	ErrInvalidConfig          = errors.New("invalid retry configuration")
	ErrInvalidCall            = errors.New("invalid retry call")
	ErrInvalidDecision        = errors.New("invalid retry classifier decision")
	ErrReconciliationRequired = errors.New("retry reconciliation required")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "retry config: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	TotalTimeout   time.Duration
	Multiplier     float64
	Jitter         string
}

type Classifier func(error) Decision

func DefaultConfig() Config {
	return Config{InitialBackoff: 100 * time.Millisecond, MaxBackoff: 10 * time.Second, Multiplier: 2, Jitter: JitterFull}
}
func (cfg Config) Validate() error {
	switch {
	case cfg.MaxRetries < 0:
		return &ConfigError{"MaxRetries", "must be nonnegative"}
	case cfg.InitialBackoff <= 0:
		return &ConfigError{"InitialBackoff", "must be positive"}
	case cfg.MaxBackoff < cfg.InitialBackoff:
		return &ConfigError{"MaxBackoff", "must be at least InitialBackoff"}
	case cfg.TotalTimeout < 0:
		return &ConfigError{"TotalTimeout", "must be nonnegative"}
	case math.IsNaN(cfg.Multiplier) || math.IsInf(cfg.Multiplier, 0) || cfg.Multiplier < 1:
		return &ConfigError{"Multiplier", "must be finite and at least one"}
	case cfg.Jitter != JitterFull && cfg.Jitter != JitterNone:
		return &ConfigError{"Jitter", "must be full or none"}
	}
	return nil
}

func Do[T any](ctx context.Context, cfg Config, classify Classifier, operation func(context.Context) (T, error)) (T, error) {
	var value T
	if ctx == nil {
		return value, fmt.Errorf("ctx: %w", ErrInvalidCall)
	}
	if err := cfg.Validate(); err != nil {
		return value, err
	}
	if operation == nil {
		return value, fmt.Errorf("operation: %w", ErrInvalidCall)
	}
	if classify == nil && cfg.MaxRetries > 0 {
		return value, fmt.Errorf("classify: %w", ErrInvalidCall)
	}
	if err := cancellation(ctx, nil); err != nil {
		return value, err
	}
	if cfg.TotalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.TotalTimeout)
		defer cancel()
	}
	var last error
	remaining := cfg.MaxRetries
	for {
		if err := cancellation(ctx, last); err != nil {
			return value, err
		}
		value, last = operation(ctx)
		if last == nil {
			return value, nil
		}
		if classify == nil {
			return value, last
		}
		switch classify(last) {
		case DecisionStop:
			return value, last
		case DecisionReconcile:
			return value, errors.Join(ErrReconciliationRequired, last)
		case DecisionRetry:
		default:
			return value, errors.Join(ErrInvalidDecision, last)
		}
		if remaining == 0 {
			return value, last
		}
		if err := cancellation(ctx, last); err != nil {
			return value, err
		}
		delay := backoff(cfg, cfg.MaxRetries-remaining)
		if cfg.Jitter == JitterFull {
			delay = fullJitter(delay, rand.Uint64N)
		}
		if deadline, ok := ctx.Deadline(); ok && time.Now().Add(delay).After(deadline) {
			return value, last
		}
		if err := wait(ctx, delay); err != nil {
			return value, cancellation(ctx, last)
		}
		remaining--
	}
}

func cancellation(ctx context.Context, last error) error {
	err := ctx.Err()
	if err == nil {
		deadline, ok := ctx.Deadline()
		if !ok || time.Now().Before(deadline) {
			return nil
		}
		err = context.DeadlineExceeded
	}
	return errors.Join(err, context.Cause(ctx), last)
}
func backoff(cfg Config, retry int) time.Duration {
	if retry == 0 || cfg.Multiplier == 1 {
		return cfg.InitialBackoff
	}
	duration := float64(cfg.InitialBackoff) * math.Pow(cfg.Multiplier, float64(retry))
	if duration >= float64(cfg.MaxBackoff) {
		return cfg.MaxBackoff
	}
	return time.Duration(duration)
}
func fullJitter(cap time.Duration, draw func(uint64) uint64) time.Duration {
	if cap <= 0 {
		return 0
	}
	//nolint:gosec // G115: draw(n) is Uint64N(n); its result is at most cap (a positive int64).
	return time.Duration(draw(uint64(cap) + 1))
}
func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

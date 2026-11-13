package resilience

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/retry"
)

const OrderRetryThenBreaker = "retry_then_breaker"

var (
	ErrRejected         = errors.New("resilience breaker rejected call")
	ErrInvalidConfig    = errors.New("invalid resilience configuration")
	ErrInvalidCall      = errors.New("invalid resilience call")
	ErrAdmission        = errors.New("resilience admission failed")
	ErrAdmissionTimeout = errors.New("resilience admission timeout")
	ErrAttemptTimeout   = errors.New("resilience attempt timeout")
	ErrTotalTimeout     = errors.New("resilience total timeout")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "resilience config: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	Order            string
	TotalTimeout     time.Duration
	AttemptTimeout   time.Duration
	AdmissionTimeout time.Duration
	Retry            retry.Config
	Weight           int64
	Tokens           int
}
type RateLimiter interface {
	Wait(context.Context, int) error
}
type Limiter interface {
	Acquire(context.Context, int64) (func(), error)
}
type Breaker interface {
	Run(context.Context, func(context.Context) error) error
}
type Dependencies struct {
	Classify    retry.Classifier
	Breaker     Breaker
	Limiter     Limiter
	RateLimiter RateLimiter
}

func DefaultConfig() Config {
	return Config{Order: OrderRetryThenBreaker, Retry: retry.DefaultConfig()}
}
func (cfg Config) Validate() error {
	switch {
	case cfg.Order != "" && cfg.Order != OrderRetryThenBreaker:
		return &ConfigError{"Order", "unsupported policy"}
	case cfg.TotalTimeout < 0:
		return &ConfigError{"TotalTimeout", "must be nonnegative"}
	case cfg.AttemptTimeout < 0:
		return &ConfigError{"AttemptTimeout", "must be nonnegative"}
	case cfg.AdmissionTimeout < 0:
		return &ConfigError{"AdmissionTimeout", "must be nonnegative"}
	}
	if err := cfg.Retry.Validate(); err != nil {
		field := "Retry"
		var ce *retry.ConfigError
		if errors.As(err, &ce) {
			field += "." + ce.Field
		}
		return errors.Join(&ConfigError{field, "invalid retry configuration"}, err)
	}
	switch {
	case cfg.Retry.TotalTimeout != 0:
		return &ConfigError{"Retry.TotalTimeout", "total budget belongs to resilience"}
	case cfg.Weight < 0:
		return &ConfigError{"Weight", "must be nonnegative"}
	case cfg.Tokens < 0:
		return &ConfigError{"Tokens", "must be nonnegative"}
	}
	return nil
}
func Wrap[T any](operation client.Operation[T], cfg Config, deps Dependencies) (client.Operation[T], error) {
	if operation == nil {
		return nil, fmt.Errorf("operation: %w", ErrInvalidCall)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch {
	case nilPointer(deps.Breaker):
		return nil, &ConfigError{"Breaker", "must not be a nil pointer"}
	case deps.Breaker != nil && cfg.Order == "":
		return nil, &ConfigError{"Order", "required with Breaker"}
	case cfg.Retry.MaxRetries > 0 && deps.Classify == nil:
		return nil, &ConfigError{"Classify", "required when retries enabled"}
	case nilPointer(deps.Limiter):
		return nil, &ConfigError{"Limiter", "must not be a nil pointer"}
	case deps.Limiter != nil && cfg.Weight == 0:
		return nil, &ConfigError{"Weight", "must be positive with Limiter"}
	case deps.Limiter == nil && cfg.Weight != 0:
		return nil, &ConfigError{"Weight", "requires Limiter"}
	case nilPointer(deps.RateLimiter):
		return nil, &ConfigError{"RateLimiter", "must not be a nil pointer"}
	case deps.RateLimiter != nil && cfg.Tokens == 0:
		return nil, &ConfigError{"Tokens", "must be positive with RateLimiter"}
	case deps.RateLimiter == nil && cfg.Tokens != 0:
		return nil, &ConfigError{"Tokens", "requires RateLimiter"}
	}
	classify := func(err error) retry.Decision {
		if errors.Is(err, ErrAdmission) || errors.Is(err, ErrRejected) || deps.Classify == nil {
			return retry.DecisionStop
		}
		return deps.Classify(err)
	}
	ioCall := func(ctx context.Context) (T, error) {
		parent := ctx
		ctx, cancel := budget(ctx, cfg.AttemptTimeout, ErrAttemptTimeout)
		defer cancel()
		value, err := operation(ctx)
		settleBudget(err, ctx, parent, cfg.AttemptTimeout)
		return value, enrichContext(err, ctx)
	}
	attempt := func(ctx context.Context) (T, error) {
		var zero T
		release, err := admit(ctx, cfg, deps)
		if err != nil {
			return zero, err
		}
		if release != nil {
			defer release()
		}
		if deps.Breaker != nil {
			var value T
			entered := false
			err := deps.Breaker.Run(ctx, func(attemptContext context.Context) error {
				entered = true
				var err error
				value, err = ioCall(attemptContext)
				return err
			})
			if err != nil && !entered {
				return value, errors.Join(ErrRejected, err)
			}
			return value, err
		}
		return ioCall(ctx)
	}
	return func(ctx context.Context) (T, error) {
		var zero T
		if ctx == nil {
			return zero, fmt.Errorf("ctx: %w", ErrInvalidCall)
		}
		parent := ctx
		ctx, cancel := budget(ctx, cfg.TotalTimeout, ErrTotalTimeout)
		defer cancel()
		value, err := retry.Do(ctx, cfg.Retry, classify, attempt)
		if cfg.TotalTimeout > 0 && !errors.Is(err, ErrAttemptTimeout) && !errors.Is(err, ErrAdmissionTimeout) {
			settleBudget(err, ctx, parent, cfg.TotalTimeout)
			err = enrichContext(err, ctx)
		}
		return value, err
	}, nil
}
func budget(ctx context.Context, duration time.Duration, cause error) (context.Context, context.CancelFunc) {
	if duration == 0 {
		return ctx, func() {}
	}
	return context.WithTimeoutCause(ctx, duration, cause)
}
func admit(ctx context.Context, cfg Config, deps Dependencies) (func(), error) {
	if deps.RateLimiter == nil && deps.Limiter == nil {
		return nil, nil
	}
	parent := ctx
	ctx, cancel := budget(ctx, cfg.AdmissionTimeout, ErrAdmissionTimeout)
	defer cancel()
	if deps.RateLimiter != nil {
		if err := deps.RateLimiter.Wait(ctx, cfg.Tokens); err != nil {
			settleBudget(err, ctx, parent, cfg.AdmissionTimeout)
			return nil, admissionError("ratelimit", err, ctx)
		}
	}
	if deps.Limiter != nil {
		release, err := deps.Limiter.Acquire(ctx, cfg.Weight)
		if err != nil {
			settleBudget(err, ctx, parent, cfg.AdmissionTimeout)
			return nil, admissionError("concurrency", err, ctx)
		}
		return release, nil
	}
	return nil, nil
}
func admissionError(stage string, err error, ctx context.Context) error {
	return fmt.Errorf("%s: %w", stage, errors.Join(ErrAdmission, err, context.Cause(ctx)))
}
func enrichContext(err error, ctx context.Context) error {
	if err == nil || ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
		return err
	}
	cause := context.Cause(ctx)
	if errors.Is(cause, ErrAttemptTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if cause == nil || errors.Is(err, cause) {
		return err
	}
	return errors.Join(err, cause)
}

func settleBudget(err error, ctx, parent context.Context, duration time.Duration) {
	if duration == 0 || !errors.Is(err, context.DeadlineExceeded) || context.Cause(ctx) != nil {
		return
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Now().Before(deadline) {
		return
	}
	if parentDeadline, ok := parent.Deadline(); ok && !deadline.Before(parentDeadline) {
		return
	}
	<-ctx.Done()
}

func nilPointer(value any) bool {
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

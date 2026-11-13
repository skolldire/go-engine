package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)

var (
	ErrInvalidConfig = errors.New("invalid circuitbreaker configuration")
	ErrInvalidCall   = errors.New("invalid circuitbreaker call")
	ErrOpen          = errors.New("circuitbreaker is open")
	ErrProbeLimit    = errors.New("circuitbreaker probe limit reached")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "circuitbreaker config: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	Name             string
	RequestThreshold uint64
	FailureRate      float64
	Interval         time.Duration
	OpenTimeout      time.Duration
	HalfOpenMax      uint64
}
type Dependencies struct{ Classify func(error) bool }
type Counts struct {
	State     State
	Requests  uint64
	InFlight  uint64
	Successes uint64
	Failures  uint64
	Ignored   uint64
	Panics    uint64
}
type generation struct {
	counts  Counts
	expires time.Time
}
type Breaker struct {
	mu       sync.Mutex
	cfg      Config
	classify func(error) bool
	current  *generation
}

type outcome uint8

const (
	success outcome = iota
	failure
	ignored
	abnormal
)

func DefaultConfig() Config {
	return Config{RequestThreshold: 5, FailureRate: 0.5, Interval: time.Minute, OpenTimeout: 30 * time.Second, HalfOpenMax: 1}
}
func (cfg Config) Validate() error {
	switch {
	case cfg.Name == "":
		return &ConfigError{"Name", "must be nonempty"}
	case cfg.RequestThreshold == 0:
		return &ConfigError{"RequestThreshold", "must be positive"}
	case math.IsNaN(cfg.FailureRate) || math.IsInf(cfg.FailureRate, 0) || cfg.FailureRate <= 0 || cfg.FailureRate > 1:
		return &ConfigError{"FailureRate", "must be finite and in (0,1]"}
	case cfg.Interval <= 0:
		return &ConfigError{"Interval", "must be positive"}
	case cfg.OpenTimeout <= 0:
		return &ConfigError{"OpenTimeout", "must be positive"}
	case cfg.HalfOpenMax == 0:
		return &ConfigError{"HalfOpenMax", "must be positive"}
	}
	return nil
}
func New(cfg Config, deps Dependencies) (*Breaker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Classify == nil {
		return nil, &ConfigError{"Classify", "must be nonnil"}
	}
	b := &Breaker{cfg: cfg, classify: deps.Classify}
	b.start(StateClosed, time.Now())
	return b, nil
}
func Execute[T any](ctx context.Context, b *Breaker, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, fmt.Errorf("ctx: %w", ErrInvalidCall)
	}
	if b == nil {
		return zero, fmt.Errorf("breaker: %w", ErrInvalidCall)
	}
	if operation == nil {
		return zero, fmt.Errorf("operation: %w", ErrInvalidCall)
	}
	var value T
	err := b.run(ctx, func(ctx context.Context) error {
		var err error
		value, err = operation(ctx)
		return err
	})
	return value, err
}
func (b *Breaker) Run(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("ctx: %w", ErrInvalidCall)
	}
	if b == nil {
		return fmt.Errorf("breaker: %w", ErrInvalidCall)
	}
	if fn == nil {
		return fmt.Errorf("fn: %w", ErrInvalidCall)
	}
	return b.run(ctx, fn)
}
func (b *Breaker) run(ctx context.Context, fn func(context.Context) error) error {
	g, err := b.admit(ctx)
	if err != nil {
		return err
	}
	finished := false
	result := abnormal
	defer func() {
		if !finished {
			result = abnormal
		}
		b.finish(g, result)
	}()
	err = fn(ctx)
	result = success
	if err != nil {
		result = ignored
		if b.classify(err) {
			result = failure
		}
	}
	finished = true
	return err
}
func (b *Breaker) State() State { return b.Stats().State }
func (b *Breaker) Stats() Counts {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.advance(time.Now())
	return b.current.counts
}
func (b *Breaker) start(state State, now time.Time) {
	g := &generation{counts: Counts{State: state}}
	switch state {
	case StateClosed:
		g.expires = now.Add(b.cfg.Interval)
	case StateOpen:
		g.expires = now.Add(b.cfg.OpenTimeout)
	}
	b.current = g
}
func (b *Breaker) advance(now time.Time) {
	g := b.current
	if g.expires.IsZero() || now.Before(g.expires) {
		return
	}
	switch g.counts.State {
	case StateClosed:
		b.start(StateClosed, now)
	case StateOpen:
		b.start(StateHalfOpen, now)
	}
}
func (b *Breaker) rotate(now time.Time) {
	g := b.current
	b.start(g.counts.State, now)
	if g.counts.State == StateOpen {
		b.current.expires = g.expires
	}
}
func (b *Breaker) admit(ctx context.Context) (*generation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	now := time.Now()
	b.advance(now)
	c := &b.current.counts
	if c.State == StateOpen {
		return nil, ErrOpen
	}
	if c.State == StateHalfOpen && c.InFlight >= b.cfg.HalfOpenMax {
		return nil, ErrProbeLimit
	}
	if c.Requests == math.MaxUint64 || c.InFlight == math.MaxUint64 {
		b.rotate(now)
		c = &b.current.counts
	}
	c.Requests++
	c.InFlight++
	return b.current, nil
}
func contextError(ctx context.Context) error {
	err := ctx.Err()
	if err == nil {
		deadline, ok := ctx.Deadline()
		if !ok || time.Now().Before(deadline) {
			return nil
		}
		err = context.DeadlineExceeded
	}
	return errors.Join(err, context.Cause(ctx))
}
func (b *Breaker) finish(g *generation, result outcome) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.advance(now)
	if b.current != g {
		return
	}
	c := &g.counts
	terminal := terminalCount(c, result)
	if *terminal == math.MaxUint64 {
		b.rotate(now)
		c = &b.current.counts
		c.Requests = 1
		c.InFlight = 1
		terminal = terminalCount(c, result)
	}
	c.InFlight--
	*terminal++
	switch c.State {
	case StateClosed:
		completed := c.Successes + c.Failures
		if completed >= b.cfg.RequestThreshold && reachedRate(c.Failures, completed, b.cfg.FailureRate) {
			b.start(StateOpen, now)
		}
	case StateHalfOpen:
		if result == failure {
			b.start(StateOpen, now)
		} else if c.Successes >= b.cfg.HalfOpenMax {
			b.start(StateClosed, now)
		}
	}
}
func terminalCount(c *Counts, result outcome) *uint64 {
	switch result {
	case success:
		return &c.Successes
	case failure:
		return &c.Failures
	case ignored:
		return &c.Ignored
	default:
		return &c.Panics
	}
}

func reachedRate(failures, completed uint64, rate float64) bool {
	if failures == 0 {
		return false
	}
	raw := math.Float64bits(rate)
	exponent := (raw >> 52) & 0x7ff
	mantissa := raw & ((1 << 52) - 1)
	shift := uint(1074)
	if exponent != 0 {
		mantissa |= 1 << 52
		shift = uint(1075 - exponent)
	}
	hi, lo := bits.Mul64(completed, mantissa)
	var threshold uint64
	var remainder bool
	switch {
	case shift >= 128:
		return true
	case shift >= 64:
		offset := shift - 64
		threshold = hi >> offset
		remainder = lo != 0 || hi&((uint64(1)<<offset)-1) != 0
	default:
		threshold = hi<<(64-shift) | lo>>shift
		remainder = lo&((uint64(1)<<shift)-1) != 0
	}
	if remainder {
		threshold++
	}
	return failures >= threshold
}

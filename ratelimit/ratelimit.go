package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"
)

var (
	ErrInvalidConfig = errors.New("invalid ratelimit configuration")
	ErrInvalidCall   = errors.New("invalid ratelimit call")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "ratelimit config: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	RatePerSecond float64
	Burst         int
}

func (cfg Config) Validate() error {
	if math.IsNaN(cfg.RatePerSecond) || math.IsInf(cfg.RatePerSecond, 0) || cfg.RatePerSecond <= 0 {
		return &ConfigError{"RatePerSecond", "must be finite and positive"}
	}
	if cfg.Burst <= 0 {
		return &ConfigError{"Burst", "must be positive"}
	}
	return nil
}

type timeAnchor struct {
	at    time.Time
	phase big.Int
}
type waiter struct {
	n          int
	ready      chan struct{}
	prev, next *waiter
}
type arithmetic struct {
	credit, whole, remainder big.Int
}
type Limiter struct {
	mu                     sync.Mutex
	cfg                    Config
	tokens                 int
	numerator, denominator big.Int
	anchor                 timeAnchor
	head, tail             *waiter
	timer                  *time.Timer
	work                   arithmetic
}

func New(cfg Config) (*Limiter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	l := &Limiter{cfg: cfg, tokens: cfg.Burst, anchor: timeAnchor{at: time.Now()}}
	rate := new(big.Rat).SetFloat64(cfg.RatePerSecond)
	rate.Quo(rate, big.NewRat(int64(time.Second), 1))
	l.numerator.Set(rate.Num())
	l.denominator.Set(rate.Denom())
	return l, nil
}
func (l *Limiter) valid(n int) bool { return l != nil && l.cfg.Burst > 0 && n > 0 && n <= l.cfg.Burst }
func (l *Limiter) Allow(n int) bool {
	if !l.valid(n) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.head != nil {
		return false
	}
	l.refill(time.Now())
	if l.tokens < n {
		return false
	}
	l.tokens -= n
	return true
}
func (l *Limiter) Wait(ctx context.Context, n int) error {
	if ctx == nil {
		return fmt.Errorf("ctx: %w", ErrInvalidCall)
	}
	if l == nil || l.cfg.Burst <= 0 {
		return fmt.Errorf("limiter: %w", ErrInvalidCall)
	}
	if !l.valid(n) {
		return fmt.Errorf("n: %w", ErrInvalidCall)
	}
	l.mu.Lock()
	if err := contextError(ctx); err != nil {
		l.mu.Unlock()
		return err
	}
	l.refill(time.Now())
	if l.head == nil && l.tokens >= n {
		l.tokens -= n
		l.mu.Unlock()
		return nil
	}
	w := &waiter{n: n, ready: make(chan struct{}), prev: l.tail}
	if l.tail != nil {
		l.tail.next = w
	} else {
		l.head = w
		close(w.ready)
	}
	l.tail = w
	l.mu.Unlock()
	select {
	case <-w.ready:
	case <-ctx.Done():
	}
	l.mu.Lock()
	for {
		if err := contextError(ctx); err != nil {
			l.remove(w)
			l.mu.Unlock()
			return err
		}
		l.refill(time.Now())
		if l.tokens >= n {
			l.tokens -= n
			l.remove(w)
			l.mu.Unlock()
			return nil
		}
		delay := l.delay(n)
		if l.timer == nil {
			l.timer = time.NewTimer(delay)
		} else {
			l.timer.Reset(delay)
		}
		tick := l.timer.C
		l.mu.Unlock()
		select {
		case <-tick:
		case <-ctx.Done():
		}
		l.mu.Lock()
	}
}
func (l *Limiter) remove(w *waiter) {
	if w.prev != nil {
		w.prev.next = w.next
	} else {
		if l.timer != nil {
			l.timer.Stop()
		}
		l.head = w.next
		if l.head != nil {
			close(l.head.ready)
		} else {
			l.timer = nil
		}
	}
	if w.next != nil {
		w.next.prev = w.prev
	} else {
		l.tail = w.prev
	}
	w.prev = nil
	w.next = nil
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
func (l *Limiter) refill(now time.Time) {
	if now.Before(l.anchor.at) {
		return
	}
	if l.tokens == l.cfg.Burst {
		l.anchor.at = now
		l.anchor.phase.SetInt64(0)
		return
	}
	credit, whole, remainder := &l.work.credit, &l.work.whole, &l.work.remainder
	for l.anchor.at.Before(now) {
		elapsed := now.Sub(l.anchor.at)
		credit.Mul(&l.numerator, big.NewInt(int64(elapsed)))
		credit.Add(credit, &l.anchor.phase)
		whole.QuoRem(credit, &l.denominator, remainder)
		if whole.Cmp(big.NewInt(int64(l.cfg.Burst-l.tokens))) >= 0 {
			l.tokens = l.cfg.Burst
			l.anchor.at = now
			l.anchor.phase.SetInt64(0)
			return
		}
		l.tokens += int(whole.Int64())
		l.anchor.phase.Set(remainder)
		l.anchor.at = l.anchor.at.Add(elapsed)
	}
}
func (l *Limiter) delay(n int) time.Duration {
	needed, quotient, remainder := &l.work.credit, &l.work.whole, &l.work.remainder
	needed.Mul(big.NewInt(int64(n-l.tokens)), &l.denominator)
	needed.Sub(needed, &l.anchor.phase)
	quotient.QuoRem(needed, &l.numerator, remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(max(quotient.Int64(), 1))
}

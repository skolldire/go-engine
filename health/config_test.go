package health

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func newRegistry(t testing.TB, cfg Config) *Registry {
	t.Helper()
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func register(t testing.TB, r *Registry, name string, c Checker) {
	t.Helper()
	if err := r.Register(name, c); err != nil {
		t.Fatal(err)
	}
}
func TestConfig(t *testing.T) {
	if DefaultConfig() != (Config{5 * time.Second, 4, 128}) {
		t.Fatal("defaults")
	}
	for _, tt := range []struct {
		cfg   Config
		field string
	}{{Config{}, "Timeout"}, {Config{Timeout: 1}, "MaxParallel"}, {Config{Timeout: 1, MaxParallel: 1}, "MaxChecks"}, {Config{Timeout: -1, MaxParallel: 1, MaxChecks: 1}, "Timeout"}} {
		r, err := New(tt.cfg)
		var ce *ConfigError
		if r != nil || !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tt.field {
			t.Fatal("config field", tt.field, err)
		}
	}
	r := newRegistry(t, Config{time.Duration(1<<63 - 1), int(^uint(0) >> 1), int(^uint(0) >> 1)})
	if !r.Live() {
		t.Fatal("live")
	}
}

type nilChecker struct{}

func (*nilChecker) Check(context.Context) error { return nil }
func TestRegistration(t *testing.T) {
	r := newRegistry(t, Config{time.Second, 1, 1})
	ok := CheckerFunc(func(context.Context) error { return nil })
	var ptr *nilChecker
	var fn CheckerFunc
	for _, tt := range []struct {
		name string
		c    Checker
	}{{"", ok}, {strings.Repeat("a", 129), ok}, {"a b", ok}, {"é", ok}, {"a/b", ok}, {"name", nil}, {"name", ptr}, {"name", fn}} {
		if err := r.Register(tt.name, tt.c); !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("invalid accepted: %q", tt.name)
		}
	}
	name := strings.Repeat("A", 125) + "_.-"
	register(t, r, name, ok)
	if err := r.Register(name, ok); !errors.Is(err, ErrDuplicateCheck) {
		t.Fatal(err)
	}
	if err := r.Register("other", ok); !errors.Is(err, ErrTooManyChecks) {
		t.Fatal(err)
	}
}
func TestConcurrentRegister(t *testing.T) {
	r := newRegistry(t, DefaultConfig())
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			if err := r.Register(fmt.Sprint("check", i), CheckerFunc(func(context.Context) error { return nil })); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(r.checks) != 100 {
		t.Fatal("lost registration")
	}
}

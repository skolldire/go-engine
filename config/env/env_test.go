package env_test

import (
	"context"
	"errors"
	"testing"

	c "github.com/skolldire/go-engine/config"
	e "github.com/skolldire/go-engine/config/env"
)

func TestBindingsDefaultsAndSnapshot(t *testing.T) {
	bindings := map[string]string{"app.port": "PORT", "app.enabled": "ENABLED", "app.empty": "EMPTY", "app.absent": "ABSENT"}
	values := map[string]string{"PORT": "0", "ENABLED": "false", "EMPTY": ""}
	calls := map[string]int{}
	src, err := e.New(e.Config{Bindings: bindings}, e.Dependencies{Env: func(name string) (string, bool) { calls[name]++; v, ok := values[name]; return v, ok }})
	if err != nil {
		t.Fatal(err)
	}
	bindings["app.port"] = "OTHER"
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg := struct {
		Port    int
		Enabled bool
		Empty   string
		Absent  int
	}{99, true, "seed", 7}
	if err := s.Decode("app", &cfg); err != nil || cfg.Port != 0 || cfg.Enabled || cfg.Empty != "" || cfg.Absent != 7 {
		t.Fatal(cfg, err)
	}
	for name, count := range calls {
		if count != 1 {
			t.Fatal(name, count)
		}
	}
	values["PORT"] = "42"
	if err := s.Decode("app", &cfg); err != nil || cfg.Port != 0 {
		t.Fatal("snapshot changed", err)
	}
	var nilCtx context.Context
	_, err = src.Load(nilCtx)
	if !errors.Is(err, c.ErrNilContext) {
		t.Fatal(err)
	}
}
func TestInvalidBindingsAndRequired(t *testing.T) {
	lookup := e.Dependencies{Env: func(string) (string, bool) { return "", false }}
	for _, cfg := range []e.Config{{}, {Bindings: map[string]string{"bad..path": "X"}}, {Bindings: map[string]string{"x": ""}}, {Bindings: map[string]string{"x": "X", "x.y": "Y"}}, {Bindings: map[string]string{"x": "X", "x-a": "Z", "x.y": "Y"}}, {Bindings: map[string]string{"x": "X"}, Required: []string{"missing"}}} {
		if _, err := e.New(cfg, lookup); !errors.Is(err, c.ErrInvalidConfig) {
			t.Fatalf("%+v: %v", cfg, err)
		}
	}
	cfg := e.Config{Bindings: map[string]string{"x": "X"}, Required: []string{"x"}}
	src, err := e.New(cfg, lookup)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Load(context.Background())
	if !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	if _, err := e.New(cfg, e.Dependencies{}); !errors.Is(err, c.ErrInvalidConfig) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = src.Load(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

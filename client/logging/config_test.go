package logging

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/skolldire/go-engine/client"
)

type disabledLogger struct{ enabledCalls, emissions int }

func (l *disabledLogger) Enabled(context.Context, slog.Level) bool                   { l.enabledCalls++; return false }
func (l *disabledLogger) LogAttrs(context.Context, slog.Level, string, ...slog.Attr) { l.emissions++ }
func TestConfigAndConstruction(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Level != "debug" || !cfg.IncludeDuration {
		t.Fatal(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, level := range []string{"", "DEBUG", "warn", "error", " info ", "do-not-emit-value"} {
		_, err := New[int](Config{Level: level}, Dependencies{})
		var ce *ConfigError
		if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != "Level" || strings.Contains(err.Error(), "do-not-emit-value") {
			t.Fatalf("%v", err)
		}
	}
	var typedNil *slog.Logger
	for _, logger := range []Logger{nil, typedNil} {
		_, err := New[int](cfg, Dependencies{Logger: logger})
		var ce *ConfigError
		if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != "Logger" {
			t.Fatal(err)
		}
	}
	log := &disabledLogger{}
	mw, err := New[int](cfg, Dependencies{Logger: log})
	if err != nil || log.enabledCalls != 0 || log.emissions != 0 || mw(nil) != nil {
		t.Fatalf("impure construction: %v", err)
	}
	if _, err := client.Chain[int](nil, mw); !errors.Is(err, client.ErrInvalidCall) {
		t.Fatal(err)
	}
}
func TestDisabledAndUnmeasuredPaths(t *testing.T) {
	for _, duration := range []bool{false, true} {
		logger := &disabledLogger{}
		reads := 0
		mw := middleware[int](slog.LevelDebug, duration, logger, func() time.Time { reads++; return time.Now() })
		if reads != 0 {
			t.Fatal("clock at construction")
		}
		next := func(context.Context, client.Invocation, client.Operation[int]) (int, error) { return 7, nil }
		value, err := mw(next)(context.Background(), client.Invocation{}, nil)
		if value != 7 || err != nil || reads != 0 || logger.emissions != 0 || logger.enabledCalls != 1 {
			t.Fatal("disabled path measured or emitted")
		}
	}
	logger := &enabledLogger{}
	reads := 0
	mw := middleware[int](slog.LevelInfo, false, logger, func() time.Time { reads++; return time.Now() })
	_, err := mw(func(context.Context, client.Invocation, client.Operation[int]) (int, error) { return 1, nil })(context.Background(), client.Invocation{}, nil)
	if err != nil || reads != 0 || logger.emissions != 1 {
		t.Fatal("duration=false measured")
	}
}

type enabledLogger struct{ emissions int }

func (*enabledLogger) Enabled(context.Context, slog.Level) bool                     { return true }
func (l *enabledLogger) LogAttrs(context.Context, slog.Level, string, ...slog.Attr) { l.emissions++ }

func TestDurationUsesOneClock(t *testing.T) {
	start := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	reads := 0
	logger := &durationLogger{}
	mw := middleware[int](slog.LevelInfo, true, logger, func() time.Time { value := start.Add(time.Duration(reads) * 7 * time.Second); reads++; return value })
	_, err := mw(func(context.Context, client.Invocation, client.Operation[int]) (int, error) { return 1, nil })(context.Background(), client.Invocation{}, nil)
	if err != nil || reads != 2 || logger.duration != 7*time.Second {
		t.Fatalf("reads=%d duration=%s error=%v", reads, logger.duration, err)
	}
}

type durationLogger struct{ duration time.Duration }

func (*durationLogger) Enabled(context.Context, slog.Level) bool { return true }
func (l *durationLogger) LogAttrs(_ context.Context, _ slog.Level, _ string, attrs ...slog.Attr) {
	for _, a := range attrs {
		if a.Key == "duration" {
			l.duration = a.Value.Duration()
		}
	}
}

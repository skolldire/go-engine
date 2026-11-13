package logging

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestConfiguration(t *testing.T) {
	a, b := DefaultConfig(), DefaultConfig()
	a.RedactKeys[0] = "changed"
	if b.RedactKeys[0] != "password" {
		t.Fatal("shared defaults")
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, field string
		edit        func(*Config)
	}{
		{"zero", "Level", func(c *Config) { *c = Config{} }},
		{"level", "Level", func(c *Config) { c.Level = "INFO" }},
		{"format", "Format", func(c *Config) { c.Format = "invalid" }},
		{"keys", "RedactKeys[1]", func(c *Config) { c.RedactKeys = []string{"valid", ""}; c.MaxValueBytes = 0 }},
		{"size", "MaxValueBytes", func(c *Config) { c.MaxValueBytes = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.edit(&cfg)
			_, err := New(cfg, Dependencies{})
			var ce *ConfigError
			if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tc.field {
				t.Fatalf("%v", err)
			}
		})
	}
	_, err := New(b, Dependencies{})
	var ce *ConfigError
	if !errors.As(err, &ce) || ce.Field != "Output" {
		t.Fatal(err)
	}
	b.RedactKeys = nil
	if _, err := New(b, Dependencies{Output: io.Discard}); err != nil {
		t.Fatal(err)
	}
}
func TestConstructionIsPure(t *testing.T) {
	old := slog.Default()
	calls := 0
	writer := writerFunc(func(p []byte) (int, error) { calls++; return len(p), nil })
	cfg := DefaultConfig()
	_, err := New(cfg, Dependencies{Output: writer})
	if err != nil || calls != 0 || slog.Default() != old {
		t.Fatalf("constructor side effects: %v", err)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestErrorConfidentialityAndCapturedKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Level = "do-not-emit-this-value"
	_, err := New(cfg, Dependencies{Output: io.Discard})
	if err == nil || strings.Contains(err.Error(), cfg.Level) || !strings.Contains(err.Error(), "Level") {
		t.Fatalf("unsafe error: %v", err)
	}
	cfg = DefaultConfig()
	var out bytes.Buffer
	l, err := New(cfg, Dependencies{Output: &out})
	if err != nil {
		t.Fatal(err)
	}
	cfg.RedactKeys[0] = "renamed"
	l.Info("capture", "password", "hidden-original-value", "renamed", "ordinary")
	if bytes.Contains(out.Bytes(), []byte("hidden-original-value")) || !bytes.Contains(out.Bytes(), []byte(`"renamed":"ordinary"`)) {
		t.Fatal(out.String())
	}
}

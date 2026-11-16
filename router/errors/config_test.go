package errors

import (
	"errors"
	"testing"
)

func TestConfigAndConstruction(t *testing.T) {
	if DefaultConfig() != (Config{true, 4096}) {
		t.Fatal("defaults")
	}
	for _, tt := range []struct {
		cfg   Config
		deps  Dependencies
		field string
	}{{Config{}, Dependencies{}, "MaxDetailsBytes"}, {Config{MaxDetailsBytes: -1}, Dependencies{}, "MaxDetailsBytes"}, {DefaultConfig(), Dependencies{}, "Classify"}} {
		r, err := NewResponder(tt.cfg, tt.deps)
		var ce *ConfigError
		if r != nil || !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tt.field {
			t.Fatal("validation field", tt.field, err)
		}
	}
	r, err := NewResponder(Config{false, 1}, Dependencies{Classify: func(error) Problem { t.Fatal("classified in constructor"); return Problem{} }})
	if err != nil || r.cfg.IncludeRequestID {
		t.Fatal("literal/constructor")
	}
}

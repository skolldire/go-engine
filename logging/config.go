package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
)

var ErrInvalidConfig = errors.New("invalid logging configuration")

type ConfigError struct {
	Field  string
	Reason string
}

func (e *ConfigError) Error() string { return fmt.Sprintf("logging: %s: %s", e.Field, e.Reason) }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	Level         string   `mapstructure:"level" json:"level"`
	Format        string   `mapstructure:"format" json:"format"`
	AddSource     bool     `mapstructure:"add_source" json:"add_source"`
	RedactKeys    []string `mapstructure:"redact_keys" json:"redact_keys"`
	MaxValueBytes int      `mapstructure:"max_value_bytes" json:"max_value_bytes"`
}
type Redactor func(groups []string, attr slog.Attr) slog.Attr

type Dependencies struct {
	Output   io.Writer
	Attrs    []slog.Attr
	Redactor Redactor
}

func DefaultConfig() Config {
	return Config{Level: "info", Format: "json", RedactKeys: []string{"password", "token", "authorization", "secret"}, MaxValueBytes: 4096}
}
func (cfg Config) Validate() error {
	switch cfg.Level {
	case "debug", "info", "warn", "error":
	default:
		return &ConfigError{Field: "Level", Reason: "must be debug, info, warn or error"}
	}
	switch cfg.Format {
	case "json", "text":
	default:
		return &ConfigError{Field: "Format", Reason: "must be json or text"}
	}
	for i, key := range cfg.RedactKeys {
		if key == "" {
			return &ConfigError{Field: fmt.Sprintf("RedactKeys[%d]", i), Reason: "must not be empty"}
		}
	}
	if cfg.MaxValueBytes <= 0 {
		return &ConfigError{Field: "MaxValueBytes", Reason: "must be positive"}
	}
	return nil
}
func New(cfg Config, deps Dependencies) (*slog.Logger, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Output == nil {
		return nil, &ConfigError{Field: "Output", Reason: "writer is required"}
	}
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	options := &slog.HandlerOptions{Level: level, AddSource: cfg.AddSource}
	var output slog.Handler
	if cfg.Format == "json" {
		output = slog.NewJSONHandler(deps.Output, options)
	} else {
		output = slog.NewTextHandler(deps.Output, options)
	}
	h := &handler{output: output, policy: &policy{keys: slices.Clone(cfg.RedactKeys), maxBytes: cfg.MaxValueBytes, redactor: deps.Redactor}}
	var selected slog.Handler = h
	if len(deps.Attrs) > 0 {
		selected = h.WithAttrs(slices.Clone(deps.Attrs))
	}
	return slog.New(selected), nil
}

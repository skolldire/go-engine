package logging

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/skolldire/go-engine/client"
)

var ErrInvalidConfig = errors.New("invalid client logging configuration")

type ConfigError struct {
	Field  string
	Reason string
}

func (e *ConfigError) Error() string { return "client/logging: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	Level           string `mapstructure:"level" json:"level"`
	IncludeDuration bool   `mapstructure:"include_duration" json:"include_duration"`
}
type Logger interface {
	Enabled(context.Context, slog.Level) bool
	LogAttrs(context.Context, slog.Level, string, ...slog.Attr)
}
type Dependencies struct{ Logger Logger }

func DefaultConfig() Config { return Config{Level: "debug", IncludeDuration: true} }
func (cfg Config) Validate() error {
	if cfg.Level != "debug" && cfg.Level != "info" {
		return &ConfigError{Field: "Level", Reason: "must be debug or info"}
	}
	return nil
}
func New[T any](cfg Config, deps Dependencies) (client.Middleware[T], error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Logger == nil {
		return nil, &ConfigError{Field: "Logger", Reason: "logger is required"}
	}
	if logger, ok := deps.Logger.(*slog.Logger); ok && logger == nil {
		return nil, &ConfigError{Field: "Logger", Reason: "logger is required"}
	}
	level := slog.LevelDebug
	if cfg.Level == "info" {
		level = slog.LevelInfo
	}
	return middleware[T](level, cfg.IncludeDuration, deps.Logger, time.Now), nil
}
func middleware[T any](level slog.Level, includeDuration bool, logger Logger, now func() time.Time) client.Middleware[T] {
	return func(next client.Handler[T]) client.Handler[T] {
		if next == nil {
			return nil
		}
		return func(ctx context.Context, info client.Invocation, operation client.Operation[T]) (T, error) {
			if !logger.Enabled(ctx, level) {
				return next(ctx, info, operation)
			}
			var start time.Time
			if includeDuration {
				start = now()
			}
			result, err := next(ctx, info, operation)
			attrs := [4]slog.Attr{
				slog.String("component", info.Component),
				slog.String("operation", info.Operation),
				slog.Bool("success", err == nil),
			}
			count := 3
			if includeDuration {
				attrs[3] = slog.Duration("duration", now().Sub(start))
				count++
			}
			logger.LogAttrs(ctx, level, "operation completed", attrs[:count]...)
			return result, err
		}
	}
}

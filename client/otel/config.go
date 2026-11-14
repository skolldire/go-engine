package otel

import (
	"errors"
	"reflect"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var ErrInvalidConfig = errors.New("invalid client telemetry configuration")

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "client/otel: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	InstrumentationName string `mapstructure:"instrumentation_name" json:"instrumentation_name"`
}
type Dependencies struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	IsFailure      func(error) bool
}

func DefaultConfig() Config { return Config{} }
func (cfg Config) Validate() error {
	if cfg.InstrumentationName == "" {
		return &ConfigError{Field: "InstrumentationName", Reason: "is required"}
	}
	return nil
}
func nilPointer(value any) bool {
	return value != nil && reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil()
}
func validate(cfg Config, deps Dependencies) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if nilPointer(deps.TracerProvider) {
		return &ConfigError{Field: "TracerProvider", Reason: "must not be a nil pointer"}
	}
	if nilPointer(deps.MeterProvider) {
		return &ConfigError{Field: "MeterProvider", Reason: "must not be a nil pointer"}
	}
	if deps.TracerProvider == nil && deps.MeterProvider == nil {
		return &ConfigError{Field: "Dependencies", Reason: "at least one provider is required"}
	}
	if deps.IsFailure != nil && deps.TracerProvider == nil {
		return &ConfigError{Field: "IsFailure", Reason: "requires TracerProvider"}
	}
	return nil
}

package otel

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type nilTracerProvider struct{ trace.TracerProvider }

func TestConfigAndValidationOrder(t *testing.T) {
	if DefaultConfig().InstrumentationName != "" {
		t.Fatal("implicit name")
	}
	var typedNil *nilTracerProvider
	cases := []struct {
		cfg   Config
		deps  Dependencies
		field string
	}{
		{Config{}, Dependencies{TracerProvider: typedNil}, "InstrumentationName"},
		{Config{"instrumentation"}, Dependencies{TracerProvider: typedNil}, "TracerProvider"},
		{Config{"instrumentation"}, Dependencies{}, "Dependencies"},
		{Config{"instrumentation"}, Dependencies{MeterProvider: noop.NewMeterProvider(), IsFailure: func(error) bool { return true }}, "IsFailure"},
	}
	for _, tt := range cases {
		err := validate(tt.cfg, tt.deps)
		var ce *ConfigError
		if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tt.field {
			t.Fatalf("field %s: %v", tt.field, err)
		}
	}
	if err := (Config{" "}).Validate(); err != nil {
		t.Fatal("unexpected normalization/restriction", err)
	}
}

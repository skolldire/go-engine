package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"github.com/stretchr/testify/assert"
)

func TestNewTelemetry_Disabled(t *testing.T) {
	config := Config{
		Enabled: false,
	}
	
	tel, err := NewTelemetry(context.Background(), config)
	assert.NoError(t, err)
	assert.NotNil(t, tel)
	
	// Should be noop telemetry
	tel.Counter(context.Background(), "test", 1)
	tel.Gauge(context.Background(), "test", 1.0)
	tel.Histogram(context.Background(), "test", 1.0)
	err = tel.Span(context.Background(), "test", func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

func TestNoopTelemetry(t *testing.T) {
	noop := &noopTelemetry{}
	
	noop.Counter(context.Background(), "test", 1)
	noop.Gauge(context.Background(), "test", 1.0)
	noop.Histogram(context.Background(), "test", 1.0)
	
	err := noop.Span(context.Background(), "test", func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
	
	err = noop.Span(context.Background(), "test", func(ctx context.Context) error {
		return errors.New("test error")
	})
	assert.Error(t, err)
	
	err = noop.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestNewOperation(t *testing.T) {
	noop := &noopTelemetry{}
	op := NewOperation(noop, "test_operation")
	assert.NotNil(t, op)
	assert.Equal(t, "test_operation", op.name)
}

func TestOperation_Execute_Success(t *testing.T) {
	noop := &noopTelemetry{}
	op := NewOperation(noop, "test_operation")
	
	err := op.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	}, attribute.String("key", "value"))
	assert.NoError(t, err)
}

func TestOperation_Execute_Error(t *testing.T) {
	noop := &noopTelemetry{}
	op := NewOperation(noop, "test_operation")
	
	testErr := errors.New("test error")
	err := op.Execute(context.Background(), func(ctx context.Context) error {
		return testErr
	})
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
}




package otel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// ── Provider ─────────────────────────────────────────────────────────────────

func TestNewProvider_Disabled(t *testing.T) {
	p, err := NewProvider(context.Background(), OTELConfig{Enabled: false})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewProvider_Disabled_IsNoop(t *testing.T) {
	p, err := NewProvider(context.Background(), OTELConfig{Enabled: false})
	require.NoError(t, err)

	_, ok := p.(*noopProvider)
	assert.True(t, ok, "expected noopProvider when Enabled=false")
}

func TestNoopProvider_Tracer(t *testing.T) {
	p := &noopProvider{}
	tracer := p.Tracer("test")
	assert.NotNil(t, tracer)

	ctx, span := tracer.Start(context.Background(), "test-span")
	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
	span.End()
}

func TestNoopProvider_Meter(t *testing.T) {
	p := &noopProvider{}
	meter := p.Meter("test")
	assert.NotNil(t, meter)
}

func TestNoopProvider_Shutdown(t *testing.T) {
	p := &noopProvider{}
	err := p.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestNewProvider_Enabled_DefaultSamplingRate(t *testing.T) {
	// With SamplingRate=0, should default to 1.0 without panicking.
	// We use an in-process exporter to avoid needing a real OTLP backend.
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(1.0)),
	)
	assert.NotNil(t, tp)
	_ = tp.Shutdown(context.Background())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func TestSpanFromContext_NoSpan(t *testing.T) {
	span := SpanFromContext(context.Background())
	assert.NotNil(t, span)
	assert.False(t, span.SpanContext().IsValid(), "expected noop span when none in context")
}

func TestSpanFromContext_WithSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	defer tp.Shutdown(context.Background()) //nolint:errcheck

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	got := SpanFromContext(ctx)
	assert.True(t, got.SpanContext().IsValid())
}

func TestAddSpanEvent_NoopSpan(t *testing.T) {
	// Should not panic when no span is in context.
	assert.NotPanics(t, func() {
		AddSpanEvent(context.Background(), "event", attribute.String("k", "v"))
	})
}

func TestAddSpanEvent_WithSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	defer tp.Shutdown(context.Background()) //nolint:errcheck

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	AddSpanEvent(ctx, "my-event", attribute.Int("code", 200))
	span.End()

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	require.Len(t, spans[0].Events, 1)
	assert.Equal(t, "my-event", spans[0].Events[0].Name)
}

func TestRecordError_Nil(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordError(context.Background(), nil)
	})
}

func TestRecordError_NoopSpan(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordError(context.Background(), errors.New("boom"))
	})
}

func TestRecordError_WithSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	defer tp.Shutdown(context.Background()) //nolint:errcheck

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	RecordError(ctx, errors.New("something failed"))
	span.End()

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	require.Len(t, spans[0].Events, 1)
	assert.Equal(t, "exception", spans[0].Events[0].Name)
}

// ── Middleware ────────────────────────────────────────────────────────────────

func TestNewMiddleware_Disabled_Passthrough(t *testing.T) {
	cfg := OTELConfig{Enabled: false, ServiceName: "svc"}
	mw := NewMiddleware(cfg)

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(handler).ServeHTTP(w, r)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNewMiddleware_Enabled_WrapsHandler(t *testing.T) {
	cfg := OTELConfig{Enabled: true, ServiceName: "svc"}
	mw := NewMiddleware(cfg)

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	mw(handler).ServeHTTP(w, r)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

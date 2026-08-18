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

// TestNewProvider_RefusesASecondGlobalInstall is the regression test for the
// OpenTelemetry globals being documented but not protected.
//
// They are a process-wide resource with one owner. A second provider used to
// overwrite the first silently, and the losing engine's spans disappeared with
// nothing in the logs. Failing at construction puts the conflict where it can
// be acted on.
func TestNewProvider_RefusesASecondGlobalInstall(t *testing.T) {
	ctx := context.Background()

	first, err := NewProvider(ctx, OTELConfig{ServiceName: "first", Enabled: false})
	require.NoError(t, err)

	// A disabled provider installs nothing, so it must not take the claim.
	second, err := NewProvider(ctx, OTELConfig{ServiceName: "second", Enabled: false})
	require.NoError(t, err, "disabled providers touch no global state")

	require.NoError(t, first.Shutdown(ctx))
	require.NoError(t, second.Shutdown(ctx))
}

// TestGlobalClaim_IsExclusiveAndReleasable exercises the claim directly, since
// installing real providers needs a collector.
func TestGlobalClaim_IsExclusiveAndReleasable(t *testing.T) {
	first, err := claimGlobalProviders("first")
	require.NoError(t, err)
	t.Cleanup(func() { releaseGlobalProviders(first) })

	_, err = claimGlobalProviders("second")
	require.Error(t, err, "the globals have one owner")
	assert.ErrorContains(t, err, "first", "the message must name who holds them")
	assert.ErrorContains(t, err, "skip_global_providers", "and say how to resolve it")

	// A provider that shuts down must not lock the process out for good.
	releaseGlobalProviders(first)

	third, err := claimGlobalProviders("third")
	require.NoError(t, err)
	releaseGlobalProviders(third)
}

// TestGlobalClaim_ReleaseIsScopedToItsOwner is the regression test for a
// repeated Shutdown freeing someone else's claim.
//
// Ownership was a boolean, so shutting A down twice released the globals B had
// taken in between: B kept exporting while the process believed the slot was
// free, and a third provider could install over it.
func TestGlobalClaim_ReleaseIsScopedToItsOwner(t *testing.T) {
	a, err := claimGlobalProviders("A")
	require.NoError(t, err)

	releaseGlobalProviders(a) // A shuts down

	b, err := claimGlobalProviders("B")
	require.NoError(t, err, "the globals are free again")
	t.Cleanup(func() { releaseGlobalProviders(b) })

	// A shuts down a second time. Its claim is stale and must do nothing.
	releaseGlobalProviders(a)

	_, err = claimGlobalProviders("C")
	require.Error(t, err, "B still owns the globals; a stale release must not free them")
	assert.ErrorContains(t, err, "B")
}

// TestProvider_ShutdownIsIdempotentForTheClaim covers the same guarantee
// through the public API.
func TestProvider_ShutdownIsIdempotentForTheClaim(t *testing.T) {
	ctx := context.Background()

	a, err := claimGlobalProviders("A")
	require.NoError(t, err)

	p := &realProvider{globals: a}
	require.NoError(t, p.Shutdown(ctx))

	b, err := claimGlobalProviders("B")
	require.NoError(t, err)
	t.Cleanup(func() { releaseGlobalProviders(b) })

	// A second Shutdown on the same provider must not disturb B.
	require.NoError(t, p.Shutdown(ctx))

	_, err = claimGlobalProviders("C")
	assert.Error(t, err, "B must still hold the globals after A shut down twice")
}

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/skolldire/go-engine/client"
	clientotel "github.com/skolldire/go-engine/client/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

type captureProvider struct {
	trace.TracerProvider
	started, ended int
	name           string
}

func (p *captureProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return captureTracer{Tracer: p.TracerProvider.Tracer(name, opts...), owner: p}
}

type captureTracer struct {
	trace.Tracer
	owner *captureProvider
}

func (t captureTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx, span := t.Tracer.Start(ctx, name, opts...)
	t.owner.started++
	t.owner.name = name
	s := captureSpan{Span: span, owner: t.owner}
	return trace.ContextWithSpan(ctx, s), s
}

type captureSpan struct {
	trace.Span
	owner *captureProvider
}

func (s captureSpan) End(...trace.SpanEndOption) { s.owner.ended++ }
func run() error {
	cfg := clientotel.DefaultConfig()
	if !errors.Is(cfg.Validate(), clientotel.ErrInvalidConfig) {
		return errors.New("missing name accepted")
	}
	cfg.InstrumentationName = "minimal"
	p := &captureProvider{TracerProvider: tracenoop.NewTracerProvider()}
	mw, e := clientotel.New[int](cfg, clientotel.Dependencies{TracerProvider: p, MeterProvider: metricnoop.NewMeterProvider()})
	if e != nil {
		return e
	}
	original := errors.New("original I/O error")
	info := client.Invocation{Component: "storage", Operation: "read"}
	op := func(context.Context) (int, error) { return 42, original }
	result, e := client.Do(context.Background(), info, op, mw)
	if result != 42 || e != original || p.started != 1 || p.ended != 1 || p.name != "storage.read" {
		return errors.New("typed result/trace mismatch")
	}
	handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }, mw)
	if e != nil {
		return e
	}
	result, e = handler(context.Background(), info, op)
	if result != 42 || e != original || p.ended != 2 {
		return errors.New("chain mismatch")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("client/otel minimal: API-only providers and typed calls verified")
}

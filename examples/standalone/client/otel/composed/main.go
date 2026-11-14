package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/client"
	clientotel "github.com/skolldire/go-engine/client/otel"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
	provider "github.com/skolldire/go-engine/telemetry/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func run() (result error) {
	cfg := provider.DefaultConfig()
	cfg.ServiceName = "composed"
	rec := tracetest.NewSpanRecorder()
	reader := sdkmetric.NewManualReader()
	p, e := provider.New(cfg, provider.Dependencies{TraceProcessor: rec, MetricReader: reader})
	if e != nil {
		return e
	}
	defer func() { result = errors.Join(result, p.Shutdown(context.Background())) }()
	mw, e := clientotel.New[int](clientotel.Config{InstrumentationName: "composed"}, clientotel.Dependencies{TracerProvider: p.TracerProvider(), MeterProvider: p.MeterProvider()})
	if e != nil {
		return e
	}
	policy := resilience.DefaultConfig()
	policy.Retry.MaxRetries = 2
	policy.Retry.InitialBackoff = time.Nanosecond
	policy.Retry.MaxBackoff = time.Nanosecond
	policy.Retry.Jitter = retry.JitterNone
	transient := errors.New("known transient")
	uncertain := errors.New("uncertain publication")
	classify := func(err error) retry.Decision {
		if errors.Is(err, uncertain) {
			return retry.DecisionReconcile
		}
		if errors.Is(err, transient) {
			return retry.DecisionRetry
		}
		return retry.DecisionStop
	}
	attempts := 0
	wrapped, e := resilience.Wrap(func(ctx context.Context) (int, error) {
		attempts++
		if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
			return 0, errors.New("missing logical span")
		}
		if attempts < 3 {
			return 7, transient
		}
		return 42, nil
	}, policy, resilience.Dependencies{Classify: classify})
	if e != nil {
		return e
	}
	ctx, parent := p.Tracer("assembler").Start(context.Background(), "parent")
	value, e := client.Do(ctx, client.Invocation{Component: "transport", Operation: "read"}, wrapped, mw)
	parent.End()
	if value != 42 || e != nil || attempts != 3 || len(rec.Ended()) != 2 {
		return errors.New("logical call/retry observation mismatch")
	}
	if rec.Ended()[0].Parent().SpanID() != rec.Ended()[1].SpanContext().SpanID() {
		return errors.New("parent mismatch")
	}
	reconcileAttempts := 0
	wrapped, e = resilience.Wrap(func(context.Context) (int, error) { reconcileAttempts++; return 9, uncertain }, policy, resilience.Dependencies{Classify: classify})
	if e != nil {
		return e
	}
	value, e = client.Do(context.Background(), client.Invocation{Component: "transport", Operation: "publish"}, wrapped, mw)
	if value != 9 || reconcileAttempts != 1 || !errors.Is(e, uncertain) || !errors.Is(e, retry.ErrReconciliationRequired) {
		return errors.New("reconciliation mismatch")
	}
	var rm metricdata.ResourceMetrics
	if e = reader.Collect(context.Background(), &rm); e != nil {
		return e
	}
	var observations uint64
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "go_engine.client.operation.duration" {
				return errors.New("unexpected instrument")
			}
			for _, dp := range m.Data.(metricdata.Histogram[float64]).DataPoints {
				observations += dp.Count
			}
		}
	}
	if observations != 2 {
		return errors.New("expected two logical histogram observations")
	}
	for _, span := range rec.Ended() {
		if len(span.Events()) != 0 {
			return errors.New("error/payload leaked as event")
		}
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("client/otel composed: real SDK, logical retry span and reconciliation verified")
}

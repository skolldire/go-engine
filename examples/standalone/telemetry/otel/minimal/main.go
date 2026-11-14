package main

import (
	"context"
	"fmt"

	provider "github.com/skolldire/go-engine/telemetry/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func run() error {
	cfg := provider.DefaultConfig()
	cfg.ServiceName = "minimal"
	cfg.MaxAttributeValueLength = 1
	rec := tracetest.NewSpanRecorder()
	reader := sdkmetric.NewManualReader()
	p, e := provider.New(cfg, provider.Dependencies{TraceProcessor: rec, MetricReader: reader})
	if e != nil {
		return e
	}
	_, span := p.Tracer("minimal").Start(context.Background(), "lookup", trace.WithAttributes(attribute.String("unicode", "界界")))
	span.End()
	counter, e := p.Meter("minimal").Int64Counter("calls")
	if e != nil {
		return e
	}
	counter.Add(context.Background(), 1)
	var rm metricdata.ResourceMetrics
	if e = reader.Collect(context.Background(), &rm); e != nil {
		return e
	}
	if len(rec.Ended()) != 1 || rec.Ended()[0].Attributes()[0].Value.AsString() != "界" || len(rm.ScopeMetrics) != 1 {
		return fmt.Errorf("native signal mismatch")
	}
	return p.Shutdown(context.Background())
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("telemetry minimal: traces, metrics and shutdown verified")
}

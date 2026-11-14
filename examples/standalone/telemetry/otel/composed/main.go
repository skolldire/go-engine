package main

import (
	"context"
	"errors"
	"example.com/telemetry-composed/observer"
	"fmt"
	"github.com/skolldire/go-engine/client"
	provider "github.com/skolldire/go-engine/telemetry/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func run() error {
	cfg := provider.DefaultConfig()
	cfg.ServiceName = "composed"
	rec := tracetest.NewSpanRecorder()
	p, e := provider.New(cfg, provider.Dependencies{TraceProcessor: rec})
	if e != nil {
		return e
	}
	defer p.Shutdown(context.Background())
	ctx, parent := p.Tracer("assembler").Start(context.Background(), "parent")
	original := errors.New("original I/O error")
	value, e := client.Do(ctx, client.Invocation{Component: "transport", Operation: "read"}, func(ctx context.Context) (int, error) { return 42, original }, observer.Middleware[int](p.TracerProvider()))
	parent.End()
	if value != 42 || e != original || len(rec.Ended()) != 2 {
		return fmt.Errorf("result/error/span mismatch")
	}
	if rec.Ended()[0].Parent().SpanID() != rec.Ended()[1].SpanContext().SpanID() {
		return fmt.Errorf("parenting mismatch")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("telemetry composed: API-only instrumentation verified")
}

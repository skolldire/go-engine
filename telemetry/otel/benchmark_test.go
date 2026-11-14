package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func BenchmarkChildSpan(b *testing.B) {
	p := create(b, validConfig(), Dependencies{})
	tr := p.Tracer("child")
	ctx, parent := tr.Start(context.Background(), "parent")
	defer parent.End()
	b.ReportAllocs()
	for b.Loop() {
		_, span := tr.Start(ctx, "child")
		span.End()
	}
}
func BenchmarkAttributeLimits(b *testing.B) {
	c := validConfig()
	c.MaxAttributes = 8
	c.MaxAttributeValueLength = 8
	p := create(b, c, Dependencies{})
	tr := p.Tracer("limits")
	attrs := make([]attribute.KeyValue, 16)
	for i := range attrs {
		attrs[i] = attribute.String(string(rune('a'+i)), "a string exceeding eight characters")
	}
	option := trace.WithAttributes(attrs...)
	b.ReportAllocs()
	for b.Loop() {
		_, span := tr.Start(context.Background(), "span", option)
		span.End()
	}
}
func BenchmarkProviderAccess(b *testing.B) {
	p := create(b, validConfig(), Dependencies{})
	b.ReportAllocs()
	for b.Loop() {
		_ = p.Tracer("reused-scope")
		_ = p.Meter("reused-scope")
	}
}
func BenchmarkInstrumentAccess(b *testing.B) {
	p := create(b, validConfig(), Dependencies{})
	meter := p.Meter("reused-scope")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := meter.Int64Counter("reused-counter"); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkIndependentParallelProviders(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		p, err := New(validConfig(), Dependencies{})
		if err != nil {
			b.Error(err)
			return
		}
		defer func() {
			if err := p.Shutdown(context.Background()); err != nil {
				b.Error(err)
			}
		}()
		tr := p.Tracer("independent")
		for pb.Next() {
			_, span := tr.Start(context.Background(), "span")
			span.End()
		}
	})
}

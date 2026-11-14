package otel

import (
	"bytes"
	"context"
	"runtime/pprof"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestNoGoroutineLeaks(t *testing.T) {
	for range 10 {
		p, err := New(validConfig(), Dependencies{TraceProcessor: &processor{}, MetricReader: sdkmetric.NewManualReader()})
		if err != nil {
			t.Fatal(err)
		}
		_, span := p.Tracer("leaks").Start(context.Background(), "span")
		span.End()
		if err := p.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("standard goroutineleak profile missing")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "total 0") {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}

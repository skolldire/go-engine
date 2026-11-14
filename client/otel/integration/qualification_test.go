package integration

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skolldire/go-engine/client"
	clientotel "github.com/skolldire/go-engine/client/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestStressSDKShared(t *testing.T) {
	if os.Getenv("CLIENTOTEL_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	tp, mp, rec, r, closeFn := providers(t, false, true)
	defer closeFn()
	mw, e := clientotel.New[int](config(), clientotel.Dependencies{TracerProvider: tp, MeterProvider: mp})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 1000 {
				v, e := client.Do(context.Background(), info(), func(context.Context) (int, error) { return 42, nil }, mw)
				if v != 42 || e != nil {
					t.Error("result")
				}
			}
		})
	}
	wg.Wait()
	if len(rec.Ended()) != 16000 {
		t.Fatal("span count")
	}
	var rm metricdata.ResourceMetrics
	if e = r.Collect(context.Background(), &rm); e != nil {
		t.Fatal(e)
	}
	hist := rm.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
	if hist.DataPoints[0].Count != 16000 {
		t.Fatal("histogram count")
	}
}
func TestStressReleasedSDKWrappers(t *testing.T) {
	if os.Getenv("CLIENTOTEL_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	defer tp.Shutdown(context.Background())
	r := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r))
	defer mp.Shutdown(context.Background())
	batch := func() {
		for range 500 {
			mw, e := clientotel.New[int](config(), clientotel.Dependencies{TracerProvider: tp, MeterProvider: mp})
			if e != nil {
				t.Fatal(e)
			}
			_, _ = client.Do(context.Background(), info(), func(context.Context) (int, error) { return 1, nil }, mw)
		}
	}
	batch()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 4 {
		batch()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > before.HeapAlloc+8<<20 {
		t.Fatalf("retention before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	}
	t.Logf("2500 SDK wrapper fixture heap before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("profile missing")
	}
	var out bytes.Buffer
	if e := p.WriteTo(&out, 1); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "total 0") {
		t.Fatal(out.String())
	}
	t.Log(out.String())
}
func BenchmarkSDKWrapper(b *testing.B) {
	for _, sampled := range []bool{false, true} {
		b.Run(map[bool]string{false: "unsampled", true: "sampled"}[sampled], func(b *testing.B) {
			sampler := sdktrace.NeverSample()
			if sampled {
				sampler = sdktrace.AlwaysSample()
			}
			tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler))
			defer tp.Shutdown(context.Background())
			r := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r))
			defer mp.Shutdown(context.Background())
			mw, e := clientotel.New[int](config(), clientotel.Dependencies{TracerProvider: tp, MeterProvider: mp})
			if e != nil {
				b.Fatal(e)
			}
			op := func(context.Context) (int, error) { return 42, nil }
			h, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }, mw)
			if e != nil {
				b.Fatal(e)
			}
			b.ReportAllocs()
			for b.Loop() {
				_, _ = h(context.Background(), info(), op)
			}
		})
	}
}
func BenchmarkSDKSpanReference(b *testing.B) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	defer tp.Shutdown(context.Background())
	tr := tp.Tracer("reference")
	b.ReportAllocs()
	for b.Loop() {
		_, span := tr.Start(context.Background(), "storage.read", trace.WithSpanKind(trace.SpanKindInternal))
		span.End()
	}
}

type countingProcessor struct{ count atomic.Int64 }

func (p *countingProcessor) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (p *countingProcessor) OnEnd(sdktrace.ReadOnlySpan)                     { p.count.Add(1) }
func (p *countingProcessor) ForceFlush(context.Context) error                { return nil }
func (p *countingProcessor) Shutdown(context.Context) error                  { return nil }
func BenchmarkSDKProcessor(b *testing.B) {
	processor := &countingProcessor{}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSpanProcessor(processor))
	defer tp.Shutdown(context.Background())
	mw, e := clientotel.New[int](config(), clientotel.Dependencies{TracerProvider: tp})
	if e != nil {
		b.Fatal(e)
	}
	op := func(context.Context) (int, error) { return 42, nil }
	h, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }, mw)
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = h(context.Background(), info(), op)
	}
}
func BenchmarkSDKAPIEquivalent(b *testing.B) {
	now := time.Now
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	defer tp.Shutdown(context.Background())
	r := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r))
	defer mp.Shutdown(context.Background())
	tracer := tp.Tracer("integration")
	histogram, e := mp.Meter("integration").Float64Histogram("go_engine.client.operation.duration", metric.WithUnit("s"), metric.WithDescription("Duration of normally completed logical client operations."))
	if e != nil {
		b.Fatal(e)
	}
	op := func(context.Context) (int, error) { return 42, nil }
	b.ReportAllocs()
	for b.Loop() {
		attrs := [3]attribute.KeyValue{attribute.String("go_engine.client.component", "storage"), attribute.String("go_engine.client.operation", "read")}
		ctx, span := tracer.Start(context.Background(), "storage.read", trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attrs[:2]...))
		start := now()
		_, err := op(ctx)
		seconds := now().Sub(start).Seconds()
		attrs[2] = attribute.Bool("go_engine.client.success", err == nil)
		span.SetAttributes(attrs[2])
		span.End()
		histogram.Record(ctx, seconds, metric.WithAttributes(attrs[:]...))
	}
}

package otel

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"

	api "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestGlobalsRecipeIsolated(t *testing.T) {
	if os.Getenv("GO_ENGINE_OTEL_GLOBAL_RECIPE") == "1" {
		rec := tracetest.NewSpanRecorder()
		r := sdkmetric.NewManualReader()
		p := create(t, validConfig(), Dependencies{rec, r})
		oldTrace, oldMeter := api.GetTracerProvider(), api.GetMeterProvider()
		api.SetTracerProvider(p.TracerProvider())
		api.SetMeterProvider(p.MeterProvider())
		_, s := api.Tracer("recipe").Start(context.Background(), "global span")
		s.End()
		if len(rec.Ended()) != 1 {
			t.Fatal("global recipe failed")
		}
		api.SetTracerProvider(oldTrace)
		api.SetMeterProvider(oldMeter)
		return
	}
	// #nosec G204 G702 -- Re-executes this test binary with a fixed test selector; no user command is accepted.
	cmd := exec.Command(os.Args[0], "-test.run=^TestGlobalsRecipeIsolated$")
	cmd.Env = append(os.Environ(), "GO_ENGINE_OTEL_GLOBAL_RECIPE=1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("recipe subprocess: %v\n%s", e, out)
	}
}
func TestStressLifecycleConcurrent(t *testing.T) {
	if os.Getenv("TELEMETRYOTEL_STRESS") != "1" && os.Getenv("TELEMETRYOTELSTRESS") != "1" {
		t.Skip("expanded campaign")
	}
	for range 100 {
		sp, r := &processor{}, newReader()
		p, e := New(validConfig(), Dependencies{sp, r})
		if e != nil {
			t.Fatal(e)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				tr := p.Tracer("stress")
				meter := p.Meter("stress")
				counter, e := meter.Int64Counter("calls")
				if e != nil {
					t.Error(e)
					return
				}
				for range 100 {
					ctx, s := tr.Start(context.Background(), "span")
					counter.Add(ctx, 1)
					s.End()
				}
			})
		}
		wg.Wait()
		if sp.starts.Load() != 800 || sp.ends.Load() != 800 {
			t.Fatal("missing callbacks")
		}
		for range 8 {
			wg.Go(func() {
				if e := p.Shutdown(context.Background()); e != nil {
					t.Error(e)
				}
			})
		}
		wg.Wait()
		if sp.closes.Load() != 1 || r.closes.Load() != 1 {
			t.Fatal("ownership")
		}
	}
}
func TestStressReleasedMemory(t *testing.T) {
	if os.Getenv("TELEMETRYOTEL_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	batch := func() {
		for range 500 {
			p, e := New(validConfig(), Dependencies{})
			if e != nil {
				t.Fatal(e)
			}
			_, s := p.Tracer("memory").Start(context.Background(), "span")
			s.End()
			if e = p.Shutdown(context.Background()); e != nil {
				t.Fatal(e)
			}
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
	// 2,000 providers are abandoned after closing; allow 8 MiB for SDK/runtime caches.
	if after.HeapAlloc > before.HeapAlloc+8<<20 {
		t.Fatalf("retained heap: before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	}
	t.Logf("closed-instance fixture heap before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	if profile := pprof.Lookup("goroutineleak"); profile != nil {
		if profile.Count() != 0 {
			t.Fatalf("goroutineleak profile: %d", profile.Count())
		}
	}
}
func FuzzConfig(f *testing.F) {
	f.Add("service", 1.0, 128, 4096)
	f.Add("", 0.0, 0, 0)
	f.Fuzz(func(t *testing.T, name string, ratio float64, count, length int) {
		c := Config{ServiceName: name, SamplingRatio: ratio, MaxAttributes: count, MaxAttributeValueLength: length}
		original := c
		_ = c.Validate()
		if c.ServiceName != original.ServiceName || c.MaxAttributes != original.MaxAttributes || c.MaxAttributeValueLength != original.MaxAttributeValueLength {
			t.Fatal("mutated config")
		}
	})
}
func FuzzNativeAttributes(f *testing.F) {
	f.Add("界界", uint8(2), uint8(3))
	f.Fuzz(func(t *testing.T, value string, count, length uint8) {
		if len(value) > 4096 {
			t.Skip()
		}
		c := validConfig()
		c.MaxAttributes = int(count%8) + 1
		c.MaxAttributeValueLength = int(length%32) + 1
		rec := tracetest.NewSpanRecorder()
		p := create(t, c, Dependencies{TraceProcessor: rec})
		_, s := p.Tracer("fuzz").Start(context.Background(), "span", trace.WithAttributes(attribute.String("value", value)))
		s.SetAttributes(attribute.StringSlice("slice", []string{value}))
		s.End()
		if len(rec.Ended()) != 1 || len(rec.Ended()[0].Attributes()) > c.MaxAttributes {
			t.Fatal("count invariant")
		}
	})
}
func FuzzLifecycle(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 1})
	f.Fuzz(func(t *testing.T, seq []byte) {
		if len(seq) > 64 {
			t.Skip()
		}
		sp, r := &processor{}, newReader()
		p, e := New(validConfig(), Dependencies{sp, r})
		if e != nil {
			t.Fatal(e)
		}
		defer p.Shutdown(context.Background())
		for _, op := range seq {
			switch op % 4 {
			case 0:
				_, s := p.Tracer("fuzz").Start(context.Background(), "span")
				s.End()
			case 1:
				if e = p.Shutdown(context.Background()); e != nil {
					t.Fatal(e)
				}
			case 2:
				_ = p.Meter("fuzz")
			case 3:
				_ = p.TracerProvider()
			}
		}
		if sp.closes.Load() > 1 || r.closes.Load() > 1 {
			t.Fatal("multiple shutdowns")
		}
	})
}
func FuzzParentSampling(f *testing.F) {
	f.Add(true, true, uint8(0))
	f.Fuzz(func(t *testing.T, sampled, remote bool, ratio uint8) {
		c := validConfig()
		c.SamplingRatio = float64(ratio) / 255
		rec := tracetest.NewSpanRecorder()
		p := create(t, c, Dependencies{TraceProcessor: rec})
		flags := trace.TraceFlags(0)
		if sampled {
			flags = trace.FlagsSampled
		}
		sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}, TraceFlags: flags, Remote: remote})
		_, s := p.Tracer("fuzz").Start(trace.ContextWithSpanContext(context.Background(), sc), "child")
		if s.IsRecording() != sampled {
			t.Fatal("parent sampling changed")
		}
		s.End()
	})
}
func BenchmarkSpan(b *testing.B) {
	for _, ratio := range []float64{0, 1} {
		name := map[float64]string{0: "off", 1: "on"}[ratio]
		b.Run(name, func(b *testing.B) {
			c := validConfig()
			c.SamplingRatio = ratio
			p := create(b, c, Dependencies{})
			tr := p.Tracer("bench")
			b.ReportAllocs()
			for b.Loop() {
				_, s := tr.Start(context.Background(), "span")
				s.End()
			}
		})
	}
}
func BenchmarkNativeSDKSpan(b *testing.B) {
	p := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1))))
	defer p.Shutdown(context.Background())
	tr := p.Tracer("bench")
	b.ReportAllocs()
	for b.Loop() {
		_, s := tr.Start(context.Background(), "span")
		s.End()
	}
}
func BenchmarkConstructionShutdown(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		p, e := New(validConfig(), Dependencies{})
		if e != nil {
			b.Fatal(e)
		}
		if e = p.Shutdown(context.Background()); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkMetricMeasurement(b *testing.B) {
	r := sdkmetric.NewManualReader()
	p := create(b, validConfig(), Dependencies{MetricReader: r})
	counter, e := p.Meter("bench").Int64Counter("calls")
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	for b.Loop() {
		counter.Add(context.Background(), 1)
	}
}
func BenchmarkParallelSpan(b *testing.B) {
	p := create(b, validConfig(), Dependencies{})
	tr := p.Tracer("parallel")
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, s := tr.Start(context.Background(), "span")
			s.End()
		}
	})
}

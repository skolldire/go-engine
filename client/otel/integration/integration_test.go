package integration

import (
	"context"
	"errors"
	"fmt"

	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/client"
	clientotel "github.com/skolldire/go-engine/client/otel"
	provider "github.com/skolldire/go-engine/telemetry/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func config() clientotel.Config { return clientotel.Config{InstrumentationName: "integration"} }
func info() client.Invocation   { return client.Invocation{Component: "storage", Operation: "read"} }
func providers(t testing.TB, ours, sampled bool) (trace.TracerProvider, metric.MeterProvider, *tracetest.SpanRecorder, *sdkmetric.ManualReader, func()) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	r := sdkmetric.NewManualReader()
	if ours {
		cfg := provider.DefaultConfig()
		cfg.ServiceName = "integration"
		if !sampled {
			cfg.SamplingRatio = 0
		}
		p, e := provider.New(cfg, provider.Dependencies{TraceProcessor: rec, MetricReader: r})
		if e != nil {
			t.Fatal(e)
		}
		closeFn := func() {
			if e := p.Shutdown(context.Background()); e != nil {
				t.Error(e)
			}
		}
		return p.TracerProvider(), p.MeterProvider(), rec, r, closeFn
	}
	sampler := sdktrace.AlwaysSample()
	if !sampled {
		sampler = sdktrace.NeverSample()
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSpanProcessor(rec))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r))
	return tp, mp, rec, r, func() {
		if e := tp.Shutdown(context.Background()); e != nil {
			t.Error(e)
		}
		if e := mp.Shutdown(context.Background()); e != nil {
			t.Error(e)
		}
	}
}
func TestNativeParentStatusHistogram(t *testing.T) {
	for _, ours := range []bool{false, true} {
		for _, sampled := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				tp, mp, rec, r, closeFn := providers(t, ours, sampled)
				defer closeFn()
				mw, e := clientotel.New[int](config(), clientotel.Dependencies{TracerProvider: tp, MeterProvider: mp, IsFailure: func(error) bool { return true }})
				if e != nil {
					t.Fatal(e)
				}
				ctx, parent := tp.Tracer("parent").Start(context.Background(), "parent")
				original := errors.New("secret business error")
				v, err := client.Do(ctx, info(), func(ctx context.Context) (int, error) {
					if sampled && !trace.SpanFromContext(ctx).IsRecording() {
						t.Fatal("not recording")
					}
					time.Sleep(125 * time.Millisecond)
					return 42, original
				}, mw)
				parent.End()
				if v != 42 || err != original {
					t.Fatal("partial result")
				}
				spans := rec.Ended()
				if sampled {
					if len(spans) != 2 {
						t.Fatal(len(spans))
					}
					span := spans[0]
					if span.Name() != "storage.read" || span.SpanKind() != trace.SpanKindInternal || span.Parent().SpanID() != spans[1].SpanContext().SpanID() || span.Status().Code != codes.Error || span.Status().Description != "" || len(span.Events()) != 0 {
						t.Fatal("native trace contract")
					}
					attrs := map[string]attribute.Value{}
					for _, a := range span.Attributes() {
						attrs[string(a.Key)] = a.Value
					}
					if len(attrs) != 3 || attrs["go_engine.client.success"].AsBool() || attrs["go_engine.client.component"].AsString() != "storage" {
						t.Fatal("attributes")
					}
				} else if len(spans) != 0 {
					t.Fatal("sampler ignored")
				}
				var rm metricdata.ResourceMetrics
				if e = r.Collect(context.Background(), &rm); e != nil {
					t.Fatal(e)
				}
				metrics := rm.ScopeMetrics[0].Metrics
				if len(metrics) != 1 || metrics[0].Name != "go_engine.client.operation.duration" || metrics[0].Unit != "s" {
					t.Fatal("extra counter or wrong histogram")
				}
				hist := metrics[0].Data.(metricdata.Histogram[float64])
				if len(hist.DataPoints) != 1 || hist.DataPoints[0].Count != 1 || hist.DataPoints[0].Sum != 0.125 {
					t.Fatal("histogram duration/count")
				}
				if sampled {
					exemplars := hist.DataPoints[0].Exemplars
					if len(exemplars) != 1 || fmt.Sprintf("%x", exemplars[0].TraceID) != spans[0].SpanContext().TraceID().String() {
						t.Fatal("exemplar correlation")
					}
				}
			})
		}
	}
}
func TestStatusPrecedenceAndDefault(t *testing.T) {
	for _, mode := range []string{"default", "false", "true", "caller-ok"} {
		tp, mp, rec, _, closeFn := providers(t, false, true)
		var failure func(error) bool
		if mode != "default" {
			failure = func(error) bool { return mode != "false" }
		}
		mw, e := clientotel.New[int](config(), clientotel.Dependencies{TracerProvider: tp, MeterProvider: mp, IsFailure: failure})
		if e != nil {
			t.Fatal(e)
		}
		_, _ = client.Do(context.Background(), info(), func(ctx context.Context) (int, error) {
			s := trace.SpanFromContext(ctx)
			s.SetAttributes(attribute.String("caller", "kept"))
			if mode == "caller-ok" {
				s.SetStatus(codes.Ok, "")
			}
			return 1, errors.New("not exported")
		}, mw)
		span := rec.Ended()[0]
		want := codes.Unset
		switch mode {
		case "true":
			want = codes.Error
		case "caller-ok":
			want = codes.Ok
		}
		if span.Status().Code != want || span.Status().Description != "" || len(span.Events()) != 0 {
			t.Fatal("status precedence")
		}
		closeFn()
	}
}
func TestDefaultSDKAbnormalEndDoesNotExposePanic(t *testing.T) {
	for _, kind := range []string{"panic", "goexit", "classifier"} {
		t.Run(kind, func(t *testing.T) {
			rec := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
			defer tp.Shutdown(context.Background())
			r := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r))
			defer mp.Shutdown(context.Background())
			token := &struct{ Secret string }{"must-not-be-exported"}
			deps := clientotel.Dependencies{TracerProvider: tp, MeterProvider: mp}
			if kind == "classifier" {
				deps.IsFailure = func(error) bool { panic(token) }
			}
			mw, e := clientotel.New[int](config(), deps)
			if e != nil {
				t.Fatal(e)
			}
			done := make(chan any, 1)
			go func() {
				defer func() { done <- recover() }()
				_, _ = client.Do(context.Background(), info(), func(context.Context) (int, error) {
					if kind == "goexit" {
						runtime.Goexit()
					}
					if kind == "panic" {
						panic(token)
					}
					return 1, errors.New("not exported")
				}, mw)
			}()
			v := <-done
			if kind != "goexit" && v != token {
				t.Fatal("panic changed")
			}
			spans := rec.Ended()
			if len(spans) != 1 || len(spans[0].Events()) != 0 {
				t.Fatal("span not closed or panic event exposed")
			}
			var rm metricdata.ResourceMetrics
			if e = r.Collect(context.Background(), &rm); e != nil {
				t.Fatal(e)
			}
			for _, scope := range rm.ScopeMetrics {
				for _, m := range scope.Metrics {
					if len(m.Data.(metricdata.Histogram[float64]).DataPoints) != 0 {
						t.Fatal("abnormal completion measured")
					}
				}
			}
		})
	}
}

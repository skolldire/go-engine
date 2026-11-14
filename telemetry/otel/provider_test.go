package otel

import (
	"context"
	"errors"
	"math"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	api "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func validConfig() Config { c := DefaultConfig(); c.ServiceName = "test-service"; return c }
func create(t testing.TB, c Config, d Dependencies) *Provider {
	t.Helper()
	p, e := New(c, d)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := p.Shutdown(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return p
}
func TestConfig(t *testing.T) {
	d := DefaultConfig()
	if d.ServiceName != "" || d.SamplingRatio != 1 || d.MaxAttributes != 128 || d.MaxAttributeValueLength != 4096 {
		t.Fatal(d)
	}
	cases := []struct {
		field string
		edit  func(*Config)
	}{{"ServiceName", func(c *Config) { c.ServiceName = ""; c.MaxAttributes = 0 }}, {"SamplingRatio", func(c *Config) { c.SamplingRatio = math.NaN() }}, {"SamplingRatio", func(c *Config) { c.SamplingRatio = math.Inf(1) }}, {"SamplingRatio", func(c *Config) { c.SamplingRatio = -1 }}, {"SamplingRatio", func(c *Config) { c.SamplingRatio = 1.1 }}, {"MaxAttributes", func(c *Config) { c.MaxAttributes = 0 }}, {"MaxAttributeValueLength", func(c *Config) { c.MaxAttributeValueLength = -1 }}}
	for _, tt := range cases {
		c := validConfig()
		tt.edit(&c)
		e := c.Validate()
		var ce *ConfigError
		if !errors.Is(e, ErrInvalidConfig) || !errors.As(e, &ce) || ce.Field != tt.field {
			t.Fatalf("field %s: %v", tt.field, e)
		}
	}
	for _, ratio := range []float64{0, 1} {
		c := validConfig()
		c.ServiceName = " "
		c.SamplingRatio = ratio
		if e := c.Validate(); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSignalsResourceAndMetrics(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	for mask := range 4 {
		t.Run(string(rune('0'+mask)), func(t *testing.T) {
			r := sdkmetric.NewManualReader()
			rec := tracetest.NewSpanRecorder()
			deps := Dependencies{}
			if mask&1 != 0 {
				deps.TraceProcessor = rec
			}
			if mask&2 != 0 {
				deps.MetricReader = r
			}
			c := validConfig()
			c.ServiceVersion = "1.2.3"
			c.Environment = "test"
			p := create(t, c, deps)
			if p.TracerProvider() == nil || p.MeterProvider() == nil {
				t.Fatal("nil API provider")
			}
			_, s := p.TracerProvider().Tracer("api").Start(context.Background(), "span")
			s.End()
			counter, e := p.MeterProvider().Meter("api").Int64Counter("calls")
			if e != nil {
				t.Fatal(e)
			}
			counter.Add(context.Background(), 3)
			if mask&1 != 0 {
				spans := rec.Ended()
				if len(spans) != 1 {
					t.Fatal(len(spans))
				}
				checkResource(t, spans[0].Resource().Attributes())
			}
			if mask&2 != 0 {
				var rm metricdata.ResourceMetrics
				if e := r.Collect(context.Background(), &rm); e != nil {
					t.Fatal(e)
				}
				checkResource(t, rm.Resource.Attributes())
				sum := rm.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64])
				if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 3 {
					t.Fatal(sum)
				}
			}
		})
	}
}
func checkResource(t *testing.T, attrs []attribute.KeyValue) {
	t.Helper()
	got := map[string]string{}
	for _, a := range attrs {
		got[string(a.Key)] = a.Value.AsString()
	}
	want := map[string]string{"service.name": "test-service", "service.version": "1.2.3", "deployment.environment.name": "test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resource %v", got)
	}
}
func TestSamplingParents(t *testing.T) {
	for _, ratio := range []float64{0, 1} {
		for _, parent := range []string{"root", "local-sampled", "remote-sampled", "local-unsampled", "remote-unsampled"} {
			t.Run(parent+string(rune('0'+int(ratio))), func(t *testing.T) {
				rec := tracetest.NewSpanRecorder()
				c := validConfig()
				c.SamplingRatio = ratio
				p := create(t, c, Dependencies{TraceProcessor: rec})
				ctx := context.Background()
				want := ratio == 1
				if parent != "root" {
					sampled := parent == "local-sampled" || parent == "remote-sampled"
					flags := trace.TraceFlags(0)
					if sampled {
						flags = trace.FlagsSampled
					}
					sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: flags, Remote: parent == "remote-sampled" || parent == "remote-unsampled"})
					ctx = trace.ContextWithSpanContext(ctx, sc)
					want = sampled
				}
				_, s := p.Tracer("sampling").Start(ctx, "span")
				if s.IsRecording() != want {
					t.Fatal("sampling policy")
				}
				s.End()
				if (len(rec.Ended()) == 1) != want {
					t.Fatal("recording mismatch")
				}
			})
		}
	}
}
func TestNativeLimitsAndPreservation(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	c := validConfig()
	c.MaxAttributes = 3
	c.MaxAttributeValueLength = 1
	p := create(t, c, Dependencies{TraceProcessor: rec})
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	link := trace.Link{SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}}), Attributes: []attribute.KeyValue{attribute.String("link", "界界界")}}
	ctx, s := p.Tracer("limits").Start(context.Background(), "span", trace.WithTimestamp(start), trace.WithAttributes(attribute.String("start", "界界界")), trace.WithLinks(link))
	s.SetAttributes(attribute.StringSlice("slice", []string{"界界", "abc"}))
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("context", "界界"))
	s.AddEvent("event", trace.WithAttributes(attribute.String("event", "界界界")))
	s.RecordError(errors.New("exception remains full"))
	s.AddLink(link)
	s.SetStatus(codes.Error, "original status")
	s.End(trace.WithTimestamp(start.Add(time.Second)))
	got := rec.Ended()[0]
	if !got.StartTime().Equal(start) || !got.EndTime().Equal(start.Add(time.Second)) || got.Status().Description != "original status" {
		t.Fatal("timestamps/status changed")
	}
	for _, a := range got.Attributes() {
		if a.Key == "slice" {
			if !reflect.DeepEqual(a.Value.AsStringSlice(), []string{"界", "a"}) {
				t.Fatal(a)
			}
		} else if a.Value.AsString() != "界" {
			t.Fatal(a)
		}
	}
	if got.Events()[0].Attributes[0].Value.AsString() != "界界界" {
		t.Fatal("event scope changed")
	}
	if got.Links()[0].Attributes[0].Value.AsString() != "界界界" || len(got.Links()) != 2 {
		t.Fatal("link scope changed")
	}
	found := false
	for _, a := range got.Events()[1].Attributes {
		if a.Key == "exception.message" && a.Value.AsString() == "exception remains full" {
			found = true
		}
	}
	if !found {
		t.Fatal("exception message changed")
	}
	_, s = p.Tracer("limits").Start(context.Background(), "counts", trace.WithAttributes(attribute.Int("a", 1), attribute.Int("b", 2), attribute.Int("c", 3), attribute.Int("d", 4)))
	for range 130 {
		s.AddEvent("event", trace.WithAttributes(attribute.Int("a", 1), attribute.Int("b", 2), attribute.Int("c", 3), attribute.Int("d", 4)))
		l := link
		l.Attributes = append(l.Attributes, attribute.Int("a", 1), attribute.Int("b", 2), attribute.Int("c", 3))
		s.AddLink(l)
	}
	s.End()
	got = rec.Ended()[1]
	if len(got.Attributes()) != 3 || got.DroppedAttributes() != 1 || len(got.Events()) != 128 || got.DroppedEvents() != 2 || len(got.Links()) != 128 || got.DroppedLinks() != 2 {
		t.Fatalf("limits %d/%d/%d", len(got.Attributes()), len(got.Events()), len(got.Links()))
	}
	if len(got.Events()[0].Attributes) != 3 || len(got.Links()[0].Attributes) != 3 {
		t.Fatal("nested count limit")
	}
}
func TestEnvironmentAndGlobalsUnchanged(t *testing.T) {
	for k, v := range map[string]string{"OTEL_RESOURCE_ATTRIBUTES": "injected=value", "OTEL_SERVICE_NAME": "injected", "OTEL_TRACES_SAMPLER": "always_off", "OTEL_TRACES_SAMPLER_ARG": "0", "OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT": "1", "OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT": "1", "OTEL_SPAN_EVENT_COUNT_LIMIT": "1", "OTEL_SPAN_LINK_COUNT_LIMIT": "1", "OTEL_EVENT_ATTRIBUTE_COUNT_LIMIT": "1", "OTEL_LINK_ATTRIBUTE_COUNT_LIMIT": "1", "OTEL_METRICS_EXEMPLAR_FILTER": "always_off", "OTEL_GO_X_CARDINALITY_LIMIT": "1"} {
		t.Setenv(k, v)
	}
	tp, mp := api.GetTracerProvider(), api.GetMeterProvider()
	rec := tracetest.NewSpanRecorder()
	r := sdkmetric.NewManualReader()
	c := validConfig()
	p := create(t, c, Dependencies{TraceProcessor: rec, MetricReader: r})
	_, s := p.Tracer("env").Start(context.Background(), "span", trace.WithAttributes(attribute.String("a", "long"), attribute.String("b", "long")))
	for range 2 {
		s.AddEvent("event", trace.WithAttributes(attribute.Int("a", 1), attribute.Int("b", 2)))
		s.AddLink(trace.Link{SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}}), Attributes: []attribute.KeyValue{attribute.Int("a", 1), attribute.Int("b", 2)}})
	}
	s.End()
	if api.GetTracerProvider() != tp || api.GetMeterProvider() != mp {
		t.Fatal("globals changed")
	}
	sp := rec.Ended()
	if len(sp) != 1 || len(sp[0].Attributes()) != 2 || sp[0].Attributes()[0].Value.AsString() != "long" {
		t.Fatal("environment affected effective trace policy")
	}
	if len(sp[0].Events()) != 2 || len(sp[0].Links()) != 2 || len(sp[0].Events()[0].Attributes) != 2 || len(sp[0].Links()[0].Attributes) != 2 {
		t.Fatal("environment affected event/link count limits")
	}
	counter, e := p.Meter("env").Int64Counter("calls")
	if e != nil {
		t.Fatal(e)
	}
	for i := range 3 {
		counter.Add(context.Background(), 1, apiMetricAttrs(i))
	}
	var rm metricdata.ResourceMetrics
	if e = r.Collect(context.Background(), &rm); e != nil {
		t.Fatal(e)
	}
	if len(rm.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints) != 3 {
		t.Fatal("environment cardinality applied")
	}
}

func TestResourceEnvironmentMergeConfigWinsBothSignals(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.name=attributes-name,service.version=attributes-version,deployment.environment.name=attributes-env,extra.key=extra-value")
	t.Setenv("OTEL_SERVICE_NAME", "environment-name")
	c := validConfig()
	c.ServiceVersion = "config-version"
	c.Environment = "config-env"
	rec, r := tracetest.NewSpanRecorder(), sdkmetric.NewManualReader()
	p := create(t, c, Dependencies{TraceProcessor: rec, MetricReader: r})
	_, span := p.Tracer("resource").Start(context.Background(), "span")
	span.End()
	counter, err := p.Meter("resource").Int64Counter("calls")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(context.Background(), 1)
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"service.name": c.ServiceName, "service.version": c.ServiceVersion, "deployment.environment.name": c.Environment, "extra.key": "extra-value"}
	for _, attrs := range [][]attribute.KeyValue{rec.Ended()[0].Resource().Attributes(), rm.Resource.Attributes()} {
		got := make(map[string]string)
		for _, a := range attrs {
			got[string(a.Key)] = a.Value.AsString()
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resource merge: got %v want %v", got, want)
		}
	}
}

type processor struct {
	starts, ends, closes atomic.Int64
	shutdown             func(context.Context) error
}

func (p *processor) OnStart(context.Context, sdktrace.ReadWriteSpan) { p.starts.Add(1) }
func (p *processor) OnEnd(sdktrace.ReadOnlySpan)                     { p.ends.Add(1) }
func (p *processor) ForceFlush(context.Context) error                { return nil }
func (p *processor) Shutdown(ctx context.Context) error {
	p.closes.Add(1)
	if p.shutdown != nil {
		return p.shutdown(ctx)
	}
	return nil
}

type reader struct {
	sdkmetric.Reader
	closes   atomic.Int64
	shutdown func(context.Context) error
}

func (r *reader) Shutdown(ctx context.Context) error {
	r.closes.Add(1)
	base := r.Reader.Shutdown(ctx)
	if r.shutdown != nil {
		return errors.Join(base, r.shutdown(ctx))
	}
	return base
}
func newReader() *reader { return &reader{Reader: sdkmetric.NewManualReader()} }
func TestInvalidConstructionDoesNotTakeOwnership(t *testing.T) {
	sp, r := &processor{}, newReader()
	if p, e := New(DefaultConfig(), Dependencies{sp, r}); p != nil || !errors.Is(e, ErrInvalidConfig) {
		t.Fatal(p, e)
	}
	if sp.closes.Load() != 0 || r.closes.Load() != 0 {
		t.Fatal("closed borrowed dependencies")
	}
	if e := r.Shutdown(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestShutdownErrorsCachedBothSignals(t *testing.T) {
	a, b := errors.New("trace failure"), errors.New("metric failure")
	sp, r := &processor{}, newReader()
	sp.shutdown = func(context.Context) error { return a }
	r.shutdown = func(context.Context) error { return b }
	p, e := New(validConfig(), Dependencies{sp, r})
	if e != nil {
		t.Fatal(e)
	}
	first := p.Shutdown(context.Background())
	if !errors.Is(first, a) || !errors.Is(first, b) {
		t.Fatal(first)
	}
	for range 3 {
		if p.Shutdown(context.Background()) != first {
			t.Fatal("result not cached")
		}
	}
	if sp.closes.Load() != 1 || r.closes.Load() != 1 {
		t.Fatal("double shutdown")
	}
	//nolint:staticcheck // Nil context intentionally verifies the public validation contract.
	if !errors.Is(p.Shutdown(nil), ErrNilContext) {
		t.Fatal("nil context")
	}
}
func TestConcurrentShutdownAndOwnContext(t *testing.T) {
	entered, gate := make(chan struct{}), make(chan struct{})
	sp, r := &processor{}, newReader()
	sp.shutdown = func(context.Context) error { close(entered); <-gate; return nil }
	p, e := New(validConfig(), Dependencies{sp, r})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- p.Shutdown(context.Background()) }()
	<-entered
	cause := errors.New("waiting caller canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	e = p.Shutdown(ctx)
	if !errors.Is(e, cause) || !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Go(func() { errs <- p.Shutdown(context.Background()) })
	}
	close(gate)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	wg.Wait()
	close(errs)
	for e = range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if sp.closes.Load() != 1 || r.closes.Load() != 1 {
		t.Fatal("callbacks repeated")
	}
}
func TestShutdownCanceledFirstNoRetry(t *testing.T) {
	sp, r := &processor{}, newReader()
	p, e := New(validConfig(), Dependencies{sp, r})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	first := p.Shutdown(ctx)
	if !errors.Is(first, context.Canceled) {
		t.Fatal(first)
	}
	if sp.closes.Load() != 0 || r.closes.Load() != 1 {
		t.Fatal("SDK canceled-close semantics")
	}
	if p.Shutdown(context.Background()) != first || sp.closes.Load() != 0 {
		t.Fatal("unexpected retry")
	}
}
func TestNonCooperativeShutdownNotAbandoned(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		sp := &processor{shutdown: func(context.Context) error { <-gate; return nil }}
		p, e := New(validConfig(), Dependencies{TraceProcessor: sp})
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- p.Shutdown(ctx) }()
		synctest.Wait()
		time.Sleep(2 * time.Second)
		select {
		case <-done:
			t.Fatal("callback abandoned")
		default:
		}
		close(gate)
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	})
}
func TestAbnormalShutdownStillClosesMetrics(t *testing.T) {
	for _, goexit := range []bool{false, true} {
		t.Run(map[bool]string{false: "panic", true: "goexit"}[goexit], func(t *testing.T) {
			token := &struct{}{}
			sp, r := &processor{}, newReader()
			sp.shutdown = func(context.Context) error {
				if goexit {
					runtime.Goexit()
				}
				panic(token)
			}
			p, e := New(validConfig(), Dependencies{sp, r})
			if e != nil {
				t.Fatal(e)
			}
			done := make(chan any, 1)
			go func() { defer func() { done <- recover() }(); _ = p.Shutdown(context.Background()) }()
			v := <-done
			if !goexit && v != token {
				t.Fatal("panic changed")
			}
			if r.closes.Load() != 1 {
				t.Fatal("metric not attempted")
			}
			if p.Shutdown(context.Background()) == nil {
				t.Fatal("abnormal shutdown hidden")
			}
		})
	}
}

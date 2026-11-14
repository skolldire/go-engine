package otel

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/client"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

type traceCapture struct {
	trace.TracerProvider
	mu    sync.Mutex
	names []string
	spans []*spanCapture
	onEnd func()
}

func newTraceCapture() *traceCapture {
	return &traceCapture{TracerProvider: tracenoop.NewTracerProvider()}
}
func (p *traceCapture) Tracer(name string, _ ...trace.TracerOption) trace.Tracer {
	p.mu.Lock()
	p.names = append(p.names, name)
	p.mu.Unlock()
	return &tracerCapture{Tracer: p.TracerProvider.Tracer(name), owner: p}
}

type tracerCapture struct {
	trace.Tracer
	owner *traceCapture
}

func (t *tracerCapture) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	_, base := t.Tracer.Start(ctx, name)
	c := trace.NewSpanStartConfig(opts...)
	p := t.owner
	p.mu.Lock()
	s := &spanCapture{Span: base, name: name, kind: c.SpanKind(), attrs: append([]attribute.KeyValue(nil), c.Attributes()...), parent: trace.SpanFromContext(ctx).SpanContext(), onEnd: p.onEnd}
	s.sc = trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{byte(len(p.spans) + 1)}, TraceFlags: trace.FlagsSampled})
	p.spans = append(p.spans, s)
	p.mu.Unlock()
	return trace.ContextWithSpan(ctx, s), s
}

type spanCapture struct {
	trace.Span
	mu                 sync.Mutex
	name               string
	kind               trace.SpanKind
	attrs              []attribute.KeyValue
	parent, sc         trace.SpanContext
	ends, recordErrors atomic.Int64
	statuses           []codes.Code
	descriptions       []string
	onEnd              func()
}

func (s *spanCapture) SpanContext() trace.SpanContext { return s.sc }
func (s *spanCapture) SetAttributes(attrs ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs = append(s.attrs, attrs...)
}
func (s *spanCapture) SetStatus(code codes.Code, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, code)
	s.descriptions = append(s.descriptions, description)
}
func (s *spanCapture) RecordError(error, ...trace.EventOption) { s.recordErrors.Add(1) }
func (s *spanCapture) End(...trace.SpanEndOption) {
	s.ends.Add(1)
	if s.onEnd != nil {
		s.onEnd()
	}
}

type metricCapture struct {
	metric.MeterProvider
	mu           sync.Mutex
	names        []string
	creations    int
	instrument   string
	config       metric.Float64HistogramConfig
	err          error
	measurements []measurement
	onRecord     func()
}
type measurement struct {
	ctx   context.Context
	value float64
	attrs attribute.Set
}

func newMetricCapture() *metricCapture {
	return &metricCapture{MeterProvider: metricnoop.NewMeterProvider()}
}
func (p *metricCapture) Meter(name string, _ ...metric.MeterOption) metric.Meter {
	p.mu.Lock()
	p.names = append(p.names, name)
	p.mu.Unlock()
	return &meterCapture{Meter: p.MeterProvider.Meter(name), owner: p}
}

type meterCapture struct {
	metric.Meter
	owner *metricCapture
}

func (m *meterCapture) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	p := m.owner
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creations++
	p.instrument = name
	p.config = metric.NewFloat64HistogramConfig(opts...)
	if p.err != nil {
		return nil, p.err
	}
	base, _ := m.Meter.Float64Histogram(name, opts...)
	return &histogramCapture{Float64Histogram: base, owner: p}, nil
}

type histogramCapture struct {
	metric.Float64Histogram
	owner *metricCapture
}

func (h *histogramCapture) Record(ctx context.Context, value float64, opts ...metric.RecordOption) {
	c := metric.NewRecordConfig(opts)
	p := h.owner
	p.mu.Lock()
	p.measurements = append(p.measurements, measurement{ctx, value, c.Attributes()})
	p.mu.Unlock()
	if p.onRecord != nil {
		p.onRecord()
	}
}
func invocation() client.Invocation {
	return client.Invocation{Component: "storage", Operation: "read"}
}
func makeMiddleware[T any](t testing.TB, deps Dependencies) client.Middleware[T] {
	t.Helper()
	mw, e := New[T](Config{"test.instrumentation"}, deps)
	if e != nil {
		t.Fatal(e)
	}
	return mw
}
func attrsMap(attrs []attribute.KeyValue) map[string]any {
	out := map[string]any{}
	for _, a := range attrs {
		out[string(a.Key)] = a.Value.AsInterface()
	}
	return out
}
func TestConstructionAndHistogramError(t *testing.T) {
	tp, mp := newTraceCapture(), newMetricCapture()
	cfg := Config{"original"}
	mw, e := New[int](cfg, Dependencies{tp, mp, nil})
	if e != nil || mw == nil {
		t.Fatal(e)
	}
	cfg.InstrumentationName = "mutated"
	if len(tp.spans) != 0 || len(mp.measurements) != 0 || mp.creations != 1 || tp.names[0] != "original" || mp.names[0] != "original" {
		t.Fatal("construction effects/capture")
	}
	if mp.instrument != durationName || mp.config.Unit() != "s" || mp.config.Description() != durationDescription {
		t.Fatal("instrument contract")
	}
	if mw(nil) != nil {
		t.Fatal("next nil")
	}
	original := &testError{}
	mp = newMetricCapture()
	mp.err = original
	mw, e = New[int](Config{"name"}, Dependencies{MeterProvider: mp})
	var typed *testError
	if mw != nil || !errors.Is(e, original) || !errors.As(e, &typed) || errors.Is(e, ErrInvalidConfig) || mp.creations != 1 {
		t.Fatal("creation cause", e)
	}
	var nilMP *metricCapture
	var nilTP *traceCapture
	for _, tt := range []struct {
		deps  Dependencies
		field string
	}{{Dependencies{MeterProvider: nilMP}, "MeterProvider"}, {Dependencies{TracerProvider: nilTP, MeterProvider: nilMP}, "TracerProvider"}} {
		_, e = New[int](Config{"name"}, tt.deps)
		var ce *ConfigError
		if !errors.As(e, &ce) || ce.Field != tt.field {
			t.Fatal(e)
		}
	}
}

type testError struct{}

func (*testError) Error() string { return "typed operation failure" }

type secretError struct{}

func (secretError) Error() string { panic("error text inspected") }

type secretResult struct{ N int }

func (secretResult) String() string { panic("result inspected") }
func TestNormalResultsStatusAndNoInspection(t *testing.T) {
	for _, classify := range []int{0, 1, 2} {
		for _, failed := range []bool{false, true} {
			t.Run(string(rune('0'+classify))+map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
				tp, mp := newTraceCapture(), newMetricCapture()
				calls := 0
				deps := Dependencies{tp, mp, nil}
				if classify != 0 {
					deps.IsFailure = func(err error) bool {
						calls++
						if err == nil {
							t.Fatal("classified success")
						}
						return classify == 2
					}
				}
				mw := makeMiddleware[secretResult](t, deps)
				var original error
				if failed {
					original = secretError{}
				}
				result, err := client.Do(context.Background(), invocation(), func(context.Context) (secretResult, error) { return secretResult{42}, original }, mw)
				if result.N != 42 || err != original {
					t.Fatal("result/error changed")
				}
				if calls != map[bool]int{false: 0, true: 1}[failed && classify != 0] {
					t.Fatal("classification count")
				}
				s := tp.spans[0]
				if s.name != "storage.read" || s.kind != trace.SpanKindInternal || s.ends.Load() != 1 || s.recordErrors.Load() != 0 {
					t.Fatal("span contract")
				}
				want := map[string]any{componentKey: "storage", operationKey: "read", successKey: !failed}
				if !reflect.DeepEqual(attrsMap(s.attrs), want) || !reflect.DeepEqual(attrsMap(mp.measurements[0].attrs.ToSlice()), want) {
					t.Fatal("attributes changed")
				}
				if len(mp.measurements) != 1 || mp.measurements[0].value < 0 {
					t.Fatal("histogram")
				}
				if failed && classify == 2 {
					if !reflect.DeepEqual(s.statuses, []codes.Code{codes.Error}) || s.descriptions[0] != "" {
						t.Fatal("status")
					}
				} else if len(s.statuses) != 0 {
					t.Fatal("automatic status")
				}
			})
		}
	}
}
func TestContextPreservation(t *testing.T) {
	type contextKey struct{}
	member, e := baggage.NewMember("key", "value")
	if e != nil {
		t.Fatal(e)
	}
	bag, e := baggage.New(member)
	if e != nil {
		t.Fatal(e)
	}
	cause := errors.New("caller cause")
	base := context.WithValue(context.Background(), contextKey{}, "value")
	base = baggage.ContextWithBaggage(base, bag)
	deadline := time.Now().Add(time.Hour)
	base, stop := context.WithDeadline(base, deadline)
	defer stop()
	ctx, cancel := context.WithCancelCause(base)
	cancel(cause)
	for _, tracing := range []bool{false, true} {
		tp, mp := newTraceCapture(), newMetricCapture()
		deps := Dependencies{MeterProvider: mp}
		if tracing {
			deps.TracerProvider = tp
		}
		mw := makeMiddleware[int](t, deps)
		value, err := client.Do(ctx, invocation(), func(got context.Context) (int, error) {
			if got.Value(contextKey{}) != "value" || context.Cause(got) != cause || baggage.FromContext(got).Member("key").Value() != "value" {
				t.Fatal("context contents")
			}
			d, ok := got.Deadline()
			if !ok || d != deadline {
				t.Fatal("deadline")
			}
			if !tracing && got != ctx {
				t.Fatal("metrics-only identity")
			}
			if tracing && !trace.SpanFromContext(got).SpanContext().IsValid() {
				t.Fatal("missing active span")
			}
			return 7, nil
		}, mw)
		if value != 7 || err != nil {
			t.Fatal("success changed after cancellation")
		}
		if tracing && !trace.SpanFromContext(mp.measurements[0].ctx).SpanContext().Equal(tp.spans[0].sc) {
			t.Fatal("measurement correlation")
		}
	}
}
func TestOneClockAndFinalizationExcluded(t *testing.T) {
	tp, mp := newTraceCapture(), newMetricCapture()
	tr := tp.Tracer("scope")
	hist, e := mp.Meter("scope").Float64Histogram(durationName)
	if e != nil {
		t.Fatal(e)
	}
	clock := time.Unix(1, 0)
	reads := 0
	now := func() time.Time { reads++; return clock }
	tp.onEnd = func() { clock = clock.Add(time.Hour) }
	mp.onRecord = func() { clock = clock.Add(time.Hour) }
	mw := middleware[int](tr, hist, nil, now)
	_, e = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { clock = clock.Add(125 * time.Millisecond); return 1, nil }, mw)
	if e != nil || reads != 2 || mp.measurements[0].value != 0.125 {
		t.Fatal("duration clock/finalization")
	}
	mw = middleware[int](tr, nil, nil, func() time.Time { t.Fatal("unneeded clock"); return time.Time{} })
	_, e = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
	if e != nil {
		t.Fatal(e)
	}
}
func TestAbnormalCompletionClosesSpan(t *testing.T) {
	for _, kind := range []string{"panic", "goexit", "classifier"} {
		t.Run(kind, func(t *testing.T) {
			tp, mp := newTraceCapture(), newMetricCapture()
			token := &struct{}{}
			deps := Dependencies{tp, mp, nil}
			if kind == "classifier" {
				deps.IsFailure = func(error) bool { panic(token) }
			}
			mw := makeMiddleware[int](t, deps)
			done := make(chan any, 1)
			go func() {
				defer func() { done <- recover() }()
				_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) {
					if kind == "goexit" {
						runtime.Goexit()
					}
					if kind == "panic" {
						panic(token)
					}
					return 9, secretError{}
				}, mw)
			}()
			v := <-done
			if kind != "goexit" && v != token {
				t.Fatal("panic identity")
			}
			if len(tp.spans) != 1 || tp.spans[0].ends.Load() != 1 || len(mp.measurements) != 0 {
				t.Fatal("abnormal finalization")
			}
			if kind != "classifier" {
				if _, exists := attrsMap(tp.spans[0].attrs)[successKey]; exists {
					t.Fatal("fabricated success")
				}
			}
		})
	}
}
func TestSignalsNoopTypesAndChain(t *testing.T) {
	for _, mask := range []int{1, 2, 3} {
		deps := Dependencies{}
		if mask&1 != 0 {
			deps.TracerProvider = tracenoop.NewTracerProvider()
		}
		if mask&2 != 0 {
			deps.MeterProvider = metricnoop.NewMeterProvider()
		}
		mw := makeMiddleware[*int](t, deps)
		want := 42
		original := &testError{}
		op := func(context.Context) (*int, error) { return &want, original }
		handler, e := client.Chain(func(ctx context.Context, _ client.Invocation, op client.Operation[*int]) (*int, error) {
			return op(ctx)
		}, mw)
		if e != nil {
			t.Fatal(e)
		}
		v, err := handler(context.Background(), invocation(), op)
		var typed *testError
		if v != &want || err != original || !errors.Is(err, original) || !errors.As(err, &typed) {
			t.Fatal("pointer/typed error identity")
		}
	}
	mp := newMetricCapture()
	mw := makeMiddleware[[]int](t, Dependencies{MeterProvider: mp})
	want := []int{1, 2}
	got, e := client.Do(context.Background(), invocation(), func(context.Context) ([]int, error) { return want, nil }, mw)
	if e != nil || &got[0] != &want[0] {
		t.Fatal("collection copied")
	}
}
func TestCallbacksAndConcurrentReuse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, gate := make(chan struct{}), make(chan struct{})
		mp := newMetricCapture()
		mp.onRecord = func() { close(entered); <-gate }
		mw := makeMiddleware[int](t, Dependencies{MeterProvider: mp})
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 1, nil }, mw)
		}()
		<-entered
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("callback abandoned")
		default:
		}
		close(gate)
		<-done
	})
	tp, mp := newTraceCapture(), newMetricCapture()
	mw := makeMiddleware[int](t, Dependencies{tp, mp, nil})
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 20 {
				v, e := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 42, nil }, mw)
				if v != 42 || e != nil {
					t.Error("result")
				}
			}
		})
	}
	wg.Wait()
	if len(tp.spans) != 640 || len(mp.measurements) != 640 || mp.creations != 1 {
		t.Fatal("concurrent counts")
	}
	for _, s := range tp.spans {
		if s.ends.Load() != 1 {
			t.Fatal("span not closed")
		}
	}
}

func TestBackendPanicsDoNotRepeatEnd(t *testing.T) {
	for _, stage := range []string{"end", "record"} {
		t.Run(stage, func(t *testing.T) {
			tp, mp := newTraceCapture(), newMetricCapture()
			token := &struct{}{}
			if stage == "end" {
				tp.onEnd = func() { panic(token) }
			} else {
				mp.onRecord = func() { panic(token) }
			}
			mw := makeMiddleware[int](t, Dependencies{tp, mp, nil})
			called := false
			func() {
				defer func() {
					if recover() != token {
						t.Error("backend panic changed")
					}
				}()
				_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { called = true; return 42, nil }, mw)
			}()
			if !called || tp.spans[0].ends.Load() != 1 {
				t.Fatal("I/O/end count")
			}
		})
	}
}
func TestMiddlewareOrderAndInvocation(t *testing.T) {
	tp, mp := newTraceCapture(), newMetricCapture()
	var events []string
	tp.onEnd = func() { events = append(events, "span.end") }
	mp.onRecord = func() { events = append(events, "histogram") }
	mw := makeMiddleware[int](t, Dependencies{tp, mp, nil})
	other := func(next client.Handler[int]) client.Handler[int] {
		return func(ctx context.Context, info client.Invocation, op client.Operation[int]) (int, error) {
			events = append(events, "inner.before")
			v, e := next(ctx, info, op)
			events = append(events, "inner.after")
			return v, e
		}
	}
	original := invocation()
	_, e := client.Do(context.Background(), original, func(context.Context) (int, error) { events = append(events, "operation"); return 1, nil }, mw, other)
	if e != nil || original != invocation() || !reflect.DeepEqual(events, []string{"inner.before", "operation", "inner.after", "span.end", "histogram"}) {
		t.Fatal("composition order/invocation changed")
	}
}

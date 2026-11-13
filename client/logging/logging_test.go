package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/client"
	clientlogging "github.com/skolldire/go-engine/client/logging"
)

type event struct {
	ctx     context.Context
	level   slog.Level
	message string
	attrs   []slog.Attr
}
type sliceLogger struct {
	mu           sync.Mutex
	entries      []event
	enabled      bool
	enabledCalls int
}

func (l *sliceLogger) Enabled(context.Context, slog.Level) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabledCalls++
	return l.enabled
}
func (l *sliceLogger) LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, event{ctx, level, msg, append([]slog.Attr(nil), attrs...)})
}
func (l *sliceLogger) snapshot() []event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]event(nil), l.entries...)
}
func attrs(e event) map[string]any {
	m := make(map[string]any, len(e.attrs))
	for _, a := range e.attrs {
		m[a.Key] = a.Value.Any()
	}
	return m
}
func newMiddleware[T any](t testing.TB, cfg clientlogging.Config, logger clientlogging.Logger) client.Middleware[T] {
	t.Helper()
	mw, err := clientlogging.New[T](cfg, clientlogging.Dependencies{Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	return mw
}
func terminal[T any](ctx context.Context, _ client.Invocation, op client.Operation[T]) (T, error) {
	return op(ctx)
}
func invocation() client.Invocation { return client.Invocation{Component: "memory", Operation: "read"} }

type domainError struct{ cause error }

func (*domainError) Error() string   { panic("error text must not be inspected") }
func (e *domainError) Unwrap() error { return e.cause }

type privateResult struct{ value int }

func (privateResult) String() string { panic("result must not be inspected") }

func TestCompletionAndExactPassthrough(t *testing.T) {
	for _, level := range []string{"debug", "info"} {
		for _, duration := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				logger := &sliceLogger{enabled: true}
				cfg := clientlogging.Config{Level: level, IncludeDuration: duration}
				mw := newMiddleware[privateResult](t, cfg, logger)
				cfg.Level = "invalid"
				cfg.IncludeDuration = !duration
				expected := privateResult{value: 7}
				sentinel := errors.New("sentinel")
				typed := &domainError{sentinel}
				var failure error
				if fail {
					failure = typed
				}
				calls := 0
				ctx := context.WithValue(context.Background(), contextKey{}, "payload-must-not-be-logged")
				value, err := client.Do(ctx, invocation(), func(got context.Context) (privateResult, error) {
					calls++
					if got != ctx {
						t.Fatal("context replaced")
					}
					return expected, failure
				}, mw)
				if value != expected || err != failure || calls != 1 {
					t.Fatal("changed result/error/count")
				}
				if fail {
					var original *domainError
					if !errors.Is(err, sentinel) || !errors.As(err, &original) || original != typed {
						t.Fatal("error chain changed")
					}
				}
				records := logger.snapshot()
				if len(records) != 1 {
					t.Fatal(len(records))
				}
				r := records[0]
				fields := attrs(r)
				wantLevel := slog.LevelDebug
				if level == "info" {
					wantLevel = slog.LevelInfo
				}
				wantFields := 3
				if duration {
					wantFields++
				}
				if r.ctx != ctx || r.level != wantLevel || r.message != "operation completed" || len(fields) != wantFields || fields["component"] != "memory" || fields["operation"] != "read" || fields["success"] != !fail {
					t.Fatal("unexpected observation")
				}
				if duration {
					if d, ok := fields["duration"].(time.Duration); !ok || d < 0 {
						t.Fatal("invalid duration")
					}
				}
			}
		}
	}
}

type contextKey struct{}

func TestContextCancellationAndSuccess(t *testing.T) {
	for _, alreadyCanceled := range []bool{false, true} {
		logger := &sliceLogger{enabled: true}
		ctx, cancel := context.WithCancelCause(context.WithValue(context.Background(), contextKey{}, "retained"))
		defer cancel(nil)
		cause := errors.New("caller cause")
		if alreadyCanceled {
			cancel(cause)
		}
		deadlineCtx, stop := context.WithDeadline(ctx, time.Now().Add(time.Hour))
		defer stop()
		wantDeadline, _ := deadlineCtx.Deadline()
		value, err := client.Do(deadlineCtx, invocation(), func(got context.Context) (int, error) {
			if got != deadlineCtx || got.Value(contextKey{}) != "retained" {
				t.Fatal("context changed")
			}
			if deadline, _ := got.Deadline(); deadline != wantDeadline {
				t.Fatal("deadline changed")
			}
			cancel(cause)
			if context.Cause(got) != cause {
				t.Fatal("cause changed")
			}
			return 9, nil
		}, newMiddleware[int](t, clientlogging.DefaultConfig(), logger))
		if value != 9 || err != nil || len(logger.snapshot()) != 1 || attrs(logger.snapshot()[0])["success"] != true {
			t.Fatal("known success replaced")
		}
	}
}
func TestDisabledAndInvalidCall(t *testing.T) {
	logger := &sliceLogger{}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	calls := 0
	result, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { calls++; return 3, nil }, mw)
	if result != 3 || err != nil || calls != 1 || logger.enabledCalls != 1 || len(logger.snapshot()) != 0 {
		t.Fatal("disabled execution changed")
	}
	//nolint:staticcheck // Nil context intentionally exercises client.Do validation.
	_, err = client.Do[int](nil, invocation(), func(context.Context) (int, error) { calls++; return 0, nil }, mw)
	if !errors.Is(err, client.ErrInvalidCall) || calls != 1 || logger.enabledCalls != 1 {
		t.Fatal("invalid call reached middleware")
	}
}
func TestGenericIdentity(t *testing.T) {
	logger := &sliceLogger{enabled: true}
	ctx := context.Background()
	pointer := &privateResult{7}
	mwPointer := newMiddleware[*privateResult](t, clientlogging.DefaultConfig(), logger)
	got, err := client.Do(ctx, invocation(), func(context.Context) (*privateResult, error) { return pointer, nil }, mwPointer)
	if err != nil || got != pointer {
		t.Fatal("pointer copied")
	}
	list := []int{1, 2}
	gotList, err := client.Do(ctx, invocation(), func(context.Context) ([]int, error) { return list, nil }, newMiddleware[[]int](t, clientlogging.DefaultConfig(), logger))
	if err != nil || &list[0] != &gotList[0] {
		t.Fatal("slice copied")
	}
	source := map[string]int{"a": 1}
	gotMap, err := client.Do(ctx, invocation(), func(context.Context) (map[string]int, error) { return source, nil }, newMiddleware[map[string]int](t, clientlogging.DefaultConfig(), logger))
	if err != nil {
		t.Fatal(err)
	}
	gotMap["a"] = 2
	if source["a"] != 2 {
		t.Fatal("map copied")
	}
}
func TestNextReceivesOriginalArguments(t *testing.T) {
	logger := &sliceLogger{enabled: true}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	ctx := context.Background()
	info := invocation()
	opCalls := 0
	operation := client.Operation[int](func(context.Context) (int, error) { opCalls++; return 8, nil })
	next := func(got context.Context, gotInfo client.Invocation, op client.Operation[int]) (int, error) {
		if got != ctx || gotInfo != info || reflect.ValueOf(op).Pointer() != reflect.ValueOf(operation).Pointer() {
			t.Fatal("arguments replaced")
		}
		gotInfo.Component = "locally changed"
		return op(got)
	}
	chain, err := client.Chain(client.Handler[int](next), mw)
	if err != nil {
		t.Fatal(err)
	}
	value, err := chain(ctx, info, operation)
	if err != nil || value != 8 || opCalls != 1 || info.Component != "memory" || attrs(logger.snapshot()[0])["component"] != "memory" {
		t.Fatal("chain mutated invocation")
	}
}
func TestDurationCoversNextNotEmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var eventAttrs []slog.Attr
		logger := loggerFuncs{enabled: func(context.Context, slog.Level) bool { return true }, emit: func(_ context.Context, _ slog.Level, _ string, a ...slog.Attr) {
			synctest.Sleep(7 * time.Second)
			eventAttrs = append([]slog.Attr(nil), a...)
		}}
		mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
		start := time.Now()
		_, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { synctest.Sleep(3 * time.Second); return 1, nil }, mw)
		if err != nil || time.Since(start) != 10*time.Second {
			t.Fatal("virtual timeline changed")
		}
		if attrs(event{attrs: eventAttrs})["duration"] != 3*time.Second {
			t.Fatal("emission included in duration")
		}
	})
}

type loggerFuncs struct {
	enabled func(context.Context, slog.Level) bool
	emit    func(context.Context, slog.Level, string, ...slog.Attr)
}

func (l loggerFuncs) Enabled(ctx context.Context, level slog.Level) bool {
	return l.enabled(ctx, level)
}
func (l loggerFuncs) LogAttrs(ctx context.Context, level slog.Level, msg string, a ...slog.Attr) {
	l.emit(ctx, level, msg, a...)
}
func TestCompositionOrderAndShortCircuit(t *testing.T) {
	var order []string
	logger := loggerFuncs{enabled: func(context.Context, slog.Level) bool { order = append(order, "enabled"); return true }, emit: func(context.Context, slog.Level, string, ...slog.Attr) { order = append(order, "emit") }}
	tracing := func(next client.Handler[int]) client.Handler[int] {
		return func(ctx context.Context, info client.Invocation, op client.Operation[int]) (int, error) {
			order = append(order, "before")
			v, err := next(ctx, info, op)
			order = append(order, "after")
			return v, err
		}
	}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	op := func(context.Context) (int, error) { order = append(order, "io"); return 1, nil }
	_, err := client.Do(context.Background(), invocation(), op, mw, tracing)
	if err != nil || !reflect.DeepEqual(order, []string{"enabled", "before", "io", "after", "emit"}) {
		t.Fatal(order)
	}
	order = nil
	_, err = client.Do(context.Background(), invocation(), op, tracing, mw)
	if err != nil || !reflect.DeepEqual(order, []string{"before", "enabled", "io", "emit", "after"}) {
		t.Fatal(order)
	}
	order = nil
	cut := func(client.Handler[int]) client.Handler[int] {
		return func(context.Context, client.Invocation, client.Operation[int]) (int, error) { return 5, nil }
	}
	value, err := client.Do(context.Background(), invocation(), op, mw, cut)
	if err != nil || value != 5 || !reflect.DeepEqual(order, []string{"enabled", "emit"}) {
		t.Fatal(order)
	}
}
func expectPanic(t *testing.T, token any, fn func()) {
	t.Helper()
	defer func() {
		if got := recover(); got != token {
			t.Fatal("panic value changed")
		}
	}()
	fn()
	t.Fatal("panic missing")
}
func TestPanicsAndGoexit(t *testing.T) {
	token := &struct{}{}
	logger := &sliceLogger{enabled: true}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	expectPanic(t, token, func() {
		_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { panic(token) }, mw)
	})
	if len(logger.snapshot()) != 0 {
		t.Fatal("panic logged as completion")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { runtime.Goexit(); return 0, nil }, mw)
	}()
	<-done
	if len(logger.snapshot()) != 0 {
		t.Fatal("Goexit logged as completion")
	}
	for _, atEntry := range []bool{false, true} {
		calls := 0
		backend := loggerFuncs{enabled: func(context.Context, slog.Level) bool {
			if atEntry {
				panic(token)
			}
			return true
		}, emit: func(context.Context, slog.Level, string, ...slog.Attr) { panic(token) }}
		observe := newMiddleware[int](t, clientlogging.DefaultConfig(), backend)
		expectPanic(t, token, func() {
			_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { calls++; return 1, nil }, observe)
		})
		want := 1
		if atEntry {
			want = 0
		}
		if calls != want {
			t.Fatal(calls)
		}
	}
}
func TestLoggerGoexit(t *testing.T) {
	for _, atEntry := range []bool{false, true} {
		backend := loggerFuncs{enabled: func(context.Context, slog.Level) bool {
			if atEntry {
				runtime.Goexit()
			}
			return true
		}, emit: func(context.Context, slog.Level, string, ...slog.Attr) { runtime.Goexit() }}
		mw := newMiddleware[int](t, clientlogging.DefaultConfig(), backend)
		done := make(chan struct{})
		var calls atomic.Int64
		go func() {
			defer close(done)
			_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { calls.Add(1); return 1, nil }, mw)
		}()
		<-done
		want := int64(1)
		if atEntry {
			want = 0
		}
		if calls.Load() != want {
			t.Fatal(calls.Load())
		}
	}
}
func TestStandardSlogAndWriteFailure(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var _ clientlogging.Logger = logger
	mw := newMiddleware[privateResult](t, clientlogging.DefaultConfig(), logger)
	failure := &domainError{errors.New("cause")}
	value, err := client.Do(context.Background(), invocation(), func(context.Context) (privateResult, error) { return privateResult{8}, failure }, mw)
	if value.value != 8 || err != failure || bytes.Count(out.Bytes(), []byte("\n")) != 1 {
		t.Fatal("changed result or count")
	}
	var r map[string]any
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r["msg"] != "operation completed" || r["success"] != false || r["level"] != "DEBUG" || len(r) != 7 {
		t.Fatal(r)
	}
	for _, key := range []string{"error", "result", "payload", "tenant", "stack"} {
		if _, ok := r[key]; ok {
			t.Fatal("unexpected field")
		}
	}
	writerFailure := errors.New("writer failure")
	logger = slog.New(slog.NewJSONHandler(writeFunc(func([]byte) (int, error) { return 0, writerFailure }), &slog.HandlerOptions{Level: slog.LevelDebug}))
	_, err = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 2, failure }, newMiddleware[int](t, clientlogging.DefaultConfig(), logger))
	if err != failure {
		t.Fatal("writer failure replaced I/O error")
	}
}

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }
func TestSlowHandlerAndConcurrentReuse(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	logger := loggerFuncs{enabled: func(context.Context, slog.Level) bool { return true }, emit: func(context.Context, slog.Level, string, ...slog.Attr) { close(entered); <-release }}
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	go func() {
		defer close(done)
		_, _ = client.Do(context.Background(), invocation(), func(context.Context) (int, error) { calls.Add(1); return 1, nil }, mw)
	}()
	<-entered
	if calls.Load() != 1 {
		close(release)
		<-done
		t.Fatal("handler ran before operation")
	}
	close(release)
	<-done
	recorder := &sliceLogger{enabled: true}
	observe := newMiddleware[int](t, clientlogging.DefaultConfig(), recorder)
	chain, err := client.Chain(client.Handler[int](terminal[int]), observe)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			for range 20 {
				value, err := chain(context.Background(), invocation(), func(context.Context) (int, error) { return i, nil })
				if err != nil || value != i {
					t.Error("changed parallel result")
				}
			}
		})
	}
	wg.Wait()
	if len(recorder.snapshot()) != 320 {
		t.Fatal("missing or duplicate completion")
	}
}
func TestSlogCanFilterAgain(t *testing.T) {
	var out bytes.Buffer
	var level slog.LevelVar
	level.Set(slog.LevelDebug)
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: &level}))
	mw := newMiddleware[int](t, clientlogging.DefaultConfig(), logger)
	_, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { level.Set(slog.LevelError); return 1, nil }, mw)
	if err != nil || out.Len() != 0 {
		t.Fatal("dynamic handler filtering broken")
	}
}

type nilAwareLogger struct{}

func (*nilAwareLogger) Enabled(context.Context, slog.Level) bool { return false }
func (*nilAwareLogger) LogAttrs(context.Context, slog.Level, string, ...slog.Attr) {
	panic("not expected")
}
func TestForeignTypedNilResponsibility(t *testing.T) {
	var foreign *nilAwareLogger
	mw, err := clientlogging.New[int](clientlogging.DefaultConfig(), clientlogging.Dependencies{Logger: foreign})
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.Do(context.Background(), invocation(), func(context.Context) (int, error) { return 4, nil }, mw)
	if value != 4 || err != nil {
		t.Fatal("unexpected foreign nil policy")
	}
}
func TestNoFormattingOfResultsOrErrors(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	failure := &domainError{}
	_, err := client.Do(context.Background(), invocation(), func(context.Context) (privateResult, error) { return privateResult{9}, failure }, newMiddleware[privateResult](t, clientlogging.DefaultConfig(), logger))
	if err != failure || strings.Contains(out.String(), "error=") || strings.Contains(out.String(), "result=") {
		t.Fatal("opaque values inspected")
	}
}
func TestIntegrationTransport(t *testing.T) {
	logger := &sliceLogger{enabled: true}
	var calls int
	transport := func(ctx context.Context) (int, error) { calls++; return 8, ctx.Err() }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	value, err := client.Do(ctx, invocation(), transport, newMiddleware[int](t, clientlogging.DefaultConfig(), logger))
	if value != 8 || !errors.Is(err, context.Canceled) || calls != 1 || len(logger.snapshot()) != 1 {
		t.Fatal("transport/cancellation contract")
	}
}
func BenchmarkStandardLoggerBaseline(b *testing.B) {
	l := slog.New(slog.NewJSONHandler(io.Discard, nil))
	b.ReportAllocs()
	for b.Loop() {
		l.LogAttrs(context.Background(), slog.LevelInfo, "operation completed", slog.String("component", "memory"), slog.String("operation", "read"), slog.Bool("success", true))
	}
}

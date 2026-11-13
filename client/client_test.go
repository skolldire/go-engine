package client_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	c "github.com/skolldire/go-engine/client"
)

func terminal[T any](ctx context.Context, _ c.Invocation, operation c.Operation[T]) (T, error) {
	return operation(ctx)
}
func info() c.Invocation                             { return c.Invocation{Component: "repository", Operation: "Fetch"} }
func identity[T any](next c.Handler[T]) c.Handler[T] { return next }
func requireInvalid(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, c.ErrInvalidCall) || !strings.Contains(err.Error(), field) {
		t.Fatalf("expected ErrInvalidCall for %s: %v", field, err)
	}
}

func TestDoValidationOrderAndNoExecution(t *testing.T) {
	var builds, calls int
	middleware := func(next c.Handler[int]) c.Handler[int] { builds++; return next }
	operation := func(context.Context) (int, error) { calls++; return 42, nil }
	var nilContext context.Context
	cases := []struct {
		name      string
		ctx       context.Context
		info      c.Invocation
		operation c.Operation[int]
		field     string
	}{
		{"context_first", nilContext, c.Invocation{}, nil, "ctx"},
		{"operation_second", context.Background(), c.Invocation{}, nil, "operation"},
		{"component_third", context.Background(), c.Invocation{}, operation, "Component"},
		{"operation_label_fourth", context.Background(), c.Invocation{Component: "component"}, operation, "Operation"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := c.Do(test.ctx, test.info, test.operation, middleware, nil)
			requireInvalid(t, err, test.field)
			if result != 0 || calls != 0 || builds != 0 {
				t.Fatal(result, calls, builds)
			}
		})
	}
	result, err := c.Do(context.Background(), info(), operation, middleware, nil)
	requireInvalid(t, err, "middleware[1]")
	if result != 0 || calls != 0 || builds != 0 {
		t.Fatal(result, calls, builds)
	}
}
func TestLabelByteBoundariesAndPrivacy(t *testing.T) {
	for _, label := range []string{"", strings.Repeat("s", 128), strings.Repeat("s", 129), strings.Repeat("é", 64), strings.Repeat("é", 65), " ", "\xff\x00"} {
		for _, component := range []bool{true, false} {
			invocation := info()
			field := "Operation"
			if component {
				invocation.Component = label
				field = "Component"
			} else {
				invocation.Operation = label
			}
			calls := 0
			got, err := c.Do(context.Background(), invocation, func(ctx context.Context) (int, error) { calls++; return 17, nil })
			valid := len(label) > 0 && len(label) <= 128
			if valid {
				if err != nil || got != 17 || calls != 1 {
					t.Fatal(len(label), got, err, calls)
				}
			} else {
				requireInvalid(t, err, field)
				if !strings.Contains(err.Error(), fmt.Sprint(len(label))) || !strings.Contains(err.Error(), "bytes") || got != 0 || calls != 0 {
					t.Fatal(got, err, calls)
				}
				if label != "" && strings.Contains(err.Error(), label) {
					t.Fatal("label value exposed")
				}
			}
		}
	}
}
func TestChainBuildAndExecutionOrder(t *testing.T) {
	var events []string
	wrap := func(name string) c.Middleware[int] {
		return func(next c.Handler[int]) c.Handler[int] {
			events = append(events, "build."+name)
			return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
				events = append(events, name+".before")
				value, err := next(ctx, inv, op)
				events = append(events, name+".after")
				return value, err
			}
		}
	}
	base := func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
		events = append(events, "base")
		return op(ctx)
	}
	first, second := wrap("A"), wrap("B")
	middleware := []c.Middleware[int]{first, second}
	original := []uintptr{reflect.ValueOf(first).Pointer(), reflect.ValueOf(second).Pointer()}
	chain, err := c.Chain(base, middleware...)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"build.B", "build.A"}) {
		t.Fatal(events)
	}
	for i := range middleware {
		if reflect.ValueOf(middleware[i]).Pointer() != original[i] {
			t.Fatal("slice changed")
		}
	}
	middleware[0] = nil
	middleware[1] = nil
	for range 2 {
		got, err := chain(context.Background(), info(), func(context.Context) (int, error) { events = append(events, "operation"); return 42, nil })
		if got != 42 || err != nil {
			t.Fatal(got, err)
		}
	}
	want := []string{"build.B", "build.A"}
	for range 2 {
		want = append(want, "A.before", "B.before", "base", "operation", "B.after", "A.after")
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatal(events)
	}
}
func TestChainInvalidAndEmpty(t *testing.T) {
	var base c.Handler[int] = terminal[int]
	if got, err := c.Chain[int](nil); got != nil {
		t.Fatal("invalid chain returned handler")
	} else {
		requireInvalid(t, err, "next")
	}
	if got, err := c.Chain(base); err != nil || reflect.ValueOf(got).Pointer() != reflect.ValueOf(base).Pointer() {
		t.Fatal("empty chain must return next", err)
	}
	for _, position := range []int{0, 1, 2} {
		for _, returnNil := range []bool{false, true} {
			middleware := []c.Middleware[int]{identity[int], identity[int], identity[int]}
			if returnNil {
				middleware[position] = func(c.Handler[int]) c.Handler[int] { return nil }
			} else {
				middleware[position] = nil
			}
			chain, err := c.Chain(base, middleware...)
			requireInvalid(t, err, fmt.Sprintf("middleware[%d]", position))
			if chain != nil {
				t.Fatal("invalid handler")
			}
		}
	}
}
func TestChainDoesNotRevalidateInvocation(t *testing.T) {
	called := false
	base := func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
		called = true
		if ctx != nil || inv != (c.Invocation{}) || op != nil {
			t.Error("unexpected input change")
		}
		return 7, nil
	}
	chain, err := c.Chain(base, identity[int])
	if err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	if got, err := chain(nilContext, c.Invocation{}, nil); err != nil || got != 7 || !called {
		t.Fatal(got, err)
	}
}

type operationError struct{ code int }

func (e *operationError) Error() string { return "domain operation failed" }

type contextKey struct{}

func TestContextResultAndErrorIdentity(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.WithValue(context.Background(), contextKey{}, "value"), deadline)
	defer cancel()
	domain := &operationError{code: 42}
	result, err := c.Do(ctx, info(), func(received context.Context) (int, error) {
		if received != ctx || received.Value(contextKey{}) != "value" {
			t.Error("context changed")
		}
		if actual, _ := received.Deadline(); actual != deadline {
			t.Error("deadline changed")
		}
		return 99, domain
	})
	var typed *operationError
	if result != 99 || err != domain || !errors.Is(err, domain) || !errors.As(err, &typed) || typed != domain {
		t.Fatal(result, err)
	}
	cause := errors.New("cancellation cause")
	canceled, cancelCause := context.WithCancelCause(ctx)
	cancelCause(cause)
	var calls int
	result, err = c.Do(canceled, info(), func(received context.Context) (int, error) {
		calls++
		if received != canceled || received.Err() != context.Canceled || context.Cause(received) != cause {
			t.Error("cancellation lost")
		}
		return 11, context.Cause(received)
	})
	if calls != 1 || result != 11 || err != cause {
		t.Fatal(calls, result, err)
	}
}
func TestGenericResults(t *testing.T) {
	ctx := context.Background()
	domain := errors.New("domain")
	type value struct{ Count int }
	expected := &value{Count: 9}
	pointer, err := c.Do(ctx, info(), func(context.Context) (*value, error) { return expected, domain })
	if pointer != expected || err != domain {
		t.Fatal(pointer, err)
	}
	structured, err := c.Do(ctx, info(), func(context.Context) (value, error) { return *expected, domain })
	if structured != *expected || err != domain {
		t.Fatal(structured, err)
	}
	data := []int{1, 2}
	slice, err := c.Do(ctx, info(), func(context.Context) ([]int, error) { return data, nil })
	if err != nil || &slice[0] != &data[0] {
		t.Fatal(slice, err)
	}
	mapping := map[string]int{"count": 1}
	returned, err := c.Do(ctx, info(), func(context.Context) (map[string]int, error) { return mapping, nil })
	if err != nil {
		t.Fatal(err)
	}
	returned["count"] = 2
	if mapping["count"] != 2 {
		t.Fatal("map copied")
	}
}
func TestShortCircuitAndCallerControlledTransformation(t *testing.T) {
	calls := 0
	short := func(c.Handler[int]) c.Handler[int] {
		return func(context.Context, c.Invocation, c.Operation[int]) (int, error) { return 7, nil }
	}
	got, err := c.Do(context.Background(), info(), func(context.Context) (int, error) { calls++; return 1, nil }, short)
	if got != 7 || err != nil || calls != 0 {
		t.Fatal(got, err, calls)
	}
	inv := info()
	alter := func(next c.Handler[int]) c.Handler[int] {
		return func(ctx context.Context, current c.Invocation, op c.Operation[int]) (int, error) {
			current.Component = "modified"
			value, err := next(ctx, current, op)
			return value + 1, err
		}
	}
	base := func(ctx context.Context, current c.Invocation, op c.Operation[int]) (int, error) {
		if current.Component != "modified" {
			t.Error(current)
		}
		return op(ctx)
	}
	chain, err := c.Chain(base, alter)
	if err != nil {
		t.Fatal(err)
	}
	got, err = chain(context.Background(), inv, func(context.Context) (int, error) { calls++; return 5, nil })
	if got != 6 || err != nil || inv != info() {
		t.Fatal(got, err, inv)
	}
	twice := func(next c.Handler[int]) c.Handler[int] {
		return func(ctx context.Context, current c.Invocation, op c.Operation[int]) (int, error) {
			first, err := next(ctx, current, op)
			if err != nil {
				return first, err
			}
			second, err := next(ctx, current, op)
			return first + second, err
		}
	}
	calls = 0
	got, err = c.Do(context.Background(), info(), func(context.Context) (int, error) { calls++; return 3, nil }, twice)
	if calls != 2 || got != 6 || err != nil {
		t.Fatal(calls, got, err)
	}
}
func TestPanicsPropagate(t *testing.T) {
	token := &struct{ message string }{"panic token"}
	cases := map[string]func(){
		"operation": func() { _, _ = c.Do(context.Background(), info(), func(context.Context) (int, error) { panic(token) }) },
		"terminal": func() {
			chain, _ := c.Chain(func(context.Context, c.Invocation, c.Operation[int]) (int, error) { panic(token) })
			_, _ = chain(context.Background(), info(), nil)
		},
		"constructor": func() {
			_, _ = c.Chain(c.Handler[int](terminal[int]), func(c.Handler[int]) c.Handler[int] { panic(token) })
		},
		"middleware": func() {
			_, _ = c.Do(context.Background(), info(), func(context.Context) (int, error) { return 1, nil }, func(c.Handler[int]) c.Handler[int] {
				return func(context.Context, c.Invocation, c.Operation[int]) (int, error) { panic(token) }
			})
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if got := recover(); got != token {
					t.Errorf("panic changed: %v", got)
				}
			}()
			call()
			t.Error("panic swallowed")
		})
	}
}
func TestCompositionDeadlineAndOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancel := context.WithTimeout(context.WithValue(context.Background(), contextKey{}, "parent"), time.Minute)
		defer cancel()
		var events []string
		record := func(next c.Handler[int]) c.Handler[int] {
			return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
				events = append(events, "outer.before")
				value, err := next(ctx, inv, op)
				events = append(events, "outer.after")
				return value, err
			}
		}
		bounded := func(next c.Handler[int]) c.Handler[int] {
			return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
				events = append(events, "inner.before")
				derived, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				value, err := next(derived, inv, op)
				events = append(events, "inner.after")
				return value, err
			}
		}
		result, err := c.Do(parent, info(), func(ctx context.Context) (int, error) {
			events = append(events, "operation")
			if ctx.Value(contextKey{}) != "parent" {
				t.Error("value lost")
			}
			parentDeadline, _ := parent.Deadline()
			deadline, _ := ctx.Deadline()
			if !deadline.Before(parentDeadline) {
				t.Error("deadline not bounded")
			}
			<-ctx.Done()
			return 12, context.Cause(ctx)
		}, record, bounded)
		if result != 12 || err != context.DeadlineExceeded || parent.Err() != nil {
			t.Fatal(result, err, parent.Err())
		}
		if !reflect.DeepEqual(events, []string{"outer.before", "inner.before", "operation", "inner.after", "outer.after"}) {
			t.Fatal(events)
		}
	})
}
func TestDoConstructsEveryCallAndConcurrentReuse(t *testing.T) {
	var builds, runs atomic.Int64
	middleware := func(next c.Handler[int]) c.Handler[int] {
		builds.Add(1)
		return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
			runs.Add(1)
			return next(ctx, inv, op)
		}
	}
	for range 3 {
		if value, err := c.Do(context.Background(), info(), func(context.Context) (int, error) { return 1, nil }, middleware); err != nil || value != 1 {
			t.Fatal(value, err)
		}
	}
	if builds.Load() != 3 || runs.Load() != 3 {
		t.Fatal(builds.Load(), runs.Load())
	}
	chain, err := c.Chain(c.Handler[int](terminal[int]), middleware)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 100 {
				got, err := chain(context.Background(), info(), func(context.Context) (int, error) { return 42, nil })
				if got != 42 || err != nil {
					t.Error(got, err)
					return
				}
			}
		})
	}
	wg.Wait()
	if builds.Load() != 4 || runs.Load() != 3203 {
		t.Fatal(builds.Load(), runs.Load())
	}
}
func ExampleDo() {
	value, err := c.Do(context.Background(), c.Invocation{Component: "store", Operation: "Read"}, func(context.Context) (string, error) { return "record", nil })
	fmt.Println(value, err)
	// Output: record <nil>
}

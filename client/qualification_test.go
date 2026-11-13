package client_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"weak"

	c "github.com/skolldire/go-engine/client"
)

func TestStressConcurrentChains(t *testing.T) {
	if os.Getenv("CLIENT_STRESS") != "1" {
		t.Skip("set CLIENT_STRESS=1")
	}
	var calls, builds, runs atomic.Int64
	middleware := func(next c.Handler[int]) c.Handler[int] {
		builds.Add(1)
		return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
			runs.Add(1)
			return next(ctx, inv, op)
		}
	}
	chain, err := c.Chain(c.Handler[int](terminal[int]), middleware, middleware, middleware, middleware)
	if err != nil {
		t.Fatal(err)
	}
	domain := errors.New("domain")
	cause := errors.New("canceled")
	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Go(func() {
			for i := range 2000 {
				ctx, cancel := context.WithCancelCause(context.Background())
				value := worker*2000 + i
				expected := error(nil)
				switch i % 3 {
				case 1:
					expected = domain
				case 2:
					expected = cause
					cancel(cause)
				}
				operation := func(received context.Context) (int, error) {
					calls.Add(1)
					if received != ctx {
						t.Error("context changed")
					}
					return value, expected
				}
				var got int
				var err error
				if i%2 == 0 {
					got, err = chain(ctx, info(), operation)
				} else {
					got, err = c.Do(ctx, info(), operation, middleware, middleware, middleware, middleware)
				}
				cancel(nil)
				if got != value || err != expected {
					t.Error(got, err)
					return
				}
			}
		})
	}
	wg.Wait()
	if calls.Load() != 64000 || runs.Load() != 256000 || builds.Load() != 128004 {
		t.Fatal(calls.Load(), runs.Load(), builds.Load())
	}
	assertNoLeaks(t)
}

type retainedPayload [64 << 10]byte

func abandonedChains(t *testing.T) []weak.Pointer[retainedPayload] {
	t.Helper()
	var pointers []weak.Pointer[retainedPayload]
	for i := range 16 {
		payload := new(retainedPayload)
		payload[0] = byte(i)
		pointers = append(pointers, weak.Make(payload))
		base := func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
			value, err := op(ctx)
			return value + int(payload[0]), err
		}
		wrap := func(next c.Handler[int]) c.Handler[int] {
			return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
				return next(ctx, inv, op)
			}
		}
		chain, err := c.Chain(base, wrap, wrap, wrap, wrap)
		if err != nil {
			t.Fatal(err)
		}
		got, err := chain(context.Background(), info(), func(context.Context) (int, error) { return 1, nil })
		if got != i+1 || err != nil {
			t.Fatal(got, err)
		}
	}
	return pointers
}
func TestStressAbandonedChains(t *testing.T) {
	if os.Getenv("CLIENT_STRESS") != "1" {
		t.Skip("set CLIENT_STRESS=1")
	}
	var baseline, peak uint64
	for cycle := range 60 {
		pointers := abandonedChains(t)
		runtime.GC()
		for _, pointer := range pointers {
			if pointer.Value() != nil {
				t.Fatal("abandoned chain retained payload")
			}
		}
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		if cycle == 9 {
			baseline = memory.HeapAlloc
			peak = baseline
		}
		if cycle >= 10 {
			if memory.HeapAlloc > peak {
				peak = memory.HeapAlloc
			}
			if memory.HeapAlloc > baseline+(4<<20) {
				t.Fatalf("retained heap: baseline=%d current=%d", baseline, memory.HeapAlloc)
			}
		}
	}
	assertNoLeaks(t)
	t.Logf("960 abandoned chains/payloads of 64KiB, heap baseline=%d peak=%d", baseline, peak)
}
func TestNoGoroutineLeaks(t *testing.T) {
	for range 100 {
		chain, err := c.Chain(c.Handler[int](terminal[int]), identity[int])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := chain(context.Background(), info(), func(context.Context) (int, error) { return 1, nil }); err != nil {
			t.Fatal(err)
		}
	}
	assertNoLeaks(t)
}
func assertNoLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("goroutineleak profile missing")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if profile.Count() != 0 {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}

func FuzzLabels(f *testing.F) {
	for _, label := range []string{"", "read", strings.Repeat("x", 128), strings.Repeat("é", 65), "\xff\x00"} {
		f.Add(label, "Fetch", uint8(0))
	}
	f.Fuzz(func(t *testing.T, component, operation string, invalid uint8) {
		if len(component)+len(operation) > 4096 {
			return
		}
		ctx := context.Background()
		if invalid%5 == 1 {
			ctx = nil
		}
		var calls int
		var op c.Operation[int] = func(context.Context) (int, error) { calls++; return 7, nil }
		if invalid%5 == 2 {
			op = nil
		}
		got, err := c.Do(ctx, c.Invocation{Component: component, Operation: operation}, op)
		valid := ctx != nil && op != nil && len(component) > 0 && len(component) <= 128 && len(operation) > 0 && len(operation) <= 128
		if valid {
			if err != nil || got != 7 || calls != 1 {
				t.Fatal(got, err, calls)
			}
		} else {
			if !errors.Is(err, c.ErrInvalidCall) || got != 0 || calls != 0 {
				t.Fatal(got, err, calls)
			}
		}
	})
}
func FuzzChain(f *testing.F) {
	f.Add(uint8(0), uint8(255), false)
	f.Add(uint8(8), uint8(4), false)
	f.Add(uint8(4), uint8(2), true)
	f.Fuzz(func(t *testing.T, size, bad uint8, returnedNil bool) {
		count := int(size % 33)
		badIndex := int(bad)
		if badIndex >= count {
			badIndex = -1
		}
		var built, executed []int
		middleware := make([]c.Middleware[int], count)
		for i := range count {
			middleware[i] = func(next c.Handler[int]) c.Handler[int] {
				built = append(built, i)
				return func(ctx context.Context, inv c.Invocation, op c.Operation[int]) (int, error) {
					executed = append(executed, i)
					return next(ctx, inv, op)
				}
			}
		}
		if badIndex >= 0 {
			if returnedNil {
				middleware[badIndex] = func(c.Handler[int]) c.Handler[int] { built = append(built, badIndex); return nil }
			} else {
				middleware[badIndex] = nil
			}
		}
		before := make([]bool, count)
		for i, mw := range middleware {
			before[i] = mw == nil
		}
		calls := 0
		chain, err := c.Chain(c.Handler[int](terminal[int]), middleware...)
		for i, mw := range middleware {
			if (mw == nil) != before[i] {
				t.Fatal("slice mutated")
			}
		}
		if badIndex >= 0 {
			requireInvalid(t, err, fmt.Sprintf("middleware[%d]", badIndex))
			if chain != nil {
				t.Fatal("handler returned on invalid chain")
			}
			return
		}
		if err != nil || len(executed) != 0 || len(built) != count {
			t.Fatal(err, built, executed)
		}
		for i, index := range built {
			if index != count-1-i {
				t.Fatal("construction order")
			}
		}
		for range 2 {
			value, err := chain(context.Background(), info(), func(context.Context) (int, error) { calls++; return 3, nil })
			if err != nil || value != 3 {
				t.Fatal(value, err)
			}
		}
		want := make([]int, 0, count*2)
		for range 2 {
			for i := range count {
				want = append(want, i)
			}
		}
		if !reflect.DeepEqual(executed, want) && count > 0 {
			t.Fatal(executed, want)
		}
		if calls != 2 || len(built) != count {
			t.Fatal(calls, built)
		}
	})
}

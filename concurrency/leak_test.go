package concurrency

import (
	"bytes"
	"context"
	"runtime"
	"runtime/pprof"
	"testing"
)

func TestGoroutineLeakProfileAfterLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p, err := NewPool(DefaultPoolConfig(), func(context.Context, int) (int, error) { return 1, nil })
	if err != nil {
		t.Fatal(err)
	}
	f := submitInt(t, p, ctx, "leak-check", "", 1)
	outcome(t, f)
	shutdownPool(t, p)
	in := source(t, ctx, 1, 2, 3)
	outputs, err := FanOut(ctx, in, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	out, err := FanIn(ctx, 0, outputs...)
	if err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	cancel()
	runtime.GC()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("Go 1.27 goroutineleak profile missing")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if profile.Count() != 0 {
		t.Fatalf("leaked goroutines: %s", report.String())
	}
	t.Log(report.String())
}

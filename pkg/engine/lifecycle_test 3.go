package engine

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLifecycle_ClosesInLIFOOrder(t *testing.T) {
	var order []string
	l := &lifecycle{}
	for _, name := range []string{"first", "second", "third"} {
		n := name
		l.register(n, func(context.Context) error {
			order = append(order, n)
			return nil
		})
	}

	err := l.close(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

func TestLifecycle_AggregatesErrors(t *testing.T) {
	errA := errors.New("a failed")
	errB := errors.New("b failed")

	l := &lifecycle{}
	l.register("a", func(context.Context) error { return errA })
	l.register("ok", func(context.Context) error { return nil })
	l.register("b", func(context.Context) error { return errB })

	err := l.close(context.Background())
	assert.Error(t, err)
	// Both errors must be reported: one failing closer must not hide another.
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)
}

func TestLifecycle_CloseIsIdempotent(t *testing.T) {
	var calls int
	l := &lifecycle{}
	l.register("x", func(context.Context) error {
		calls++
		return nil
	})

	assert.NoError(t, l.close(context.Background()))
	assert.NoError(t, l.close(context.Background()))
	assert.Equal(t, 1, calls, "closer must run exactly once across repeated close calls")
}

func TestLifecycle_RegisterAfterCloseIsRejected(t *testing.T) {
	l := &lifecycle{}
	_ = l.close(context.Background())

	ok := l.register("late", func(context.Context) error { return nil })
	assert.False(t, ok, "registration after close must be rejected so the resource is not silently leaked")
}

func TestLifecycle_NilCloserIgnored(t *testing.T) {
	l := &lifecycle{}
	assert.False(t, l.register("nil", nil))
	assert.NoError(t, l.close(context.Background()))
}

func TestLifecycle_NilContextIsTolerated(t *testing.T) {
	var got context.Context
	l := &lifecycle{}
	l.register("x", func(ctx context.Context) error {
		got = ctx
		return nil
	})

	//nolint:staticcheck // SA1012: passing nil is the case under test
	assert.NoError(t, l.close(nil))
	assert.NotNil(t, got, "a nil context must be substituted, not handed to the closer")
}

func TestLifecycle_ConcurrentRegister(t *testing.T) {
	l := &lifecycle{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.register("c", func(context.Context) error { return nil })
		}()
	}
	wg.Wait()

	l.mu.Lock()
	n := len(l.entries)
	l.mu.Unlock()
	assert.Equal(t, 50, n)
	assert.NoError(t, l.close(context.Background()))
}

func TestEngine_Close_NoLifecycleIsNoop(t *testing.T) {
	e := &Engine{}
	assert.NoError(t, e.Close(context.Background()))
}

func TestEngine_RegisterCloser_ClosesViaEngine(t *testing.T) {
	e := &Engine{lifecycle: &lifecycle{}}
	closed := false
	e.RegisterCloser("res", func(context.Context) error {
		closed = true
		return nil
	})

	assert.NoError(t, e.Close(context.Background()))
	assert.True(t, closed)
}

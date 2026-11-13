package ratelimit_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/ratelimit"
)

func TestMemoryTransport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, err := ratelimit.New(ratelimit.Config{RatePerSecond: 2, Burst: 3})
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		failure := errors.New("transport failure")
		send := func(ctx context.Context, n int) error {
			if err := l.Wait(ctx, n); err != nil {
				return err
			}
			calls++
			return failure
		}
		if err := send(context.Background(), 3); err != failure {
			t.Fatal(err)
		}
		if l.Allow(1) {
			t.Fatal("operation failure refunded tokens")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second/4)
		defer cancel()
		if err := send(ctx, 1); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatal(err, calls)
		}
		start := time.Now()
		if err := send(context.Background(), 1); err != failure || time.Since(start) != time.Second/4 || calls != 2 {
			t.Fatal(err, calls, time.Since(start))
		}
		time.Sleep(time.Second)
		if !l.Allow(2) {
			t.Fatal("shared Allow budget")
		}
		started := time.Now()
		if err := send(context.Background(), 2); err != failure || time.Since(started) != time.Second {
			t.Fatal(err, time.Since(started))
		}
		other, _ := ratelimit.New(ratelimit.Config{RatePerSecond: 2, Burst: 3})
		if !other.Allow(3) {
			t.Fatal("instances share budget")
		}
	})
}

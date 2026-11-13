package main

import (
	"context"
	"github.com/skolldire/go-engine/ratelimit"
	"testing"
	"testing/synctest"
	"time"
)

func TestConsumer(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
func TestFIFO(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, err := ratelimit.New(ratelimit.Config{RatePerSecond: 1, Burst: 2})
		if err != nil {
			t.Fatal(err)
		}
		l.Allow(2)
		large := make(chan error, 1)
		small := make(chan error, 1)
		go func() { large <- l.Wait(context.Background(), 2) }()
		synctest.Wait()
		go func() { small <- l.Wait(context.Background(), 1) }()
		synctest.Wait()
		time.Sleep(time.Second)
		if l.Allow(1) {
			t.Fatal("Allow bypassed FIFO")
		}
		time.Sleep(time.Second)
		if err := <-large; err != nil {
			t.Fatal(err)
		}
		select {
		case <-small:
			t.Fatal("small overtook large")
		default:
		}
		time.Sleep(time.Second)
		if err := <-small; err != nil {
			t.Fatal(err)
		}
	})
}

package circuitbreaker

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"testing"
	"testing/synctest"
	"time"
)

func invokeRun(b *Breaker, ctx context.Context, fn func(context.Context) error) error {
	return b.Run(ctx, fn)
}
func invokeExecute(b *Breaker, ctx context.Context, fn func(context.Context) error) error {
	_, err := Execute(ctx, b, func(ctx context.Context) (int, error) { return 7, fn(ctx) })
	return err
}
func TestRunExecuteEquivalence(t *testing.T) {
	for _, abnormal := range []string{"normal", "panic-operation", "panic-classifier", "exit-operation", "exit-classifier"} {
		t.Run(abnormal, func(t *testing.T) {
			var snapshots []Counts
			for _, invoke := range []func(*Breaker, context.Context, func(context.Context) error) error{invokeRun, invokeExecute} {
				ignoredError, remoteError := errors.New("ignored"), errors.New("remote")
				cfg := testConfig()
				cfg.RequestThreshold = 100
				b := newTestBreaker(t, cfg, func(err error) bool {
					if abnormal == "panic-classifier" && err == remoteError {
						panic("same panic")
					}
					if abnormal == "exit-classifier" && err == remoteError {
						runtime.Goexit()
					}
					return err == remoteError
				})
				if err := invoke(b, context.Background(), func(context.Context) error { return nil }); err != nil {
					t.Fatal(err)
				}
				if err := invoke(b, context.Background(), func(context.Context) error { return ignoredError }); err != ignoredError {
					t.Fatal("ignored identity")
				}
				done := make(chan struct{})
				var panicValue any
				go func() {
					defer close(done)
					defer func() { panicValue = recover() }()
					err := invoke(b, context.Background(), func(context.Context) error {
						if abnormal == "panic-operation" {
							panic("same panic")
						}
						if abnormal == "exit-operation" {
							runtime.Goexit()
						}
						return remoteError
					})
					if abnormal == "normal" && err != remoteError {
						t.Error("failure identity")
					}
				}()
				<-done
				if (abnormal == "panic-operation" || abnormal == "panic-classifier") && panicValue != "same panic" {
					t.Fatal("panic value changed")
				}
				counts := b.Stats()
				checkCounts(t, counts)
				snapshots = append(snapshots, counts)
			}
			if !reflect.DeepEqual(snapshots[0], snapshots[1]) {
				t.Fatal(snapshots)
			}
		})
	}
}
func TestRunRejectionAndProbeEquivalence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, invoke := range []func(*Breaker, context.Context, func(context.Context) error) error{invokeRun, invokeExecute} {
			cfg := testConfig()
			cfg.RequestThreshold = 1
			cfg.OpenTimeout = time.Second
			b := newTestBreaker(t, cfg, remote)
			failure := errors.New("remote")
			if err := invoke(b, context.Background(), func(context.Context) error { return failure }); err != failure {
				t.Fatal(err)
			}
			calls := 0
			if err := invoke(b, context.Background(), func(context.Context) error { calls++; return nil }); !errors.Is(err, ErrOpen) || calls != 0 {
				t.Fatal("open rejection")
			}
			synctest.Sleep(time.Second)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				defer close(done)
				_ = invoke(b, context.Background(), func(context.Context) error { close(entered); <-release; return nil })
			}()
			<-entered
			if err := invoke(b, context.Background(), func(context.Context) error { calls++; return nil }); !errors.Is(err, ErrProbeLimit) || calls != 0 {
				close(release)
				<-done
				t.Fatal("probe rejection")
			}
			close(release)
			<-done
			if b.State() != StateClosed {
				t.Fatal(b.Stats())
			}
		}
	})
}
func TestRunValidationAndCanceledContext(t *testing.T) {
	b := newTestBreaker(t, testConfig(), remote)
	var nilBreaker *Breaker
	var nilContext context.Context
	for _, err := range []error{b.Run(nilContext, nil), nilBreaker.Run(context.Background(), nil), b.Run(context.Background(), nil)} {
		if !errors.Is(err, ErrInvalidCall) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("caller")
	cancel(cause)
	err := b.Run(ctx, func(context.Context) error { t.Fatal("canceled call executed"); return nil })
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || b.Stats().Requests != 0 {
		t.Fatal(err)
	}
}

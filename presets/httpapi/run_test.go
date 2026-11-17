package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

type testListener struct {
	entered, closed chan struct{}
	once, enterOnce sync.Once
	failure         error
	closes          atomic.Int64
}

func (l *testListener) Accept() (net.Conn, error) {
	l.enterOnce.Do(func() { close(l.entered) })
	if l.failure != nil {
		return nil, l.failure
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *testListener) Close() error {
	l.once.Do(func() { l.closes.Add(1); close(l.closed) })
	return nil
}
func (l *testListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func listener() *testListener {
	return &testListener{entered: make(chan struct{}), closed: make(chan struct{})}
}
func TestRunValidationAndOwnership(t *testing.T) {
	a := app(t, config(), Dependencies{})
	if e := a.Run(nil); !errors.Is(e, ErrInvalidCall) { //nolint:staticcheck // Invalid-context contract.
		t.Fatal(e)
	}
	api, ops := listener(), listener()
	if e := a.RunListeners(context.Background(), api, ops); !errors.Is(e, ErrInvalidCall) || api.closes.Load() != 1 || ops.closes.Load() != 1 {
		t.Fatal(e)
	}
	if e := a.RunListeners(context.Background(), nil, nil); !errors.Is(e, ErrInvalidCall) {
		t.Fatal(e)
	}
	c := operationsConfig()
	a = app(t, c, Dependencies{})
	api = listener()
	if e := a.RunListeners(context.Background(), api, nil); !errors.Is(e, ErrInvalidCall) || api.closes.Load() != 1 {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	api, ops = listener(), listener()
	if e := a.RunListeners(ctx, api, ops); e != nil {
		t.Fatal(e)
	}
	if api.closes.Load() != 1 || ops.closes.Load() != 1 {
		t.Fatal("ownership")
	}
	api, ops = listener(), listener()
	if e := a.RunListeners(ctx, api, ops); !errors.Is(e, ErrAlreadyStarted) || api.closes.Load() != 1 || ops.closes.Load() != 1 {
		t.Fatal(e)
	}
}
func TestFailureCancelsPeerAndOnce(t *testing.T) {
	for _, apiFails := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			a := app(t, operationsConfig(), Dependencies{})
			api, ops := listener(), listener()
			cause := errors.New("accept failure")
			if apiFails {
				api.failure = cause
			} else {
				ops.failure = cause
			}
			if e := a.RunListeners(context.Background(), api, ops); !errors.Is(e, cause) {
				t.Fatal(e)
			}
			if api.closes.Load() != 1 || ops.closes.Load() != 1 {
				t.Fatal("listeners not closed")
			}
			if e := a.Run(context.Background()); !errors.Is(e, ErrAlreadyStarted) {
				t.Fatal(e)
			}
		})
	}
	synctest.Test(t, func(t *testing.T) {
		a := app(t, config(), Dependencies{})
		api := listener()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- a.RunListeners(ctx, api, nil) }()
		<-api.entered
		extra := listener()
		if e := a.RunListeners(ctx, extra, nil); !errors.Is(e, ErrAlreadyStarted) || extra.closes.Load() != 1 {
			t.Fatal(e)
		}
		cancel()
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	})
}
func TestRunSecondBindFailure(t *testing.T) {
	occupied, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	probe, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := probe.Addr().String()
	_ = probe.Close()
	c := operationsConfig()
	c.Server.Listen = addr
	c.OperationsServer.Listen = occupied.Addr().String()
	a := app(t, c, Dependencies{})
	if e := a.Run(context.Background()); e == nil {
		t.Fatal("occupied ops accepted")
	}
	again, e := net.Listen("tcp", addr)
	if e != nil {
		t.Fatal("API listener retained", e)
	}
	_ = again.Close()
}
func TestRunLoopback(t *testing.T) {
	a := app(t, operationsConfig(), Dependencies{Routes: []Route{{Method: "GET", Pattern: "/", Handler: respond("api")}}})
	api, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ops, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		_ = api.Close()
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.RunListeners(ctx, api, ops) }()
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	for _, tt := range []struct{ addr, path string }{{api.Addr().String(), "/"}, {ops.Addr().String(), "/ping"}} {
		resp, e := client.Get("http://" + tt.addr + tt.path)
		if e != nil {
			t.Error(e)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Error(resp.StatusCode)
		}
	}
	transport.CloseIdleConnections()
	cancel()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestStressRun(t *testing.T) {
	if os.Getenv("PRESETSHTTPAPI_STRESS") != "1" {
		t.Skip("separate server campaign")
	}
	for range 10 {
		TestRunLoopback(t)
		TestFailureCancelsPeerAndOnce(t)
	}
}

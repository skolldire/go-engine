package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Listen = "127.0.0.1:0"
	if err := validateServer(cfg); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*ServerConfig)
	}{
		{"Listen", func(c *ServerConfig) { c.Listen = "" }},
		{"ReadHeaderTimeout", func(c *ServerConfig) { c.ReadHeaderTimeout = 0 }},
		{"ReadTimeout", func(c *ServerConfig) { c.ReadTimeout = -1 }},
		{"WriteTimeout", func(c *ServerConfig) { c.WriteTimeout = -1 }},
		{"IdleTimeout", func(c *ServerConfig) { c.IdleTimeout = 0 }},
		{"ShutdownTimeout", func(c *ServerConfig) { c.ShutdownTimeout = 0 }},
		{"MaxHeaderBytes", func(c *ServerConfig) { c.MaxHeaderBytes = 0 }},
		{"TLSCertFile", func(c *ServerConfig) { c.TLSKeyFile = "key" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfg
			tt.change(&c)
			var ce *ConfigError
			err := validateServer(c)
			if !errors.Is(err, ErrInvalidConfig) || !errors.As(err, &ce) || ce.Field != tt.name {
				t.Fatalf("wrong validation: %v", err)
			}
		})
	}
	cfg.ReadTimeout = 0
	cfg.WriteTimeout = 0
	if err := validateServer(cfg); err != nil {
		t.Fatal(err)
	}
}

type observedListener struct {
	net.Listener
	accepted chan struct{}
	once     sync.Once
}

func (l *observedListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.accepted) })
	return l.Listener.Accept()
}

func TestServeListenerGraceful(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultServerConfig()
	cfg.Listen = ln.Addr().String()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), struct{}{}, "kept"))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ServeListener(ctx, cfg, ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	}()
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	resp, err := client.Get("http://" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(body) != "ok" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := ln.Accept(); err == nil {
		t.Fatal("listener still open")
	}
}
func TestServeOccupiedAndCanceled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := DefaultServerConfig()
	cfg.Listen = ln.Addr().String()
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := Serve(context.Background(), cfg, h); err == nil {
		t.Fatal("occupied listener accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Serve(ctx, cfg, h); err != nil {
		t.Fatal(err)
	}
	if err := ServeListener(ctx, cfg, ln, h); err != nil {
		t.Fatal(err)
	}
	if _, err := ln.Accept(); err == nil {
		t.Fatal("canceled ServeListener retained listener")
	}
	if err := Serve(nil, cfg, h); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	if err := Serve(context.Background(), cfg, nil); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	if err := ServeListener(context.Background(), cfg, nil, h); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
}

// The fake listener makes shutdown timing deterministic without a real socket.
type blockingListener struct {
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (l *blockingListener) Accept() (net.Conn, error) {
	close(l.entered)
	<-l.closed
	return nil, net.ErrClosed
}
func (l *blockingListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *blockingListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func TestServeShutdownStopsAccept(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &blockingListener{entered: make(chan struct{}), closed: make(chan struct{})}
		cfg := DefaultServerConfig()
		cfg.Listen = "127.0.0.1:0"
		cfg.ShutdownTimeout = time.Second
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- ServeListener(ctx, cfg, l, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		}()
		<-l.entered
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

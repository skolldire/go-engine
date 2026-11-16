package router

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
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

type serverValueKey struct{}

func TestServeListenerGraceful(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultServerConfig()
	cfg.Listen = ln.Addr().String()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), serverValueKey{}, "kept"))
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
	if err := Serve(nil, cfg, h); !errors.Is(err, ErrInvalidCall) { //nolint:staticcheck // Exercise the invalid-context contract.
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

// A pipe connection permits exact virtual shutdown-deadline assertions.
type oneConnListener struct {
	conn     net.Conn
	accepted chan struct{}
	closed   chan struct{}
	once     sync.Once
	taken    bool
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	if !l.taken {
		l.taken = true
		close(l.accepted)
		return l.conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *oneConnListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *oneConnListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func TestShutdownDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, client := net.Pipe()
		defer client.Close()
		ln := &oneConnListener{conn: server, accepted: make(chan struct{}), closed: make(chan struct{})}
		entered := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})
		cfg := DefaultServerConfig()
		cfg.Listen = "127.0.0.1:0"
		cfg.ShutdownTimeout = time.Second
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- ServeListener(ctx, cfg, ln, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { defer close(finished); close(entered); <-release }))
		}()
		go func() { _, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n") }()
		<-entered
		cancel()
		err := <-done
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		close(release)
		<-finished
	})
}

type closeObservedListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *closeObservedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}
func TestConcurrentDrain(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	observed := &closeObservedListener{Listener: ln, closed: make(chan struct{})}
	cfg := DefaultServerConfig()
	cfg.Listen = ln.Addr().String()
	cfg.ShutdownTimeout = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 8)
	gate := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- ServeListener(ctx, cfg, observed, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entered <- struct{}{}
			<-gate
			_, _ = io.WriteString(w, "drained")
		}))
	}()
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			resp, e := client.Get("http://" + ln.Addr().String())
			if e != nil {
				t.Error(e)
				return
			}
			body, e := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if e != nil || string(body) != "drained" {
				t.Errorf("body=%q err=%v", body, e)
			}
		})
	}
	for range 8 {
		<-entered
	}
	cancel()
	<-observed.closed
	close(gate)
	wg.Wait()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func TestTLSStartupFailureClosesListener(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ln := &blockingListener{entered: make(chan struct{}), closed: make(chan struct{})}
		cfg := DefaultServerConfig()
		cfg.Listen = "127.0.0.1:0"
		cfg.TLSCertFile = "/nonexistent-router-test-certificate"
		cfg.TLSKeyFile = "/nonexistent-router-test-key"
		err := ServeListener(context.Background(), cfg, ln, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		if err == nil {
			t.Fatal("missing certificate accepted")
		}
		select {
		case <-ln.closed:
		default:
			t.Fatal("listener retained after TLS startup failure")
		}
	})
}

func TestPreconfiguredTLSListener(t *testing.T) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	tlsListener := tls.NewListener(ln, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	cfg := DefaultServerConfig()
	cfg.Listen = ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ServeListener(ctx, cfg, tlsListener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "tls") }))
	}()
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	resp, e := client.Get("https://" + ln.Addr().String())
	if e != nil {
		cancel()
		<-done
		t.Fatal(e)
	}
	body, e := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	tr.CloseIdleConnections()
	cancel()
	if serveErr := <-done; serveErr != nil {
		t.Fatal(serveErr)
	}
	if e != nil || string(body) != "tls" {
		t.Fatal(string(body), e)
	}
}

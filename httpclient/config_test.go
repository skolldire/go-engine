package httpclient

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// valid returns a configuration that builds its own transport.
func valid(base string) Config {
	cfg := DefaultConfig()
	cfg.BaseURL = base
	cfg.MaxResponseBytes = 1 << 20
	cfg.Transport.MaxConnsPerHost = 8
	return cfg
}

// borrowed returns a configuration for a caller-supplied RoundTripper.
func borrowed(base string) Config {
	cfg := valid(base)
	cfg.Transport = TransportConfig{}
	return cfg
}

func wantConfigError(t *testing.T, err error, field string) {
	t.Helper()
	var ce *ConfigError
	if !errors.As(err, &ce) || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ConfigError for %s, got %v", field, err)
	}
	if ce.Field != field {
		t.Fatalf("field = %q, want %q (%v)", ce.Field, field, err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	want := Config{RequestTimeout: 10 * time.Second, Transport: TransportConfig{
		ConnectTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second,
		MaxIdleConns: 100, MaxIdleConnsPerHost: 10,
	}}
	if cfg != want {
		t.Fatalf("defaults changed: %+v", cfg)
	}
	// The required fields are not defaulted, so the default alone is invalid.
	wantConfigError(t, cfg.Validate(), "BaseURL")
}

func TestValidateBaseURL(t *testing.T) {
	for _, base := range []string{
		"", "example.com", "example.com:8080/api", "/api", "//example.com", "ftp://example.com", "http://",
		"http://user:secret@example.com", "http://example.com?x=1", "http://example.com/?", "http://example.com#f",
		"http://example.com/#", "http://example.com/a/../b", "http://example.com/a/./b", "http://example.com/a//b",
		"http://example.com/a%2Fb", "http://example.com/a%20b", "http://example.com/a b", "http://example.com/a\\b",
		"http://example.com:0", "http://example.com:99999", "http:example.com", "http://exa mple.com", "mailto:a@b.c",
	} {
		cfg := valid(base)
		wantConfigError(t, cfg.Validate(), "BaseURL")
		if _, err := New(cfg, Dependencies{}); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("New accepted %q", base)
		}
	}
	for _, base := range []string{
		"http://example.com", "https://example.com/", "HTTPS://Example.COM:8443/api/v1/", "http://[::1]:8080", "http://127.0.0.1:9/api",
	} {
		if err := valid(base).Validate(); err != nil {
			t.Fatalf("%q: %v", base, err)
		}
	}
}

func TestConfigErrorNeverEchoesValues(t *testing.T) {
	cfg := valid("http://user:hunter2@example.com")
	if err := cfg.Validate(); strings.Contains(err.Error(), "hunter2") {
		t.Fatal(err)
	}
	cfg = valid("http://example.com")
	cfg.Transport.ProxyURL = "gopher://user:hunter2@proxy"
	if err := cfg.Validate(); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatal(err)
	}
}

func TestValidateFields(t *testing.T) {
	cases := []struct {
		field  string
		mutate func(*Config)
	}{
		{"RequestTimeout", func(c *Config) { c.RequestTimeout = 0 }},
		{"RequestTimeout", func(c *Config) { c.RequestTimeout = -time.Second }},
		{"MaxResponseBytes", func(c *Config) { c.MaxResponseBytes = 0 }},
		{"MaxResponseBytes", func(c *Config) { c.MaxResponseBytes = -1 }},
		{"MaxRedirects", func(c *Config) { c.MaxRedirects = -1 }},
		{"MaxRedirects", func(c *Config) { c.FollowRedirects = true }},
		{"MaxRedirects", func(c *Config) { c.MaxRedirects = 3 }},
		{"Transport.ConnectTimeout", func(c *Config) { c.Transport.ConnectTimeout = 0 }},
		{"Transport.ResponseHeaderTimeout", func(c *Config) { c.Transport.ResponseHeaderTimeout = 0 }},
		{"Transport.IdleTimeout", func(c *Config) { c.Transport.IdleTimeout = -1 }},
		{"Transport.MaxIdleConns", func(c *Config) { c.Transport.MaxIdleConns = 0 }},
		{"Transport.MaxIdleConnsPerHost", func(c *Config) { c.Transport.MaxIdleConnsPerHost = 0 }},
		{"Transport.MaxConnsPerHost", func(c *Config) { c.Transport.MaxConnsPerHost = 0 }},
		{"Transport.ProxyURL", func(c *Config) { c.Transport.ProxyURL = "proxy.internal:3128" }},
		{"Transport.ProxyURL", func(c *Config) { c.Transport.ProxyURL = "ftp://proxy.internal" }},
		{"Transport.ProxyURL", func(c *Config) { c.Transport.ProxyURL = "http://" }},
		{"Transport.ProxyURL", func(c *Config) { c.Transport.ProxyURL = "http://proxy.internal/path" }},
		{"Transport.ProxyURL", func(c *Config) { c.Transport.ProxyURL = "http://proxy.internal:0" }},
		{"Transport.ProxyURL", func(c *Config) { c.Transport.ProxyURL = "http://pro xy" }},
	}
	for _, tc := range cases {
		cfg := valid("http://example.com")
		tc.mutate(&cfg)
		wantConfigError(t, cfg.Validate(), tc.field)
		_, err := New(cfg, Dependencies{})
		wantConfigError(t, err, tc.field)
	}
	cfg := valid("http://example.com")
	cfg.FollowRedirects, cfg.MaxRedirects = true, 1
	cfg.Transport.ProxyURL = "socks5://user:pass@proxy.internal:1080"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestZeroTransportDeclaresBorrowed(t *testing.T) {
	cfg := borrowed("http://example.com")
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	// Without a RoundTripper the module must build one, and a zero
	// TransportConfig cannot describe it.
	_, err := New(cfg, Dependencies{})
	wantConfigError(t, err, "Transport.ConnectTimeout")
	if _, err := New(cfg, Dependencies{RoundTripper: http.DefaultTransport}); err != nil {
		t.Fatal(err)
	}
}

func TestBorrowedTransportRejectsMixedConfiguration(t *testing.T) {
	rt := rtFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") })
	_, err := New(valid("http://example.com"), Dependencies{RoundTripper: rt})
	wantConfigError(t, err, "Transport")
	one := borrowed("http://example.com")
	one.Transport.ProxyURL = "http://proxy.internal"
	_, err = New(one, Dependencies{RoundTripper: rt})
	wantConfigError(t, err, "Transport")
	_, err = New(borrowed("http://example.com"), Dependencies{RoundTripper: rt, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}})
	wantConfigError(t, err, "TLSConfig")
	var typed *http.Transport
	_, err = New(borrowed("http://example.com"), Dependencies{RoundTripper: typed})
	wantConfigError(t, err, "RoundTripper")
}

func TestWrapTransport(t *testing.T) {
	calls := 0
	var seen http.RoundTripper
	wrap := func(next http.RoundTripper) http.RoundTripper {
		calls++
		seen = next
		return rtFunc(func(r *http.Request) (*http.Response, error) {
			r = r.Clone(r.Context())
			r.Header.Set("X-Wrapped", "1")
			return next.RoundTrip(r)
		})
	}
	// Own transport: the wrapper receives the *http.Transport built here.
	c, err := New(valid("http://example.com"), Dependencies{WrapTransport: wrap})
	if err != nil || calls != 1 || seen != http.RoundTripper(c.owned) {
		t.Fatal(err, calls)
	}
	// Borrowed transport: the wrapper receives exactly what was lent.
	var got string
	lent := rtFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Header.Get("X-Wrapped")
		return &http.Response{StatusCode: 204, Body: http.NoBody, Request: r}, nil
	})
	calls = 0
	c, err = New(borrowed("http://example.com"), Dependencies{RoundTripper: lent, WrapTransport: wrap})
	if err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	req, _ := c.NewRequest(t.Context(), http.MethodGet, "/", nil)
	resp, err := c.Do(t.Context(), req)
	if err != nil || got != "1" || calls != 1 {
		t.Fatal(err, got, calls)
	}
	resp.Body.Close()

	_, err = New(valid("http://example.com"), Dependencies{WrapTransport: func(http.RoundTripper) http.RoundTripper { return nil }})
	wantConfigError(t, err, "WrapTransport")
	_, err = New(valid("http://example.com"), Dependencies{WrapTransport: func(http.RoundTripper) http.RoundTripper { var typed *http.Transport; return typed }})
	wantConfigError(t, err, "WrapTransport")
}

func TestOwnTransportFollowsConfig(t *testing.T) {
	// The environment must not reach the transport.
	t.Setenv("HTTP_PROXY", "http://env-proxy.invalid:3128")
	t.Setenv("HTTPS_PROXY", "http://env-proxy.invalid:3128")
	cfg := valid("https://example.com")
	cfg.Transport.MaxIdleConns, cfg.Transport.MaxIdleConnsPerHost, cfg.Transport.MaxConnsPerHost = 7, 5, 3
	cfg.Transport.IdleTimeout, cfg.Transport.ResponseHeaderTimeout, cfg.Transport.ConnectTimeout = 11*time.Second, 12*time.Second, 13*time.Second
	original := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "before"}
	c, err := New(cfg, Dependencies{TLSConfig: original})
	if err != nil {
		t.Fatal(err)
	}
	tr := c.owned
	if tr.Proxy != nil || !tr.ForceAttemptHTTP2 || tr.MaxIdleConns != 7 || tr.MaxIdleConnsPerHost != 5 || tr.MaxConnsPerHost != 3 ||
		tr.IdleConnTimeout != 11*time.Second || tr.ResponseHeaderTimeout != 12*time.Second || tr.TLSHandshakeTimeout != 13*time.Second || tr.DialContext == nil {
		t.Fatalf("transport does not follow config: %+v", tr)
	}
	original.ServerName = "after"
	if tr.TLSClientConfig == original || tr.TLSClientConfig.ServerName != "before" || tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatal("TLS configuration was not cloned")
	}
	cfg.Transport.ProxyURL = "http://proxy.internal:3128"
	c, err = New(cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	u, err := c.owned.Proxy(&http.Request{})
	if err != nil || u.Host != "proxy.internal:3128" {
		t.Fatal(u, err)
	}
	if first, again := c.HTTPClient(), c.HTTPClient(); first != again || first.Timeout != 0 || first.Jar != nil {
		t.Fatal("shared client is not stable")
	}
}

func TestNewDoesNoNetworkOrBackgroundWork(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct{}, 16)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			conn.Close()
		}
	}()
	before := runtime.NumGoroutine()
	for range 50 {
		if _, err := New(valid("http://"+ln.Addr().String()), Dependencies{}); err != nil {
			t.Fatal(err)
		}
	}
	if after := runtime.NumGoroutine(); after != before {
		t.Fatalf("New started goroutines: %d -> %d", before, after)
	}
	// Dialing ourselves afterwards proves the listener would have reported
	// any earlier connection: this one must be the first it sees.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	<-accepted
	ln.Close()
	select {
	case <-accepted:
		t.Fatal("New opened a connection")
	default:
	}
}

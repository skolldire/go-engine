package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
)

func newMux(t *testing.T) *Mux {
	t.Helper()
	m, e := New(Config{AllowedHosts: []string{"example.com", "*.example.com", "[::1]", "127.0.0.1"}})
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func output(s string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, s+r.PathValue("id")) //nolint:gosec // Local route-matching fixture, never served on a listener.
	})
}
func request(h http.Handler, method, host, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.com"+path, nil)
	r.Host = host
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestPolicyConfig(t *testing.T) {
	for _, cfg := range []Config{{}, {AllowedHosts: []string{""}}, {AllowedHosts: []string{"é.com"}}, {AllowedHosts: []string{"*.*.com"}}, {AllowedHosts: []string{".example.com"}}, {AllowedHosts: []string{"x"}, TrustedProxies: []string{"bad/24"}}, {AllowedHosts: []string{"x"}, CORS: &CORS{Origins: []string{"*"}, Credentials: true}}, {AllowedHosts: []string{"x"}, CORS: &CORS{Origins: []string{""}}}} {
		if _, e := New(cfg); !errors.Is(e, ErrInvalidConfig) {
			t.Fatalf("accepted %#v: %v", cfg, e)
		}
	}
	cfg := Config{AllowedHosts: []string{"EXAMPLE.COM."}, TrustedProxies: []string{"127.0.0.1"}, CORS: &CORS{Origins: []string{"https://web.test"}}}
	p, e := NewPolicy(cfg)
	if e != nil {
		t.Fatal(e)
	}
	cfg.AllowedHosts[0] = "bad"
	cfg.CORS.Origins[0] = "bad"
	if w := request(p.Wrap(output("ok")), "GET", "example.com", "/"); w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestHostsAndFreeze(t *testing.T) {
	m := newMux(t)
	if w := request(m, "GET", "example.com", "/"); w.Code != 503 || w.Body.Len() != 0 {
		t.Fatal(w)
	}
	if e := m.Handle("GET", "", "/", output("ok")); e != nil {
		t.Fatal(e)
	}
	if e := m.Freeze(); e != nil {
		t.Fatal(e)
	}
	_ = m.Freeze()
	for _, h := range []string{"example.com", "EXAMPLE.COM.:8080", "a.example.com", "[::1]:80", "127.0.0.1:99"} {
		if w := request(m, "GET", h, "/"); w.Code != 200 {
			t.Fatalf("host %s: %d", h, w.Code)
		}
	}
	for _, h := range []string{"bad.com", "a.b.example.com", "é.com", "example.com:bad", "example.com..", "::1"} {
		w := request(m, "GET", h, "/")
		if w.Code != 421 || w.Body.Len() != 0 {
			t.Fatal(h, w)
		}
	}
	if !errors.Is(m.Handle("GET", "", "/x", output("x")), ErrFrozen) || !errors.Is(m.Use(), ErrFrozen) {
		t.Fatal("not frozen")
	}
}
func TestRoutesAndOrder(t *testing.T) {
	m := newMux(t)
	var order []string
	for _, s := range []string{"a", "b"} {
		_ = m.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { order = append(order, s); next.ServeHTTP(w, r) })
		})
	}
	for _, r := range []struct{ h, s string }{{"", "generic"}, {"*.example.com", "wild"}, {"a.example.com", "exact"}} {
		if e := m.Handle("GET", r.h, "/item/{id}", output(r.s)); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.Handle("GET", "", "/item/{id}", output("")); !errors.Is(e, ErrDuplicateRoute) {
		t.Fatal(e)
	}
	for _, r := range []route{{method: "get", pattern: "/x"}, {method: "GET", host: "bad.com", pattern: "/x"}, {method: "GET", pattern: "GET /x"}, {method: "GET", pattern: "/item/{other}"}, {method: "GET", pattern: "/{bad"}} {
		if e := m.Handle(r.method, r.host, r.pattern, output("")); !errors.Is(e, ErrInvalidCall) {
			t.Fatalf("accepted %#v %v", r, e)
		}
	}
	_ = m.Freeze()
	for host, want := range map[string]string{"a.example.com": "exact7", "b.example.com": "wild7", "example.com": "generic7"} {
		w := request(m, "GET", host, "/item/7")
		if w.Body.String() != want {
			t.Fatalf("%s %q", host, w.Body.String())
		}
	}
	if strings.Join(order, "") != "ababab" {
		t.Fatal(order)
	}
}
func TestProxyTrust(t *testing.T) {
	for _, tt := range []struct {
		peer, xff, real, want string
		trusted               bool
	}{
		{"192.0.2.1:9", "1.1.1.1", "", "192.0.2.1", false}, {"192.0.2.1:9", "1.1.1.1", "", "192.0.2.1", true}, {"10.0.0.1:9", "6.6.6.6, 198.51.100.2, 10.0.0.2", "", "198.51.100.2", true}, {"10.0.0.1:9", "bad, 10.0.0.2", "", "10.0.0.1", true}, {"10.0.0.1:9", "", "198.51.100.7", "198.51.100.7", true},
	} {
		cfg := Config{AllowedHosts: []string{"example.com"}}
		if tt.trusted {
			cfg.TrustedProxies = []string{"10.0.0.0/8"}
		}
		p, _ := NewPolicy(cfg)
		r := httptest.NewRequest("GET", "http://example.com/", nil)
		r.RemoteAddr = tt.peer
		r.Header.Set("X-Forwarded-For", tt.xff)
		r.Header.Set("X-Real-IP", tt.real)
		p.Wrap(http.HandlerFunc(func(w http.ResponseWriter, got *http.Request) {
			ip, ok := ClientIP(got.Context())
			if !ok || ip != netip.MustParseAddr(tt.want) || got.RemoteAddr != tt.peer {
				t.Fatalf("ip=%v peer=%s", ip, got.RemoteAddr)
			}
		})).ServeHTTP(httptest.NewRecorder(), r)
	}
	if _, ok := ClientIP(context.Background()); ok {
		t.Fatal("unexpected IP")
	}
}
func TestCORS(t *testing.T) {
	p, e := NewPolicy(Config{AllowedHosts: []string{"example.com"}, CORS: &CORS{Origins: []string{"https://web.test"}, Methods: []string{"GET"}, Headers: []string{"X-Test"}, ExposedHeaders: []string{"X-Out"}, Credentials: true}})
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	h := p.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	for _, origin := range []string{"https://web.test", "https://bad.test"} {
		for _, method := range []string{"GET", "OPTIONS"} {
			r := httptest.NewRequest(method, "http://example.com/", nil)
			r.Header.Set("Origin", origin)
			if method == "OPTIONS" {
				r.Header.Set("Access-Control-Request-Method", "GET")
				r.Header.Set("Access-Control-Request-Headers", "x-test")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Header().Get("Vary") == "" {
				t.Fatal("no vary")
			}
			if origin == "https://web.test" {
				if w.Header().Get("Access-Control-Allow-Origin") != origin || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
					t.Fatal(w.Header())
				}
				if method == "OPTIONS" && w.Code != 204 {
					t.Fatal(w.Code)
				}
			} else if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("bad origin allowed")
			}
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestConcurrentFreeze(t *testing.T) {
	m := newMux(t)
	_ = m.Handle("GET", "", "/", output("ok"))
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 50 {
				w := request(m, "GET", "example.com", "/")
				if w.Code != 200 && w.Code != 503 {
					t.Error(w.Code)
				}
			}
		})
	}
	wg.Go(func() { _ = m.Freeze() })
	wg.Wait()
}
func TestStressRequests(t *testing.T) {
	if os.Getenv("ROUTER_STRESS") != "1" {
		t.Skip("separate campaign")
	}
	for range 10 {
		TestConcurrentFreeze(t)
		TestConcurrentDrain(t)
	}
}
func TestNoGoroutineLeaks(t *testing.T) {
	var b strings.Builder
	if e := pprof.Lookup("goroutineleak").WriteTo(&b, 1); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(b.String(), "total 0") {
		t.Fatal(b.String())
	}
}
func FuzzPolicy(f *testing.F) {
	f.Add("example.com", "https://web.test", "198.51.100.2, 10.0.0.1")
	f.Fuzz(func(t *testing.T, host, origin, xff string) {
		if len(host)+len(origin)+len(xff) > 4096 {
			return
		}
		p, _ := NewPolicy(Config{AllowedHosts: []string{"example.com", "*.example.com"}, TrustedProxies: []string{"10.0.0.0/8"}, CORS: &CORS{Origins: []string{"https://web.test"}}})
		r := httptest.NewRequest("GET", "http://example.com/", nil)
		r.Host = host
		r.RemoteAddr = "10.0.0.1:9"
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		p.Wrap(output("ok")).ServeHTTP(w, r)
		if w.Code != 200 && w.Code != 421 {
			t.Fatal(w.Code)
		}
	})
}
func FuzzPatterns(f *testing.F) {
	f.Add("/item/{id}")
	f.Add("/{bad")
	f.Fuzz(func(t *testing.T, p string) {
		if len(p) > 512 {
			return
		}
		m := newMux(t)
		_ = m.Handle("GET", "", p, output("ok"))
		_ = m.Freeze()
		_ = request(m, "GET", "example.com", "/item/1")
	})
}
func BenchmarkServeHTTP(b *testing.B) {
	for _, n := range []int{1, 50, 500} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for _, neutral := range []bool{false, true} {
				b.Run(fmt.Sprint(neutral), func(b *testing.B) {
					var h http.Handler
					if neutral {
						m, _ := New(Config{AllowedHosts: []string{"example.com"}})
						for i := range n {
							_ = m.Handle("GET", "", fmt.Sprintf("/route/%d", i), output("ok"))
						}
						_ = m.Freeze()
						h = m
					} else {
						m := http.NewServeMux()
						for i := range n {
							m.Handle(fmt.Sprintf("GET /route/%d", i), output("ok"))
						}
						h = m
					}
					r := httptest.NewRequest("GET", fmt.Sprintf("http://example.com/route/%d", n-1), nil)
					w := httptest.NewRecorder()
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						w.Body.Reset()
						h.ServeHTTP(w, r)
					}
				})
			}
		})
	}
}

func TestHostGroupBeforePath(t *testing.T) {
	m := newMux(t)
	_ = m.Handle("GET", "a.example.com", "/exact", output("exact"))
	_ = m.Handle("GET", "*.example.com", "/wild", output("wild"))
	_ = m.Handle("GET", "", "/generic", output("generic"))
	_ = m.Freeze()
	for _, tt := range []struct {
		host, path string
		want       int
	}{{"a.example.com", "/wild", 404}, {"a.example.com", "/generic", 404}, {"b.example.com", "/generic", 404}, {"example.com", "/generic", 200}} {
		if w := request(m, "GET", tt.host, tt.path); w.Code != tt.want {
			t.Fatalf("%s%s got%d", tt.host, tt.path, w.Code)
		}
	}
}
func TestCORSFinalContract(t *testing.T) {
	for _, c := range []CORS{{Origins: []string{"null"}}, {Origins: []string{"https://web.test/"}}, {Origins: []string{"https://web.test/path"}}, {Origins: []string{"*", "https://web.test"}}, {Origins: []string{"https://web.test"}, Headers: []string{"*"}, Credentials: true}, {Origins: []string{"https://*.test"}}, {Origins: []string{"https://web.test"}, MaxAge: -1}} {
		if _, e := NewPolicy(Config{AllowedHosts: []string{"example.com"}, CORS: &c}); !errors.Is(e, ErrInvalidConfig) {
			t.Fatalf("accepted %#v %v", c, e)
		}
	}
	for _, tt := range []struct {
		method, headers, origin string
		status                  int
	}{{"GET", "Accept", "HTTPS://WEB.TEST", 204}, {"HEAD", "Content-Type", "https://web.test", 204}, {"POST", "Range", "https://web.test", 204}, {"DELETE", "", "https://web.test", 403}, {"GET", "X-Private", "https://web.test", 403}, {"GET", "", "https://evil.test", 403}} {
		p, e := NewPolicy(Config{AllowedHosts: []string{"example.com"}, CORS: &CORS{Origins: []string{"HTTPS://WEB.TEST"}, MaxAge: 1500_000_000}})
		if e != nil {
			t.Fatal(e)
		}
		calls := 0
		r := httptest.NewRequest("OPTIONS", "http://example.com/", nil)
		r.Header.Set("Origin", tt.origin)
		r.Header.Set("Access-Control-Request-Method", tt.method)
		r.Header.Set("Access-Control-Request-Headers", tt.headers)
		w := httptest.NewRecorder()
		p.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ })).ServeHTTP(w, r)
		if w.Code != tt.status || calls != 0 || w.Body.Len() != 0 {
			t.Fatal(w.Code, calls)
		}
		if tt.status == 204 {
			if w.Header().Get("Access-Control-Max-Age") != "1" || w.Header().Get("Access-Control-Allow-Origin") != tt.origin {
				t.Fatal(w.Header())
			}
		} else if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal(w.Header())
		}
	}
	p, _ := NewPolicy(Config{AllowedHosts: []string{"example.com"}, CORS: &CORS{Origins: []string{"*"}, MaxAge: 1}})
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()
	p.Wrap(output("ok")).ServeHTTP(w, r)
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal(w.Header())
	}
	r.Header.Set("Origin", "https://any.test")
	w = httptest.NewRecorder()
	p.Wrap(output("ok")).ServeHTTP(w, r)
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal(w.Header())
	}
}

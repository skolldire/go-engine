package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skolldire/go-engine/router"
	"github.com/skolldire/go-engine/router/middleware"
	"github.com/skolldire/go-engine/router/operations"
)

func config() Config {
	c := DefaultConfig()
	c.Router.AllowedHosts = []string{"example.test", "127.0.0.1"}
	c.Server.Listen = "127.0.0.1:0"
	c.EnableOperations = false
	c.EnableRecovery = false
	return c
}
func app(t testing.TB, c Config, d Dependencies) *Application {
	t.Helper()
	a, e := New(c, d)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func request(h http.Handler, host, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://"+host+path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func respond(s string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(s)) })
}

type healthTrap struct{ operations.Health }

func (*healthTrap) Live() bool { panic("health called during construction") }
func operationsConfig() Config {
	c := config()
	c.EnableOperations = true
	c.OperationsRouter.AllowedHosts = []string{"internal.test", "127.0.0.1"}
	c.OperationsServer.Listen = "127.0.0.1:0"
	c.Operations.Health = false
	return c
}
func TestDefaultsAndValidation(t *testing.T) {
	d := DefaultConfig()
	if d.RequestTimeout != 0 || d.Server.Listen != "" || d.Router.AllowedHosts != nil || !d.RequestID.Enabled || d.RequestID.AcceptIncoming || !d.EnableRecovery || !d.EnableOperations || d.Operations.Pprof || d.OperationsServer.WriteTimeout != 45*time.Second {
		t.Fatal(d)
	}
	for _, change := range []func(*Config){func(c *Config) { c.Router.AllowedHosts = nil }, func(c *Config) { c.Server.Listen = "" }, func(c *Config) { c.RequestTimeout = -1 }, func(c *Config) { c.RequestID.Header = "bad\n" }, func(c *Config) { c.RequestTimeout = time.Second; c.Server.WriteTimeout = time.Second }} {
		c := config()
		change(&c)
		if e := c.Validate(); !errors.Is(e, ErrInvalidConfig) {
			t.Fatal(e)
		}
	}
	c := operationsConfig()
	c.Operations.Pprof = true
	c.OperationsServer.WriteTimeout = c.Operations.MaxProfileDuration
	if e := c.Validate(); !errors.Is(e, ErrInvalidConfig) {
		t.Fatal(e)
	}
	c.OperationsServer.WriteTimeout = 0
	c.Server.WriteTimeout = 0
	c.RequestTimeout = time.Hour
	if e := c.Validate(); e != nil {
		t.Fatal("unlimited rejected", e)
	}
	c = config()
	c.EnableRecovery = true
	if _, e := New(c, Dependencies{}); !errors.Is(e, ErrInvalidConfig) || !errors.Is(e, router.ErrInvalidConfig) {
		t.Fatal(e)
	}
	c = operationsConfig()
	c.Operations.Health = true
	if _, e := New(c, Dependencies{}); !errors.Is(e, operations.ErrInvalidConfig) {
		t.Fatal(e)
	}
}
func TestPureCaptureAndDelegatedErrors(t *testing.T) {
	c := config()
	routes := []Route{{Method: "GET", Pattern: "/", Handler: respond("original")}}
	ids := 0
	factory := 0
	mw := []router.Middleware{func(next http.Handler) http.Handler { factory++; return next }}
	a := app(t, c, Dependencies{Routes: routes, Middleware: mw, GenerateRequestID: func() string { ids++; return "id" }})
	if ids != 0 || factory != 1 {
		t.Fatal(ids, factory)
	}
	routes[0].Handler = respond("changed")
	mw[0] = nil
	c.Router.AllowedHosts[0] = "changed"
	c.Server.Listen = "changed"
	if w := request(a.Handler(), "example.test", "/"); w.Body.String() != "original" || w.Header().Get("X-Request-ID") != "id" {
		t.Fatal(w)
	}
	if a.ServerConfig().Listen == "changed" || a.OperationsHandler() != nil {
		t.Fatal("capture/disabled ops")
	}
	if _, ok := a.OperationsServerConfig(); ok {
		t.Fatal("operations flag")
	}
	_, e := New(config(), Dependencies{Routes: []Route{{Method: "GET", Pattern: "/", Handler: respond("")}, {Method: "GET", Pattern: "/", Handler: respond("")}}, Middleware: []router.Middleware{func(h http.Handler) http.Handler { t.Fatal("factory before validation"); return h }}})
	if !errors.Is(e, router.ErrDuplicateRoute) {
		t.Fatal(e)
	}
	c = operationsConfig()
	c.Operations.Health = true
	_ = app(t, c, Dependencies{Health: &healthTrap{}})
}
func TestOrderRecoveryTimeoutAndIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := operationsConfig()
		c.EnableRecovery = true
		c.RequestTimeout = time.Second
		var order []string
		d := Dependencies{GenerateRequestID: func() string { order = append(order, "id"); return "safe-id" }, OnPanic: func(w http.ResponseWriter, r *http.Request, v any) {
			id, _ := middleware.RequestIDFrom(r.Context())
			if id != "safe-id" || v != "private" {
				t.Error("panic context")
			}
			w.WriteHeader(500)
		}, Middleware: []router.Middleware{func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "caller")
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("timeout did not cover caller")
				}
				next.ServeHTTP(w, r)
			})
		}}, OperationsMiddleware: []router.Middleware{func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, ok := r.Context().Deadline(); ok {
					t.Fatal("API timeout applied to ops")
				}
				order = append(order, "ops")
				next.ServeHTTP(w, r)
			})
		}}, Routes: []Route{{Method: "GET", Pattern: "/panic", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private") })}, {Method: "GET", Pattern: "/wait", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done(); w.WriteHeader(202) })}}}
		a := app(t, c, d)
		if w := request(a.Handler(), "bad.test", "/panic"); w.Code != 421 || len(order) != 0 {
			t.Fatal("policy order")
		}
		if w := request(a.Handler(), "example.test", "/panic"); w.Code != 500 || w.Body.Len() != 0 {
			t.Fatal(w)
		}
		if strings.Join(order, ",") != "id,caller" {
			t.Fatal(order)
		}
		if w := request(a.Handler(), "example.test", "/wait"); w.Code != 202 {
			t.Fatal("automatic timeout response", w.Code)
		}
		if w := request(a.OperationsHandler(), "internal.test", "/ping"); w.Code != 200 {
			t.Fatal(w.Code)
		}
		if w := request(a.OperationsHandler(), "example.test", "/ping"); w.Code != 421 {
			t.Fatal("operations host policy")
		}
	})
}
func TestIncomingAndFactoryPanics(t *testing.T) {
	c := config()
	a := app(t, c, Dependencies{GenerateRequestID: func() string { return "generated" }, Routes: []Route{{Pattern: "/", Handler: respond("ok")}}})
	r := httptest.NewRequest("GET", "http://example.test/", nil)
	r.Header.Set("X-Request-ID", "untrusted")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Header().Get("X-Request-ID") != "generated" {
		t.Fatal(w.Header())
	}
	defer func() {
		if recover() != "factory" {
			t.Fatal("factory panic was recovered")
		}
	}()
	_, _ = New(c, Dependencies{Middleware: []router.Middleware{func(http.Handler) http.Handler { panic("factory") }}})
}
func TestAbortAndConcurrent(t *testing.T) {
	c := config()
	c.EnableRecovery = true
	a := app(t, c, Dependencies{OnPanic: func(http.ResponseWriter, *http.Request, any) { t.Error("abort callback") }, Routes: []Route{{Pattern: "/abort", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })}, {Pattern: "/", Handler: respond("ok")}}})
	func() {
		defer func() {
			if recover() != http.ErrAbortHandler {
				t.Error("abort not preserved")
			}
		}()
		request(a.Handler(), "example.test", "/abort")
	}()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 32 {
				if w := request(a.Handler(), "example.test", "/"); w.Code != 200 {
					t.Error(w.Code)
				}
			}
		})
	}
	wg.Wait()
	var b strings.Builder
	if e := pprof.Lookup("goroutineleak").WriteTo(&b, 1); e != nil || !strings.Contains(b.String(), "total 0") {
		t.Fatal(b.String(), e)
	}
}
func FuzzAssembly(f *testing.F) {
	f.Add(int64(0), true, "/item/{id}")
	f.Fuzz(func(t *testing.T, timeout int64, enabled bool, pattern string) {
		if len(pattern) > 256 {
			return
		}
		c := config()
		c.RequestTimeout = time.Duration(timeout)
		c.RequestID.Enabled = enabled
		_, _ = New(c, Dependencies{Routes: []Route{{Method: "GET", Pattern: pattern, Handler: respond("ok")}}})
	})
}
func BenchmarkAssembly(b *testing.B) {
	for _, mode := range []string{"manual", "preset", "construction"} {
		b.Run(mode, func(b *testing.B) {
			c := config()
			c.RequestID.Enabled = false
			d := Dependencies{Routes: []Route{{Method: "GET", Pattern: "/", Handler: respond("ok")}}}
			var h http.Handler
			switch mode {
			case "manual":
				m, _ := router.New(c.Router)
				_ = m.Handle("GET", "", "/", respond("ok"))
				_ = m.Freeze()
				h = m
			case "preset":
				h = app(b, c, d).Handler()
			}
			r := httptest.NewRequest("GET", "http://example.test/", nil)
			w := httptest.NewRecorder()
			b.ReportAllocs()
			for b.Loop() {
				if mode == "construction" {
					_, e := New(c, d)
					if e != nil {
						b.Fatal(e)
					}
				} else {
					w.Body.Reset()
					h.ServeHTTP(w, r)
				}
			}
		})
	}
}

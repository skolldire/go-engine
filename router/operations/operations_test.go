package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skolldire/go-engine/health"
)

type fakeHealth struct {
	live bool
	err  error
}

func (f *fakeHealth) Live() bool { return f.live }
func (f *fakeHealth) Ready(context.Context) (health.Report, error) {
	return health.Report{Ready: f.err == nil}, f.err
}
func (f *fakeHealth) Check(context.Context) (health.Report, error) { return health.Report{}, f.err }
func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://internal"+path, nil))
	return w
}
func TestConfig(t *testing.T) {
	cfg := DefaultConfig()
	if e := cfg.Validate(); e != nil {
		t.Fatal(e)
	}
	cfg.MaxProfileDuration = 0
	if !errors.Is(cfg.Validate(), ErrInvalidConfig) {
		t.Fatal("invalid accepted")
	}
	cfg = DefaultConfig()
	var typedNil *fakeHealth
	for _, dep := range []Health{nil, typedNil} {
		if _, e := New(cfg, Dependencies{Health: dep}); !errors.Is(e, ErrInvalidConfig) {
			t.Fatal(e)
		}
	}
	cfg.Health = false
	if _, e := New(cfg, Dependencies{}); e != nil {
		t.Fatal(e)
	}
}
func TestRoutes(t *testing.T) {
	for _, err := range []error{nil, health.ErrNotReady, errors.New("private-error")} {
		h, e := New(DefaultConfig(), Dependencies{Health: &fakeHealth{live: err == nil, err: err}, Metrics: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, "metrics") })})
		if e != nil {
			t.Fatal(e)
		}
		for path, want := range map[string]int{"/ping": 200, "/live": 200, "/ready": 200, "/deps": 200, "/missing": 404, "/debug/pprof/": 404, "/metrics": 200} {
			if err != nil {
				if path == "/live" {
					want = 503
				}
				if path == "/ready" {
					want = 500
					if errors.Is(err, health.ErrNotReady) {
						want = 503
					}
				}
				if path == "/deps" {
					want = 500
				}
			}
			w := get(h, path)
			if w.Code != want {
				t.Fatalf("%s got%d want%d", path, w.Code, want)
			}
			if strings.Contains(w.Body.String(), "private-error") {
				t.Fatal("secret leaked")
			}
		}
		if w := get(h, "/ping"); w.Body.String() != `{"status":"ok","message":"pong"}` {
			t.Fatal(w.Body.String())
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "http://internal/ping", nil))
		if w.Code != 405 {
			t.Fatal(w.Code)
		}
	}
}
func TestRealHealthPrivate(t *testing.T) {
	reg, e := health.New(health.DefaultConfig())
	if e != nil {
		t.Fatal(e)
	}
	_ = reg.Register("db", health.CheckerFunc(func(context.Context) error { return errors.New("private-password") }))
	_ = reg.Register("panic", health.CheckerFunc(func(context.Context) error { panic("private-value") }))
	h, e := New(DefaultConfig(), Dependencies{Health: reg})
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/ready", "/deps"} {
		w := get(h, path)
		want := 503
		if path == "/deps" {
			want = 200
		}
		if w.Code != want {
			t.Fatal(path, w.Code)
		}
		var dto map[string]json.RawMessage
		if e := json.Unmarshal(w.Body.Bytes(), &dto); e != nil {
			t.Fatal(e)
		}
		if len(dto) != 2 && path == "/ready" || len(dto) != 3 && path == "/deps" {
			t.Fatal(dto)
		}
		var checks []map[string]json.RawMessage
		_ = json.Unmarshal(dto["checks"], &checks)
		for _, check := range checks {
			if len(check) != 4 || check["duration_ms"] == nil {
				t.Fatal(check)
			}
		}
		for _, secret := range []string{"private-password", "private-value", "Stack", "Value", "Err"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal(secret)
			}
		}
	}
}
func TestPprof(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Health = false
	cfg.Pprof = true
	h, e := New(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	if w := get(h, "/debug/pprof/"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/debug/pprof/profile?seconds=31", "/debug/pprof/trace?seconds=31", "/debug/pprof/profile?seconds=NaN", "/debug/pprof/trace?seconds=-1", "/debug/pprof/profile?seconds=0", "/debug/pprof/profile?seconds=1&seconds=2"} {
		if w := get(h, path); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://x/debug/pprof/profile", nil)
	if !profileSeconds(r, 2*time.Second) || r.URL.Query().Get("seconds") != "2" {
		t.Fatal(r.URL)
	}
}
func FuzzSeconds(f *testing.F) {
	f.Add("31")
	f.Add("NaN")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 512 {
			return
		}
		r := httptest.NewRequest("GET", "http://x/debug/pprof/trace", nil)
		q := r.URL.Query()
		q.Set("seconds", s)
		r.URL.RawQuery = q.Encode()
		_ = profileSeconds(r, 30*time.Second)
	})
}
func BenchmarkReady(b *testing.B) {
	r, _ := health.New(health.DefaultConfig())
	for i := range 10 {
		_ = r.Register(fmt.Sprint(i), health.CheckerFunc(func(context.Context) error { return nil }))
	}
	h, _ := New(DefaultConfig(), Dependencies{Health: r})
	req := httptest.NewRequest("GET", "http://internal/ready", nil)
	w := httptest.NewRecorder()
	b.ReportAllocs()
	for b.Loop() {
		w.Body.Reset()
		h.ServeHTTP(w, req)
	}
}

func TestJSONFinalContract(t *testing.T) {
	h, _ := New(DefaultConfig(), Dependencies{Health: &fakeHealth{err: errors.New("private error")}})
	for _, path := range []string{"/ready", "/deps"} {
		w := get(h, path)
		if w.Code != 500 || w.Body.String() != `{"error":"internal_error"}` || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
			t.Fatal(path, w.Code, w.Body.String(), w.Header())
		}
	}
	cfg := DefaultConfig()
	cfg.Pprof = true
	cfg.Health = false
	h, _ = New(cfg, Dependencies{})
	w := get(h, "/debug/pprof/trace?seconds=31")
	if w.Body.String() != `{"error":"invalid_seconds"}` || w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/ping", "/debug/pprof/trace?seconds=31"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("HEAD", "http://internal"+path, nil))
		if w.Body.Len() != 0 {
			t.Fatal("HEAD body", w.Body.String())
		}
	}
	cfg.Health = true
	h, _ = New(cfg, Dependencies{Health: &fakeHealth{live: true}})
	w = get(h, "/deps")
	var dto map[string]json.RawMessage
	if e := json.Unmarshal(w.Body.Bytes(), &dto); e != nil || len(dto) != 3 || string(dto["live"]) != "true" || string(dto["checks"]) != "[]" {
		t.Fatal(w.Body.String())
	}
}

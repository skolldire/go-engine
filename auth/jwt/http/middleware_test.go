package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/skolldire/go-engine/auth/jwt"
)

type fakeVerifier struct {
	calls  atomic.Int64
	verify func(context.Context, string) (jwt.Claims, error)
}

func (v *fakeVerifier) Verify(ctx context.Context, token string) (jwt.Claims, error) {
	v.calls.Add(1)
	if v.verify != nil {
		return v.verify(ctx, token)
	}
	return jwt.Claims{Subject: "subject", Groups: []string{"staff"}}, nil
}
func configured(t testing.TB, c Config, d Dependencies) func(nethttp.Handler) nethttp.Handler {
	t.Helper()
	mw, e := Middleware(c, d)
	if e != nil {
		t.Fatal(e)
	}
	return mw
}
func serve(h nethttp.Handler, path, authorization string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://example.test"+path, nil)
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func next204() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { w.WriteHeader(204) })
}
func TestConfig(t *testing.T) {
	if e := DefaultConfig().Validate(); e != nil {
		t.Fatal(e)
	}
	for _, c := range []Config{{}, {Header: "bad\nname"}, {Header: "Authorization", SkipPaths: []string{"relative"}}, {Header: "Authorization", SkipPaths: []string{"/a/../public"}}, {Header: "Authorization", RequiredGroups: []string{""}}} {
		if e := c.Validate(); !errors.Is(e, jwt.ErrInvalidConfig) {
			t.Fatal(c, e)
		}
	}
	var nilVerifier *fakeVerifier
	for _, v := range []Verifier{nil, nilVerifier} {
		if _, e := Middleware(DefaultConfig(), Dependencies{Verifier: v}); !errors.Is(e, jwt.ErrInvalidConfig) {
			t.Fatal(e)
		}
	}
	if _, e := Middleware(Config{}, Dependencies{}); e == nil {
		t.Fatal("literal defaulted")
	}
	v := &fakeVerifier{}
	mw := configured(t, DefaultConfig(), Dependencies{Verifier: v})
	if v.calls.Load() != 0 || mw(nil) != nil {
		t.Fatal("construction executed callback or nil handler substituted")
	}
}
func TestBearerAndCustomHeader(t *testing.T) {
	for _, tt := range []struct {
		header string
		want   int
	}{{"Bearer token", 204}, {"bearer token", 204}, {"bEaReR   token", 204}, {"Basic token", 401}, {"Bearer", 401}, {"Bearer ", 401}, {"", 401}, {"Bearer\ttoken", 401}, {"Bearer a b", 401}} {
		v := &fakeVerifier{verify: func(ctx context.Context, token string) (jwt.Claims, error) {
			if token != "token" {
				t.Error(token)
			}
			return jwt.Claims{}, nil
		}}
		h := configured(t, DefaultConfig(), Dependencies{Verifier: v})(next204())
		w := serve(h, "/", tt.header)
		if w.Code != tt.want {
			t.Fatal(tt.header, w.Code)
		}
		if tt.want == 401 && v.calls.Load() != 0 {
			t.Fatal("missing token verified")
		}
	}
	c := DefaultConfig()
	c.Header = "X-Auth"
	v := &fakeVerifier{}
	h := configured(t, c, Dependencies{Verifier: v})(next204())
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Auth", "Bearer token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || v.calls.Load() != 1 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Add("X-Auth", "Bearer first")
	r.Header.Add("X-Auth", "Bearer second")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("ambiguous headers accepted")
	}
}
func TestSkipPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SkipPaths = []string{"/health", "/public/"}
	v := &fakeVerifier{}
	h := configured(t, cfg, Dependencies{Verifier: v})(next204())
	cfg.SkipPaths[0] = "/private"
	for _, tt := range []struct {
		path string
		want int
	}{{"/health", 204}, {"/healthz", 401}, {"/health/a", 401}, {"/health-admin", 401}, {"/public/", 204}, {"/public/a", 204}, {"/public", 401}, {"/public/../private", 401}, {"/public//private", 401}, {"/public/%2e%2e/private", 401}, {"/public/%2Fprivate", 401}, {"/public/%252e%252e/private", 401}, {"/public/%61", 401}, {"/public/a?query=1", 204}, {"/private", 401}} {
		if w := serve(h, tt.path, ""); w.Code != tt.want {
			t.Fatalf("%s got%d want%d", tt.path, w.Code, tt.want)
		}
	}
	if v.calls.Load() != 0 {
		t.Fatal("skip/missing path called verifier")
	}
}

type privateError struct{}

func (*privateError) Error() string { panic("must not format verifier error") }
func TestStatusAndPrivateResponse(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{{jwt.ErrInvalidToken, 401, "invalid_token"}, {jwt.ErrUnknownKey, 401, "invalid_token"}, {jwt.ErrExpiredToken, 401, "expired_token"}, {jwt.ErrForbidden, 403, "forbidden"}, {jwt.ErrKeySetUnavailable, 503, "auth_unavailable"}, {&privateError{}, 500, "internal_error"}, {errors.Join(jwt.ErrKeySetUnavailable, jwt.ErrExpiredToken), 503, "auth_unavailable"}} {
		v := &fakeVerifier{verify: func(context.Context, string) (jwt.Claims, error) { return jwt.Claims{}, tt.err }}
		h := configured(t, DefaultConfig(), Dependencies{Verifier: v})(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) { t.Fatal("failed verification reached next") }))
		w := serve(h, "/", "Bearer private-token")
		if w.Code != tt.status || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatal(w.Code, w.Header())
		}
		var problem map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &problem); e != nil || len(problem) != 4 || problem["type"] != "about:blank" || problem["status"] != float64(tt.status) || problem["title"] != nethttp.StatusText(tt.status) || problem["code"] != tt.code {
			t.Fatal(w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Body.String())
		}
		challenge := w.Header().Get("WWW-Authenticate")
		if tt.status == 401 && challenge != `Bearer error="invalid_token"` || tt.status != 401 && challenge != "" {
			t.Fatal(challenge)
		}
	}
	h := configured(t, DefaultConfig(), Dependencies{Verifier: &fakeVerifier{}})(next204())
	w := serve(h, "/", "")
	if w.Header().Get("WWW-Authenticate") != "Bearer" || !strings.Contains(w.Body.String(), `"code":"missing_token"`) {
		t.Fatal(w.Header(), w.Body.String())
	}
}

type watchedWriter struct {
	headers int
	writes  int
	status  int
	header  nethttp.Header
}

func (w *watchedWriter) Header() nethttp.Header {
	w.headers++
	if w.header == nil {
		w.header = make(nethttp.Header)
	}
	return w.header
}
func (w *watchedWriter) Write(b []byte) (int, error) { w.writes++; return len(b), nil }
func (w *watchedWriter) WriteHeader(n int)           { w.status = n }
func TestOnError(t *testing.T) {
	original := &privateError{}
	calls := 0
	v := &fakeVerifier{verify: func(context.Context, string) (jwt.Claims, error) { return jwt.Claims{}, original }}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer token")
	w := &watchedWriter{}
	h := configured(t, DefaultConfig(), Dependencies{Verifier: v, OnError: func(got nethttp.ResponseWriter, req *nethttp.Request, status int, e error) {
		calls++
		if got != w || req != r || e != original || status != 500 {
			t.Error("callback arguments changed")
		}
	}})(next204())
	h.ServeHTTP(w, r)
	if calls != 1 || w.headers != 0 || w.writes != 0 || w.status != 0 {
		t.Fatal("middleware wrote during callback", calls, w)
	}
}
func TestGroupsAndCapturedConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequiredGroups = []string{"admin", "staff"}
	v := &fakeVerifier{}
	h := configured(t, cfg, Dependencies{Verifier: v})(next204())
	cfg.RequiredGroups[1] = "other"
	if w := serve(h, "/", "Bearer token"); w.Code != 204 {
		t.Fatal(w.Code)
	}
	cfg.RequiredGroups = []string{"admin"}
	h = configured(t, cfg, Dependencies{Verifier: v})(next204())
	if w := serve(h, "/", "Bearer token"); w.Code != 403 {
		t.Fatal(w.Code)
	}
}

type valueKey struct{}

func TestClaimsCopiesAndContext(t *testing.T) {
	original := jwt.Claims{Subject: "user", Audience: []string{"app"}, Groups: []string{"staff"}, Custom: map[string]any{"nested": map[string]any{"list": []any{"value"}}, "number": json.Number("9007199254740993")}}
	ctx, cancel := context.WithCancelCause(context.WithValue(context.Background(), valueKey{}, "context-value"))
	cause := errors.New("caller canceled")
	cancel(cause)
	v := &fakeVerifier{verify: func(got context.Context, token string) (jwt.Claims, error) {
		if got != ctx {
			t.Fatal("verifier context changed")
		}
		return original, nil
	}}
	next := nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Context() == ctx || r.Context().Value(valueKey{}) != "context-value" || context.Cause(r.Context()) != cause {
			t.Fatal("context semantics changed")
		}
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims.Subject != "user" {
			t.Fatal(claims)
		}
		claims.Audience[0] = "mutated"
		claims.Groups[0] = "mutated"
		claims.Custom["nested"].(map[string]any)["list"].([]any)[0] = "mutated"
		again, _ := ClaimsFromContext(r.Context())
		if again.Audience[0] != "app" || again.Groups[0] != "staff" || again.Custom["nested"].(map[string]any)["list"].([]any)[0] != "value" {
			t.Fatal("getter shares state")
		}
	})
	r := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer token")
	configured(t, DefaultConfig(), Dependencies{Verifier: v})(next).ServeHTTP(httptest.NewRecorder(), r)
	with := ContextWithClaims(context.Background(), original)
	original.Custom["nested"].(map[string]any)["list"].([]any)[0] = "after"
	copy, _ := ClaimsFromContext(with)
	if copy.Custom["nested"].(map[string]any)["list"].([]any)[0] != "value" || copy.Custom["number"] != json.Number("9007199254740993") {
		t.Fatal("setter shares state")
	}
	if _, ok := ClaimsFromContext(context.Background()); ok {
		t.Fatal("unexpected claims")
	}
	if _, ok := ClaimsFromContext(nil); ok { //nolint:staticcheck // Exercise the nil getter.
		t.Fatal("nil getter")
	}
}
func TestConcurrentAndPanics(t *testing.T) {
	v := &fakeVerifier{}
	h := configured(t, DefaultConfig(), Dependencies{Verifier: v})(next204())
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 32 {
				if w := serve(h, "/", "Bearer token"); w.Code != 204 {
					t.Error(w.Code)
				}
			}
		})
	}
	wg.Wait()
	if v.calls.Load() != 1024 {
		t.Fatal(v.calls.Load())
	}
	defer func() {
		if recover() != "same panic" {
			t.Fatal("panic not propagated")
		}
	}()
	v = &fakeVerifier{verify: func(context.Context, string) (jwt.Claims, error) { panic("same panic") }}
	serve(configured(t, DefaultConfig(), Dependencies{Verifier: v})(next204()), "/", "Bearer token")
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
func FuzzPathsAndAuthorization(f *testing.F) {
	f.Add("/public/a", "Bearer token")
	f.Add("/public/%2e%2e/private", "Basic token")
	f.Fuzz(func(t *testing.T, path, auth string) {
		if len(path)+len(auth) > 4096 {
			return
		}
		r := httptest.NewRequest("GET", "http://example.test/", nil)
		u, e := url.ParseRequestURI(path)
		if e != nil || !strings.HasPrefix(path, "/") {
			return
		}
		r.URL = u
		r.Header.Set("Authorization", auth)
		v := &fakeVerifier{verify: func(ctx context.Context, token string) (jwt.Claims, error) {
			if token != "token" {
				return jwt.Claims{}, jwt.ErrInvalidToken
			}
			return jwt.Claims{}, nil
		}}
		cfg := DefaultConfig()
		cfg.SkipPaths = []string{"/public/", "/health"}
		w := httptest.NewRecorder()
		configured(t, cfg, Dependencies{Verifier: v})(next204()).ServeHTTP(w, r)
		if w.Code != 204 && w.Code != 401 {
			t.Fatal(w.Code)
		}
		if w.Code == 204 && v.calls.Load() == 0 {
			p := r.URL.Path
			public := p == "/health" || strings.HasPrefix(p, "/public/")
			if !public || strings.Contains(p, "//") || strings.Contains(p, "/../") || strings.Contains(p, "/./") || r.URL.RawPath != "" || strings.Contains(r.URL.EscapedPath(), "%") {
				t.Fatal("ambiguous unauthenticated path")
			}
		}
	})
}
func BenchmarkMiddleware(b *testing.B) {
	h := configured(b, DefaultConfig(), Dependencies{Verifier: &fakeVerifier{}})(next204())
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	b.ReportAllocs()
	for b.Loop() {
		h.ServeHTTP(w, r)
	}
}

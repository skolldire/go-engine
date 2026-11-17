package resty

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	restyv2 "github.com/go-resty/resty/v2"

	"github.com/skolldire/go-engine/httpclient"
)

type item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// upstream is the loopback server every test talks to.
type upstream struct {
	hits    atomic.Int64
	seen    sync.Map // path -> *http.Request header snapshot
	release chan struct{}
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.hits.Add(1)
	u.seen.Store(r.URL.Path, r.Header.Clone())
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("n"))
	switch r.URL.Path {
	case "/api/echo":
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		w.Header().Set("X-Method", r.Method)
		io.Copy(w, r.Body)
	case "/api/status":
		w.WriteHeader(n)
		io.WriteString(w, "private upstream detail")
	case "/api/fixed":
		w.Header().Set("Content-Length", strconv.Itoa(n))
		w.Write(bytes.Repeat([]byte("a"), n))
	case "/api/stream":
		w.(http.Flusher).Flush()
		w.Write(bytes.Repeat([]byte("a"), n))
	case "/api/gzip":
		w.Header().Set("Content-Encoding", "gzip")
		z := gzip.NewWriter(w)
		z.Write(bytes.Repeat([]byte("a"), n))
		z.Close()
	case "/api/to":
		w.Header().Set("Location", q.Get("u"))
		w.WriteHeader(http.StatusFound)
	case "/api/stall":
		select {
		case <-r.Context().Done():
		case <-u.release:
		}
	default:
		io.WriteString(w, "ok")
	}
}

func (u *upstream) header(t *testing.T, path string) http.Header {
	t.Helper()
	v, ok := u.seen.Load(path)
	if !ok {
		t.Fatalf("%s was never requested", path)
	}
	return v.(http.Header)
}

func start(t testing.TB) (*upstream, *httptest.Server) {
	t.Helper()
	u := &upstream{release: make(chan struct{})}
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(u.release) })
	return u, srv
}

func adapter(t testing.TB, base string, deps httpclient.Dependencies, mutate func(*httpclient.Config)) (*Client, *httpclient.Client) {
	t.Helper()
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = base
	cfg.MaxResponseBytes = 1024
	cfg.RequestTimeout = 5 * time.Second
	cfg.Transport.MaxConnsPerHost = 8
	if mutate != nil {
		mutate(&cfg)
	}
	hc, err := httpclient.New(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hc.CloseIdleConnections)
	c, err := New(Dependencies{HTTPClient: hc})
	if err != nil {
		t.Fatal(err)
	}
	return c, hc
}

func TestInvalidDependencies(t *testing.T) {
	var typed *httpclient.Client
	for _, deps := range []Dependencies{{}, {HTTPClient: typed}, {HTTPClient: &httpclient.Client{}}} {
		c, err := New(deps)
		var ce *httpclient.ConfigError
		if c != nil || !errors.As(err, &ce) || ce.Field != "HTTPClient" || !errors.Is(err, httpclient.ErrInvalidConfig) {
			t.Fatal(c, err)
		}
	}
}

func TestJSONRoundTripAndVerbs(t *testing.T) {
	up, srv := start(t)
	c, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	var got item
	resp, err := c.R(t.Context()).SetBody(item{ID: 7, Name: "bolt"}).SetResult(&got).Post("/echo")
	if err != nil || CheckStatus(resp) != nil || got != (item{ID: 7, Name: "bolt"}) {
		t.Fatal(resp, err, got)
	}
	if ct := up.header(t, "/api/echo").Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type %q", ct)
	}
	headers := map[string]string{"X-Trace": "abc"}
	calls := map[string]func() (*restyv2.Response, error){
		http.MethodGet:    func() (*restyv2.Response, error) { return c.Get(t.Context(), "/echo", headers) },
		http.MethodDelete: func() (*restyv2.Response, error) { return c.Delete(t.Context(), "/echo", headers) },
		http.MethodPost:   func() (*restyv2.Response, error) { return c.Post(t.Context(), "/echo", item{ID: 1}, headers) },
		http.MethodPut:    func() (*restyv2.Response, error) { return c.Put(t.Context(), "/echo", item{ID: 1}, headers) },
		http.MethodPatch:  func() (*restyv2.Response, error) { return c.Patch(t.Context(), "/echo", item{ID: 1}, headers) },
	}
	for method, call := range calls {
		resp, err := call()
		if err != nil || resp.StatusCode() != http.StatusOK || resp.Header().Get("X-Method") != method || up.header(t, "/api/echo").Get("X-Trace") != "abc" {
			t.Fatal(method, resp, err)
		}
		var body item
		hasBody := method != http.MethodGet && method != http.MethodDelete
		if hasBody && (json.Unmarshal(resp.Body(), &body) != nil || body.ID != 1) {
			t.Fatalf("%s lost its body: %q", method, resp.Body())
		}
		if !hasBody && len(resp.Body()) != 0 {
			t.Fatalf("%s sent a body: %q", method, resp.Body())
		}
	}
	// A nil body sends none, and a nil context is refused before resty.
	if resp, err := c.Post(t.Context(), "/echo", nil, nil); err != nil || len(resp.Body()) != 0 {
		t.Fatal(resp, err)
	}
	var none context.Context // the nil context is the case under test
	if _, err := c.Get(none, "/echo", nil); !errors.Is(err, httpclient.ErrNilContext) {
		t.Fatal(err)
	}
}

func TestNon2xxAndCheckStatus(t *testing.T) {
	up, srv := start(t)
	c, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	if err := CheckStatus(nil); !errors.Is(err, httpclient.ErrInvalidCall) {
		t.Fatal(err)
	}
	if err := CheckStatus(&restyv2.Response{}); !errors.Is(err, httpclient.ErrInvalidCall) {
		t.Fatal(err)
	}
	for _, code := range []int{400, 404, 429, 500, 503} {
		before := up.hits.Load()
		resp, err := c.Get(t.Context(), "/status?n="+strconv.Itoa(code), nil)
		// The legacy client returned an error here, with the body in it.
		if err != nil || resp.StatusCode() != code || string(resp.Body()) != "private upstream detail" {
			t.Fatal(code, resp, err)
		}
		err = CheckStatus(resp)
		var status *httpclient.StatusError
		if !errors.As(err, &status) || status.StatusCode != code || status.Response != resp.RawResponse {
			t.Fatal(err)
		}
		if msg := err.Error(); msg != "httpclient: unexpected status "+strconv.Itoa(code) || strings.Contains(msg, "private") {
			t.Fatal(msg)
		}
		// One request per call: resty's retry loop is off.
		if up.hits.Load() != before+1 {
			t.Fatalf("status %d was sent %d times", code, up.hits.Load()-before)
		}
	}
}

func TestRestyRetryStaysOff(t *testing.T) {
	up, srv := start(t)
	attempts := atomic.Int64{}
	count := func(next http.RoundTripper) http.RoundTripper {
		return roundTripper(func(r *http.Request) (*http.Response, error) {
			attempts.Add(1)
			return next.RoundTrip(r)
		})
	}
	c, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{WrapTransport: count}, nil)
	if c.resty.RetryCount != 0 || len(c.resty.RetryConditions) != 0 {
		t.Fatal("resty retry is configured")
	}
	// A request-level condition asking for a retry changes nothing while
	// the client-level count is zero, and the client is not reachable.
	always := func(*restyv2.Response, error) bool { return true }
	resp, err := c.R(t.Context()).AddRetryCondition(always).Get("/status?n=503")
	if err != nil || resp.StatusCode() != http.StatusServiceUnavailable || up.hits.Load() != 1 || attempts.Load() != 1 {
		t.Fatal(resp, err, up.hits.Load(), attempts.Load())
	}
	// A transport failure is attempted once as well.
	srv.CloseClientConnections()
	srv.Close()
	attempts.Store(0)
	if _, err := c.R(t.Context()).AddRetryCondition(always).Get("/status?n=200"); err == nil || attempts.Load() != 1 {
		t.Fatal(err, attempts.Load())
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInheritsOriginRule(t *testing.T) {
	up, srv := start(t)
	other, otherSrv := start(t)
	c, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	for _, target := range []string{otherSrv.URL + "/steal", "http://203.0.113.1/steal", "https" + strings.TrimPrefix(srv.URL, "http") + "/api/x"} {
		resp, err := c.Get(t.Context(), target, map[string]string{"Authorization": "Bearer secret"})
		if !errors.Is(err, httpclient.ErrOriginNotAllowed) || (resp != nil && resp.RawResponse != nil) {
			t.Fatal(target, resp, err)
		}
		if _, err := c.R(t.Context()).Get(target); !errors.Is(err, httpclient.ErrOriginNotAllowed) {
			t.Fatal(target, err)
		}
	}
	if up.hits.Load() != 0 || other.hits.Load() != 0 {
		t.Fatal("a refused call reached a server")
	}
	// An absolute URL on the allowed origin passes, as it does in httpclient.
	if resp, err := c.Get(t.Context(), srv.URL+"/api/echo", nil); err != nil || resp.StatusCode() != http.StatusOK {
		t.Fatal(resp, err)
	}
	// resty treats a scheme-relative reference as a path under the base.
	if _, err := c.Get(t.Context(), "//203.0.113.1/steal", nil); err != nil || up.hits.Load() != 2 {
		t.Fatal(err, up.hits.Load())
	}
}

func TestInheritsResponseLimit(t *testing.T) {
	_, srv := start(t)
	c, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	for _, path := range []string{"/fixed?n=1024", "/stream?n=1024", "/gzip?n=1024"} {
		resp, err := c.Get(t.Context(), path, nil)
		if err != nil || len(resp.Body()) != 1024 {
			t.Fatal(path, err)
		}
	}
	// resty buffers the body; the read error it meets is what comes back.
	for _, path := range []string{"/fixed?n=1025", "/stream?n=1025", "/stream?n=300000", "/gzip?n=1025", "/gzip?n=3000000"} {
		if _, err := c.Get(t.Context(), path, nil); !errors.Is(err, httpclient.ErrResponseTooLarge) {
			t.Fatal(path, err)
		}
	}
	// Streaming is bounded too.
	resp, err := c.R(t.Context()).SetDoNotParseResponse(true).Get("/stream?n=300000")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.RawBody())
	resp.RawBody().Close()
	if !errors.Is(err, httpclient.ErrResponseTooLarge) || len(data) != 1024 {
		t.Fatal(len(data), err)
	}
	// Asking for gzip by hand would move decoding past the limit.
	if _, err := c.R(t.Context()).SetHeader("Accept-Encoding", "gzip").Get("/gzip?n=3000000"); !errors.Is(err, httpclient.ErrInvalidCall) {
		t.Fatal(err)
	}
}

func TestInheritsRedirectsAndTimeout(t *testing.T) {
	up, srv := start(t)
	other, otherSrv := start(t)
	auth := map[string]string{"Authorization": "Bearer secret", "Cookie": "session=secret"}

	off, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	resp, err := off.Get(t.Context(), "/to?u="+otherSrv.URL+"/landed", auth)
	if err != nil || resp.StatusCode() != http.StatusFound || other.hits.Load() != 0 {
		t.Fatal(resp, err)
	}

	on, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, func(cfg *httpclient.Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 1 })
	resp, err = on.Get(t.Context(), "/to?u="+otherSrv.URL+"/landed", auth)
	if err != nil || resp.StatusCode() != http.StatusOK {
		t.Fatal(resp, err)
	}
	if got := other.header(t, "/landed"); got.Get("Authorization") != "" || got.Get("Cookie") != "" {
		t.Fatalf("cross-origin redirect carried credentials: %v", got)
	}
	if got := up.header(t, "/api/to"); got.Get("Authorization") != "Bearer secret" {
		t.Fatal("first hop lost its credentials")
	}
	if _, err = on.Get(t.Context(), "/to?u=/api/to%3Fu%3D/api/echo", auth); !errors.Is(err, httpclient.ErrRedirectNotAllowed) {
		t.Fatal(err)
	}

	quick, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, func(cfg *httpclient.Config) { cfg.RequestTimeout = 100 * time.Millisecond })
	if _, err = quick.Get(t.Context(), "/stall", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestSharedHTTPClientIsNotModified(t *testing.T) {
	_, srv := start(t)
	c, hc := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	shared := hc.HTTPClient()
	before := *shared
	// Digest authentication makes resty swap the transport of the client
	// it holds for the duration of a request.
	if _, err := c.R(t.Context()).SetDigestAuth("user", "pass").Get("/echo"); err != nil {
		t.Fatal(err)
	}
	if shared != hc.HTTPClient() || shared.Transport != before.Transport || shared.Timeout != before.Timeout || shared.Jar != before.Jar {
		t.Fatal("the shared http.Client was modified")
	}
}

func TestConcurrentUse(t *testing.T) {
	_, srv := start(t)
	c, _ := adapter(t, srv.URL+"/api", httpclient.Dependencies{}, nil)
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			for i := range 20 {
				var got item
				resp, err := c.R(t.Context()).SetBody(item{ID: worker*100 + i}).SetResult(&got).Post("/echo")
				if err != nil || CheckStatus(resp) != nil || got.ID != worker*100+i {
					t.Error(resp, err, got)
					return
				}
			}
		})
	}
	wg.Wait()
}

// FuzzPath is the one target the catalog requires of a production module.
// Whatever path is given, the adapter sends nothing to another origin.
func FuzzPath(f *testing.F) {
	for _, seed := range []string{"/items", "items?x=1", "//evil.example/x", "https://evil.example/", "https://api.example.com/v1/x", "http://api.example.com/x", "/../admin", "https://user:pw@api.example.com/", "{id}", "\\\\evil"} {
		f.Add(seed)
	}
	var last atomic.Pointer[http.Request]
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://api.example.com/v1"
	cfg.MaxResponseBytes = 1024
	cfg.Transport = httpclient.TransportConfig{}
	hc, err := httpclient.New(cfg, httpclient.Dependencies{RoundTripper: roundTripper(func(r *http.Request) (*http.Response, error) {
		last.Store(r)
		return &http.Response{StatusCode: 204, Body: http.NoBody, Header: http.Header{}, Request: r}, nil
	})})
	if err != nil {
		f.Fatal(err)
	}
	c, err := New(Dependencies{HTTPClient: hc})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, path string) {
		if len(path) > 512 {
			return
		}
		last.Store(nil)
		_, err := c.Get(context.Background(), path, nil)
		sent := last.Load()
		if sent == nil {
			if err == nil {
				t.Fatalf("%q: no request and no error", path)
			}
			return
		}
		if u := sent.URL; u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "api.example.com") || (u.Port() != "" && u.Port() != "443") || u.User != nil {
			t.Fatalf("%q left the origin: %q", path, u)
		}
	})
}

func BenchmarkGet(b *testing.B) {
	_, srv := start(b)
	c, _ := adapter(b, srv.URL+"/api", httpclient.Dependencies{}, nil)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		resp, err := c.Get(ctx, "/echo", nil)
		if err != nil || resp.StatusCode() != http.StatusOK {
			b.Fatal(err)
		}
	}
}

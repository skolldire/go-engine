package httpclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// server is a loopback httptest server that counts its connections, which is
// how the tests observe pooling and leaks from the outside.
type server struct {
	*httptest.Server
	opened, closed atomic.Int64
}

func serve(t testing.TB, secure bool, h http.Handler) *server {
	t.Helper()
	s := &server{Server: httptest.NewUnstartedServer(h)}
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			s.opened.Add(1)
		case http.StateClosed, http.StateHijacked:
			s.closed.Add(1)
		}
	}
	if secure {
		s.EnableHTTP2 = true
		s.StartTLS()
	} else {
		s.Start()
	}
	t.Cleanup(s.Close)
	return s
}

func (s *server) trust() *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(s.Certificate())
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func live(t testing.TB, base string, deps Dependencies, mutate func(*Config)) *Client {
	t.Helper()
	cfg := valid(base)
	cfg.RequestTimeout = 5 * time.Second
	if mutate != nil {
		mutate(&cfg)
	}
	c, err := New(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.CloseIdleConnections)
	return c
}

func get(t testing.TB, c *Client, path string) (*http.Response, error) {
	t.Helper()
	req, err := c.NewRequest(t.Context(), http.MethodGet, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.Do(t.Context(), req)
}

func eventually(t testing.TB, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// sized answers /fixed?n= with a Content-Length and /stream?n= without one.
func sized(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	payload := bytes.Repeat([]byte("a"), n)
	switch r.URL.Path {
	case "/fixed":
		w.Header().Set("Content-Length", strconv.Itoa(n))
		if r.Method != http.MethodHead {
			w.Write(payload)
		}
	case "/stream":
		w.(http.Flusher).Flush()
		w.Write(payload)
	case "/gzip":
		w.Header().Set("Content-Encoding", "gzip")
		z := gzip.NewWriter(w)
		z.Write(payload)
		z.Close()
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestNon2xxIsAResponse(t *testing.T) {
	srv := serve(t, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, _ := strconv.Atoi(r.URL.Query().Get("code"))
		w.WriteHeader(code)
		io.WriteString(w, "upstream detail")
	}))
	c := live(t, srv.URL, Dependencies{}, nil)
	for _, code := range []int{400, 404, 429, 500, 503} {
		resp, err := get(t, c, "/?code="+strconv.Itoa(code))
		if err != nil || resp.StatusCode != code {
			t.Fatal(code, resp, err)
		}
		var se *StatusError
		if err := CheckStatus(resp); !errors.As(err, &se) || se.StatusCode != code || strings.Contains(err.Error(), "detail") || strings.Contains(err.Error(), "127.0.0.1") {
			t.Fatal(err)
		}
		if data, err := io.ReadAll(resp.Body); err != nil || string(data) != "upstream detail" {
			t.Fatal(data, err)
		}
		resp.Body.Close()
	}
	if srv.opened.Load() != 1 {
		t.Fatalf("closed bodies should reuse one connection, opened %d", srv.opened.Load())
	}
}

func TestMaxResponseBytes(t *testing.T) {
	srv := serve(t, false, http.HandlerFunc(sized))
	const limit = 1024
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.MaxResponseBytes = limit })
	for _, path := range []string{"/fixed?n=0", "/fixed?n=1023", "/fixed?n=1024", "/stream?n=1023", "/stream?n=1024", "/gzip?n=1024"} {
		resp, err := get(t, c, path)
		if err != nil {
			t.Fatal(path, err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		want, _ := strconv.Atoi(path[strings.Index(path, "=")+1:])
		if err != nil || len(data) != want {
			t.Fatalf("%s: %d bytes, %v", path, len(data), err)
		}
	}
	// A declared length over the limit is refused before any body byte.
	for _, path := range []string{"/fixed?n=1025", "/fixed?n=500000"} {
		resp, err := get(t, c, path)
		if resp != nil || !errors.Is(err, ErrResponseTooLarge) {
			t.Fatal(path, resp, err)
		}
	}
	// An undeclared length fails while reading, at the limit, never short.
	// The gzip case is a small response on the wire that inflates past it.
	for _, path := range []string{"/stream?n=1025", "/stream?n=500000", "/gzip?n=1025", "/gzip?n=4000000"} {
		resp, err := get(t, c, path)
		if err != nil {
			t.Fatal(path, err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !errors.Is(err, ErrResponseTooLarge) || len(data) != limit {
			t.Fatalf("%s: %d bytes, %v", path, len(data), err)
		}
	}
	// HEAD declares the size of a body it does not send.
	req, _ := c.NewRequest(t.Context(), http.MethodHead, "/fixed?n=500000", nil)
	resp, err := c.Do(t.Context(), req)
	if err != nil || resp.ContentLength != 500000 {
		t.Fatal(resp, err)
	}
	resp.Body.Close()
	// The limit also binds code that only holds the shared client.
	raw, err := c.HTTPClient().Get(srv.URL + "/fixed?n=1025")
	if raw != nil || !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal(raw, err)
	}
	// And the client is still usable afterwards.
	resp, err = get(t, c, "/fixed?n=10")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestTimeoutsOnLoopback(t *testing.T) {
	release := make(chan struct{})
	srv := serve(t, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/body" {
			w.Write([]byte("partial"))
			w.(http.Flusher).Flush()
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release) })
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.RequestTimeout = 150 * time.Millisecond })
	var timeout net.Error
	resp, err := get(t, c, "/headers")
	if resp != nil || !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatal(resp, err)
	}
	resp, err = get(t, c, "/body")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "partial" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%q, %v", data, err)
	}
	// The transport's own header timeout fires independently and sooner.
	quick := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.Transport.ResponseHeaderTimeout = 50 * time.Millisecond })
	start := time.Now()
	resp, err = get(t, quick, "/headers")
	if resp != nil || !errors.As(err, &timeout) || !timeout.Timeout() || time.Since(start) > 2*time.Second {
		t.Fatal(resp, err, time.Since(start))
	}
	// Cancelling the caller's context aborts a body read as well.
	ctx, cancel := context.WithCancel(t.Context())
	req, _ := quick.NewRequest(ctx, http.MethodGet, "/body", nil)
	resp, err = quick.Do(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := io.ReadAll(resp.Body); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// hops records the headers each origin receives and redirects on request.
type hops struct {
	seen sync.Map // path -> http.Header
	hits atomic.Int64
}

func (h *hops) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.hits.Add(1)
	h.seen.Store(r.URL.Path, r.Header.Clone())
	q := r.URL.Query()
	switch r.URL.Path {
	case "/to":
		code, _ := strconv.Atoi(q.Get("code"))
		if code == 0 {
			code = http.StatusFound
		}
		w.Header().Set("Location", q.Get("u"))
		w.WriteHeader(code)
	case "/chain":
		n, _ := strconv.Atoi(q.Get("n"))
		if n == 0 {
			io.WriteString(w, "end")
			return
		}
		http.Redirect(w, r, "/chain?n="+strconv.Itoa(n-1), http.StatusFound)
	case "/body":
		io.Copy(w, r.Body)
	default:
		io.WriteString(w, "ok")
	}
}

func (h *hops) header(t *testing.T, path string) http.Header {
	t.Helper()
	v, ok := h.seen.Load(path)
	if !ok {
		t.Fatalf("%s was never requested", path)
	}
	return v.(http.Header)
}

func credentialed(t *testing.T, c *Client, path string) (*http.Response, error) {
	t.Helper()
	req, err := c.NewRequest(t.Context(), http.MethodGet, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Proxy-Authorization", "Basic secret")
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("X-Trace", "kept")
	return c.Do(t.Context(), req)
}

func hasCredentials(h http.Header) bool {
	return h.Get("Authorization") != "" || h.Get("Proxy-Authorization") != "" || h.Get("Cookie") != ""
}

func TestRedirectsDisabledByDefault(t *testing.T) {
	a := &hops{}
	srv := serve(t, false, a)
	c := live(t, srv.URL, Dependencies{}, nil)
	resp, err := get(t, c, "/chain?n=2")
	if err != nil || resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/chain?n=1" || a.hits.Load() != 1 {
		t.Fatal(resp, err, a.hits.Load())
	}
	resp.Body.Close()
	var se *StatusError
	if !errors.As(CheckStatus(resp), &se) || se.StatusCode != http.StatusFound {
		t.Fatal("a 3xx left unfollowed is a non-2xx status")
	}
}

func TestRedirectLimit(t *testing.T) {
	a := &hops{}
	srv := serve(t, false, a)
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 2 })
	resp, err := get(t, c, "/chain?n=2")
	if err != nil || resp.StatusCode != http.StatusOK || a.hits.Load() != 3 {
		t.Fatal(resp, err, a.hits.Load())
	}
	resp.Body.Close()
	a.hits.Store(0)
	resp, err = get(t, c, "/chain?n=3")
	// Three requests were made: the original and the two permitted hops.
	if resp != nil || !errors.Is(err, ErrRedirectNotAllowed) || a.hits.Load() != 3 {
		t.Fatal(resp, err, a.hits.Load())
	}
	one := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 1 })
	if resp, err := get(t, one, "/chain?n=2"); resp != nil || !errors.Is(err, ErrRedirectNotAllowed) {
		t.Fatal(resp, err)
	}
	// The shared client applies the same policy and, as net/http does,
	// returns the last response already closed next to the error.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/chain?n=2", nil)
	if _, err := one.HTTPClient().Do(req); !errors.Is(err, ErrRedirectNotAllowed) {
		t.Fatal(err)
	}
}

func TestRedirectCredentials(t *testing.T) {
	a, b := &hops{}, &hops{}
	srvA, srvB := serve(t, false, a), serve(t, false, b)
	c := live(t, srvA.URL, Dependencies{}, func(cfg *Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 5 })

	// Same origin, by relative and by absolute Location: credentials stay.
	for _, target := range []string{"/same-relative", srvA.URL + "/same-absolute"} {
		resp, err := credentialed(t, c, "/to?u="+target)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatal(resp, err)
		}
		resp.Body.Close()
		got := a.header(t, strings.TrimPrefix(target, srvA.URL))
		if got.Get("Authorization") != "Bearer secret" || got.Get("Cookie") != "session=secret" || got.Get("Proxy-Authorization") != "Basic secret" {
			t.Fatalf("same-origin redirect lost credentials: %v", got)
		}
	}

	// Another origin: same host, other port. net/http would keep the
	// credentials here; the redirect itself is followed because redirects
	// are not bound by the BaseURL origin rule.
	resp, err := credentialed(t, c, "/to?u="+srvB.URL+"/other")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatal(resp, err)
	}
	resp.Body.Close()
	if got := b.header(t, "/other"); hasCredentials(got) || got.Get("X-Trace") != "kept" {
		t.Fatalf("cross-origin redirect carried credentials: %v", got)
	}

	// Bounced back home by the other origin: still without credentials.
	resp, err = credentialed(t, c, "/to?u="+srvB.URL+"/to%3Fu%3D"+srvA.URL+"/bounced")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatal(resp, err)
	}
	resp.Body.Close()
	if got := a.header(t, "/bounced"); hasCredentials(got) || got.Get("X-Trace") != "kept" {
		t.Fatalf("credentials returned after leaving the origin: %v", got)
	}
}

func TestRedirectSchemeChange(t *testing.T) {
	plain, secure := &hops{}, &hops{}
	srvPlain, srvSecure := serve(t, false, plain), serve(t, true, secure)
	down := live(t, srvSecure.URL, Dependencies{TLSConfig: srvSecure.trust()}, func(cfg *Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 5 })
	resp, err := credentialed(t, down, "/to?u="+srvPlain.URL+"/downgraded")
	if resp != nil || !errors.Is(err, ErrRedirectNotAllowed) || plain.hits.Load() != 0 {
		t.Fatal(resp, err, plain.hits.Load())
	}
	// The secure server negotiated HTTP/2, so the guard was exercised on it.
	resp, err = get(t, down, "/proto")
	if err != nil || resp.ProtoMajor != 2 {
		t.Fatal(resp, err)
	}
	resp.Body.Close()
	// Upgrading is allowed, and is a change of origin like any other.
	up := live(t, srvPlain.URL, Dependencies{TLSConfig: srvSecure.trust()}, func(cfg *Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 5 })
	resp, err = credentialed(t, up, "/to?u="+srvSecure.URL+"/upgraded")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatal(resp, err)
	}
	resp.Body.Close()
	if got := secure.header(t, "/upgraded"); hasCredentials(got) {
		t.Fatalf("scheme change carried credentials: %v", got)
	}
}

func TestRedirectReplaysBody(t *testing.T) {
	a := &hops{}
	srv := serve(t, false, a)
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.FollowRedirects, cfg.MaxRedirects = true, 2 })
	req, err := c.NewRequest(t.Context(), http.MethodPost, "/to?code=307&u=/body", []byte("whole payload"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "whole payload" {
		t.Fatalf("307 replayed %q", data)
	}
}

func TestRedirectPolicyInMemory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := borrowed("http://example.com")
		cfg.RequestTimeout = 5 * time.Second
		cfg.FollowRedirects, cfg.MaxRedirects = true, 10
		hopCount := 0
		c, err := New(cfg, Dependencies{RoundTripper: rtFunc(func(r *http.Request) (*http.Response, error) {
			hopCount++
			if r.URL.Path == "/scheme" {
				return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"ftp://example.com/x"}}, Body: http.NoBody}, nil
			}
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"/next"}}, Body: http.NoBody}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		// Each hop takes two seconds; the budget is five for the whole
		// chain, not five per hop.
		req, _ := c.NewRequest(t.Context(), http.MethodGet, "/slow", nil)
		start := time.Now()
		resp, err := c.Do(t.Context(), req)
		if resp != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 5*time.Second || hopCount != 3 {
			t.Fatal(resp, err, time.Since(start), hopCount)
		}
		req, _ = c.NewRequest(t.Context(), http.MethodGet, "/scheme", nil)
		if resp, err := c.Do(t.Context(), req); resp != nil || !errors.Is(err, ErrRedirectNotAllowed) {
			t.Fatal(resp, err)
		}
	})
}

func TestBodyIsResentOnEveryAttempt(t *testing.T) {
	var bodies []string
	var mu sync.Mutex
	srv := serve(t, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(data))
		n := len(bodies)
		mu.Unlock()
		if n%2 == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	c := live(t, srv.URL, Dependencies{}, nil)
	payload := []byte(`{"order":42}`)
	attempt := func(req *http.Request) int {
		resp, err := c.Do(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	// The shape a retry policy uses: one request per attempt.
	for _, want := range []int{503, 200} {
		req, err := c.NewRequest(t.Context(), http.MethodPut, "/orders/42", payload)
		if err != nil {
			t.Fatal(err)
		}
		if got := attempt(req); got != want {
			t.Fatal(got)
		}
	}
	// Reusing one request works too, because GetBody rebuilds the body.
	req, _ := c.NewRequest(t.Context(), http.MethodPut, "/orders/42", payload)
	if attempt(req) != 503 {
		t.Fatal("fixture")
	}
	req.Body, _ = req.GetBody()
	if attempt(req) != 200 {
		t.Fatal("fixture")
	}
	if len(bodies) != 4 {
		t.Fatal(bodies)
	}
	for _, body := range bodies {
		if body != string(payload) {
			t.Fatalf("attempt received %q", body)
		}
	}
}

func TestCloseIdleConnections(t *testing.T) {
	srv := serve(t, false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") }))
	own := live(t, srv.URL, Dependencies{}, nil)
	resp, err := get(t, own, "/")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if srv.opened.Load() != 1 || srv.closed.Load() != 0 {
		t.Fatal("expected one idle connection")
	}
	own.CloseIdleConnections()
	eventually(t, "own idle connection to close", func() bool { return srv.closed.Load() == 1 })

	// A borrowed transport keeps its pool: the next call reuses the socket.
	lent := &http.Transport{}
	t.Cleanup(lent.CloseIdleConnections)
	cfg := borrowed(srv.URL)
	c, err := New(cfg, Dependencies{RoundTripper: lent})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		resp, err := get(t, c, "/")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		c.CloseIdleConnections()
	}
	if srv.opened.Load() != 2 || srv.closed.Load() != 1 {
		t.Fatalf("borrowed pool was closed: opened %d closed %d", srv.opened.Load(), srv.closed.Load())
	}
}

func TestCallerMustCloseTheBody(t *testing.T) {
	srv := serve(t, false, http.HandlerFunc(sized))
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) { cfg.Transport.MaxConnsPerHost = 1 })
	// Closed bodies: any number of calls fit in the single connection.
	for range 50 {
		resp, err := get(t, c, "/fixed?n=64")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if srv.opened.Load() != 1 {
		t.Fatalf("opened %d connections for sequential closed calls", srv.opened.Load())
	}
	// An unclosed body holds the only connection, so the next call can only
	// wait until its own context gives up. This is the leak the README
	// warns about, shown rather than assumed.
	leaked, err := get(t, c, "/fixed?n=64")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	req, _ := c.NewRequest(ctx, http.MethodGet, "/fixed?n=64", nil)
	if resp, err := c.Do(ctx, req); resp != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(resp, err)
	}
	leaked.Body.Close()
	resp, err := get(t, c, "/fixed?n=64")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// settled waits for the goroutine count to return to the baseline and then
// asks the runtime's leak profile, which finds goroutines blocked forever.
func settled(t testing.TB, baseline int) {
	t.Helper()
	eventually(t, "goroutines to finish", func() bool { return runtime.NumGoroutine() <= baseline })
	var b strings.Builder
	if err := pprof.Lookup("goroutineleak").WriteTo(&b, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "total 0") {
		t.Fatal(b.String())
	}
}

func TestConcurrentUseLeavesNothingBehind(t *testing.T) {
	baseline := runtime.NumGoroutine()
	var inFlight, peak atomic.Int64
	srv := serve(t, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := inFlight.Add(1)
		for {
			if old := peak.Load(); now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		sized(w, r)
		inFlight.Add(-1)
	}))
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) {
		cfg.MaxResponseBytes = 1024
		cfg.Transport.MaxConnsPerHost = 4
	})
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			for i := range 25 {
				path := []string{"/fixed?n=100", "/stream?n=1024", "/stream?n=5000", "/fixed?n=5000", "/missing"}[(worker+i)%5]
				req, err := c.NewRequest(t.Context(), http.MethodGet, path, nil)
				if err != nil {
					t.Error(err)
					return
				}
				resp, err := c.Do(t.Context(), req)
				if err != nil {
					if !errors.Is(err, ErrResponseTooLarge) {
						t.Error(path, err)
					}
					continue
				}
				_, err = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if err != nil && !errors.Is(err, ErrResponseTooLarge) {
					t.Error(path, err)
				}
			}
		})
	}
	wg.Wait()
	if peak.Load() > 4 {
		t.Fatalf("MaxConnsPerHost exceeded: %d requests in flight", peak.Load())
	}
	c.CloseIdleConnections()
	srv.Close()
	if srv.opened.Load() != srv.closed.Load() {
		t.Fatalf("connections left open: opened %d closed %d", srv.opened.Load(), srv.closed.Load())
	}
	settled(t, baseline)
}

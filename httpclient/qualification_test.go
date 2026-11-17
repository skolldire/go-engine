package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStressHostileUpstream runs many concurrent calls against a loopback
// server that is slow, cuts responses and sends oversized bodies, and then
// requires every connection and goroutine to be gone.
func TestStressHostileUpstream(t *testing.T) {
	if os.Getenv("HTTPCLIENT_STRESS") != "1" {
		t.Skip("separate stress campaign")
	}
	baseline := runtime.NumGoroutine()
	release := make(chan struct{})
	srv := serve(t, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow-headers":
			select {
			case <-r.Context().Done():
			case <-release:
			}
		case "/slow-body":
			w.Write([]byte("partial"))
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-release:
			}
		case "/cut":
			// Promise more than is sent, then drop the connection.
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 512\r\n\r\ntruncated")
			buf.Flush()
			conn.Close()
		default:
			sized(w, r)
		}
	}))
	c := live(t, srv.URL, Dependencies{}, func(cfg *Config) {
		cfg.RequestTimeout = 60 * time.Millisecond
		cfg.MaxResponseBytes = 4096
		cfg.Transport.MaxConnsPerHost = 8
	})
	paths := []string{"/fixed?n=4096", "/stream?n=4096", "/fixed?n=100000", "/stream?n=100000", "/gzip?n=1000000", "/cut", "/slow-headers", "/slow-body", "/missing"}
	var ok, tooLarge, timedOut, cut atomic.Int64
	var wg sync.WaitGroup
	for worker := range 64 {
		wg.Go(func() {
			for i := range 40 {
				path := paths[(worker+i)%len(paths)]
				ctx := t.Context()
				cancel := context.CancelFunc(func() {})
				if (worker+i)%7 == 0 {
					// Some callers give up on their own, mid-flight.
					ctx, cancel = context.WithTimeout(ctx, time.Duration(i%5)*10*time.Millisecond)
				}
				req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
				if err != nil {
					t.Error(err)
					cancel()
					return
				}
				resp, err := c.Do(ctx, req)
				if err == nil {
					_, err = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
				cancel()
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, ErrResponseTooLarge):
					tooLarge.Add(1)
				case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
					timedOut.Add(1)
				case path == "/cut":
					cut.Add(1)
				default:
					t.Errorf("%s: unexpected error %v", path, err)
				}
			}
		})
	}
	wg.Wait()
	if ok.Load() == 0 || tooLarge.Load() == 0 || timedOut.Load() == 0 || cut.Load() == 0 {
		t.Fatalf("a failure mode was not exercised: ok=%d large=%d timeout=%d cut=%d", ok.Load(), tooLarge.Load(), timedOut.Load(), cut.Load())
	}
	t.Logf("ok=%d too-large=%d timed-out=%d cut=%d", ok.Load(), tooLarge.Load(), timedOut.Load(), cut.Load())
	close(release)
	c.CloseIdleConnections()
	srv.Close()
	if srv.opened.Load() != srv.closed.Load() {
		t.Fatalf("connections left open: opened %d closed %d", srv.opened.Load(), srv.closed.Load())
	}
	settled(t, baseline)
}

// FuzzOrigin holds the property the origin rule exists for: whatever string
// a caller turns into a request, nothing leaves for another origin, and a
// relative reference never escapes the base path.
func FuzzOrigin(f *testing.F) {
	for _, seed := range []string{
		"/users", "users?x=1", "", "/../admin", "/%2e%2e/admin", "//evil.example/x", "https://api.example.com:8443/v1/x",
		"https://evil.example/", "https://api.example.com:8443@evil.example/", "https://user:pw@api.example.com:8443/", "http://api.example.com:8443/",
		"https://api.example.com:443/", "https://API.EXAMPLE.COM:8443/..%2f", "https:api.example.com", "/a%2Fb", "\\\\evil.example\\x", "/\t/x", "https://api.example.com:8443\\@evil.example/",
		"https://[::1]:8443/", "HTTPS://api.example.com:08443/", "https://api.example.com.:8443/", "?q=1", "#frag", "/a b", "javascript:alert(1)",
	} {
		f.Add(seed)
	}
	rec := &recorder{}
	c, err := New(borrowed("https://api.example.com:8443/v1"), Dependencies{RoundTripper: rec})
	if err != nil {
		f.Fatal(err)
	}
	onOrigin := func(t *testing.T, u *url.URL) {
		if u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "api.example.com") || u.User != nil || u.Opaque != "" {
			t.Fatalf("request left the origin: %q", u)
		}
		// The port is compared as a number: "08443" is the same socket.
		if got, ok := originOf(u); !ok || got.port != 8443 {
			t.Fatalf("request left the origin port: %q", u)
		}
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 512 {
			return
		}
		ctx := context.Background()
		if req, err := c.NewRequest(ctx, http.MethodGet, raw, nil); err == nil {
			onOrigin(t, req.URL)
			// NewRequest takes paths only, so the result is always under
			// the configured prefix and free of dot segments.
			if p := req.URL.Path; p != "/v1" && !strings.HasPrefix(p, "/v1/") {
				t.Fatalf("%q escaped the base path: %q", raw, p)
			}
			for _, segment := range strings.Split(req.URL.Path, "/") {
				if segment == "." || segment == ".." {
					t.Fatalf("%q kept a dot segment: %q", raw, req.URL.Path)
				}
			}
			// What NewRequest builds, Do must accept unchanged.
			if again, err := c.resolve(req.URL); err != nil || again != req.URL {
				t.Fatalf("%q: built but refused: %v", raw, err)
			}
		} else if !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("%q: unclassified error %v", raw, err)
		}
		u, err := url.Parse(raw)
		if err != nil {
			return
		}
		// Do's decision. The guard is driven directly below rather than
		// through http.Client: the fuzzer instruments net/http as well, and
		// its coverage noise starves the search of executions.
		if target, err := c.resolve(u); err == nil {
			onOrigin(t, target)
		} else if !errors.Is(err, ErrOriginNotAllowed) && !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("%q: unclassified error %v", raw, err)
		}
		// The shared client's decision, for code that holds HTTPClient().
		before := rec.calls.Load()
		resp, err := c.http.Transport.RoundTrip(&http.Request{Method: http.MethodGet, URL: u, Header: http.Header{}})
		if err != nil {
			if rec.calls.Load() != before {
				t.Fatalf("%q: refused after reaching the transport: %v", raw, err)
			}
			if !errors.Is(err, ErrOriginNotAllowed) && !errors.Is(err, ErrInvalidCall) {
				t.Fatalf("%q: unclassified error %v", raw, err)
			}
			return
		}
		resp.Body.Close()
		onOrigin(t, rec.last.Load().URL)
	})
}

// FuzzRedirect drives the redirect policy with arbitrary Location values.
func FuzzRedirect(f *testing.F) {
	for _, seed := range []string{
		"/next", "https://api.example.com/next", "http://api.example.com/next", "https://sub.api.example.com/", "https://api.example.com:8443/",
		"https://evil.example/", "//evil.example/", "ftp://api.example.com/", "https://user:pw@api.example.com/", "HTTPS://API.EXAMPLE.COM/x", "", "?q", "mailto:a@b",
	} {
		f.Add(seed, uint8(0), false)
		f.Add(seed, uint8(3), true)
	}
	const limit = 4
	policy := redirectPolicy(true, limit)
	first, _ := url.Parse("https://api.example.com/start")
	elsewhere, _ := url.Parse("https://other.example/hop")
	home := origin{"https", "api.example.com", 443}
	f.Fuzz(func(t *testing.T, location string, hops uint8, detour bool) {
		if len(location) > 512 {
			return
		}
		// net/http resolves Location against the previous URL the same way.
		target, err := first.Parse(location)
		if err != nil {
			return
		}
		via := []*http.Request{{URL: first}}
		for i := range int(hops % 8) {
			hop := first
			if detour && i == 0 {
				hop = elsewhere
			}
			via = append(via, &http.Request{URL: hop})
		}
		req := &http.Request{URL: target, Header: http.Header{
			"Authorization": {"Bearer secret"}, "Proxy-Authorization": {"Basic secret"}, "Cookie": {"session=secret"}, "X-Trace": {"kept"},
		}}
		if err := policy(req, via); err != nil {
			if !errors.Is(err, ErrRedirectNotAllowed) {
				t.Fatalf("unclassified refusal: %v", err)
			}
			return
		}
		got, ok := originOf(target)
		switch {
		case len(via) > limit:
			t.Fatalf("followed hop %d past the limit", len(via))
		case !ok:
			t.Fatalf("followed a non-HTTP target: %q", target)
		case target.Scheme == "http":
			t.Fatalf("followed https to http: %q", target)
		}
		left := got != home || (detour && len(via) > 1)
		if left == hasCredentials(req.Header) || req.Header.Get("X-Trace") != "kept" {
			t.Fatalf("%q (left=%v): headers %v", target, left, req.Header)
		}
	})
	if !errors.Is(redirectPolicy(false, 0)(&http.Request{URL: first}, []*http.Request{{URL: first}}), http.ErrUseLastResponse) {
		f.Fatal("redirects must be off unless enabled")
	}
}

// The pair below measures what the guard adds over the same transport used
// bare: one loopback GET with a small body, read to the end and closed.
func benchServer(b *testing.B) (*server, *Client) {
	srv := serve(b, false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"ok":true}`) }))
	return srv, live(b, srv.URL, Dependencies{}, nil)
}

func BenchmarkDo(b *testing.B) {
	_, c := benchServer(b)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		req, err := c.NewRequest(ctx, http.MethodGet, "/item", nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := c.Do(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func BenchmarkBareHTTPClient(b *testing.B) {
	srv, c := benchServer(b)
	bare := &http.Client{Transport: c.owned}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/item", nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := bare.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

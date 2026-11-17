package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type key struct{}

// recorder is a borrowed RoundTripper that answers 204 and remembers what it
// was asked, so origin tests can prove that nothing reached the network.
type recorder struct {
	calls atomic.Int64
	last  atomic.Pointer[http.Request]
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	r.last.Store(req)
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if req.Body != nil {
		io.Copy(io.Discard, req.Body)
		req.Body.Close()
	}
	return &http.Response{StatusCode: 204, Body: http.NoBody, Request: req, Header: http.Header{}}, nil
}

func memory(t *testing.T, base string) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{}
	c, err := New(borrowed(base), Dependencies{RoundTripper: rec})
	if err != nil {
		t.Fatal(err)
	}
	return c, rec
}

type closeSpy struct {
	io.Reader
	closed atomic.Bool
}

func (c *closeSpy) Close() error { c.closed.Store(true); return nil }

func TestNewRequestJoinsUnderBase(t *testing.T) {
	c, _ := memory(t, "https://API.example.com/v1/")
	for path, want := range map[string]string{
		"":                  "https://api.example.com/v1",
		"/":                 "https://api.example.com/v1/",
		"/users":            "https://api.example.com/v1/users",
		"users/7":           "https://api.example.com/v1/users/7",
		"/users?page=2&q=a": "https://api.example.com/v1/users?page=2&q=a",
		"/a%2Fb/c":          "https://api.example.com/v1/a%2Fb/c",
		"a%2Fb":             "https://api.example.com/v1/a%2Fb",
		"/files/a b":        "https://api.example.com/v1/files/a%20b",
		"/x#fragment":       "https://api.example.com/v1/x",
	} {
		req, err := c.NewRequest(t.Context(), http.MethodGet, path, nil)
		if err != nil || req.URL.String() != want {
			t.Fatalf("%q -> %v, %v; want %s", path, req, err, want)
		}
	}
	root, _ := memory(t, "http://example.com")
	req, err := root.NewRequest(t.Context(), http.MethodGet, "/users", nil)
	if err != nil || req.URL.String() != "http://example.com/users" {
		t.Fatal(req, err)
	}
}

func TestNewRequestRejections(t *testing.T) {
	c, _ := memory(t, "https://api.example.com/v1")
	var none context.Context // the nil context is the case under test
	if _, err := c.NewRequest(none, http.MethodGet, "/", nil); !errors.Is(err, ErrNilContext) {
		t.Fatal(err)
	}
	for _, path := range []string{
		"https://api.example.com/v1/users", "http://evil.example/x", "//evil.example/x", "https://user:pw@api.example.com/x", "mailto:a@b.c", "a:b",
	} {
		_, err := c.NewRequest(t.Context(), http.MethodGet, path, nil)
		if !errors.Is(err, ErrInvalidCall) || !errors.Is(err, ErrOriginNotAllowed) {
			t.Fatalf("%q: %v", path, err)
		}
	}
	for _, path := range []string{"/../admin", "/a/../../b", "..", "/a/./b", "/%2e%2e/admin", "/a/%2E%2E", "/%zz", "/a\x7f"} {
		_, err := c.NewRequest(t.Context(), http.MethodGet, path, nil)
		if !errors.Is(err, ErrInvalidCall) || errors.Is(err, ErrOriginNotAllowed) {
			t.Fatalf("%q: %v", path, err)
		}
	}
	for _, method := range []string{"", "GE T", "GET\r\nX: y"} {
		if _, err := c.NewRequest(t.Context(), method, "/", nil); !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("%q: %v", method, err)
		}
	}
}

func TestNewRequestBodyCanBeReplayed(t *testing.T) {
	c, _ := memory(t, "http://example.com")
	req, err := c.NewRequest(t.Context(), http.MethodPost, "/orders", []byte(`{"id":1}`))
	if err != nil || req.ContentLength != 8 || req.GetBody == nil {
		t.Fatal(req, err)
	}
	for range 3 {
		body, err := req.GetBody()
		if err != nil {
			t.Fatal(err)
		}
		if data, _ := io.ReadAll(body); string(data) != `{"id":1}` {
			t.Fatalf("replayed %q", data)
		}
	}
	// No body and an empty body are both sent as Content-Length: 0.
	for _, body := range [][]byte{nil, {}} {
		req, err := c.NewRequest(t.Context(), http.MethodPost, "/orders", body)
		if err != nil || req.ContentLength != 0 {
			t.Fatal(req, err)
		}
	}
	if len(req.Header) != 0 {
		t.Fatalf("implicit headers: %v", req.Header)
	}
}

func TestDoInvalidCalls(t *testing.T) {
	c, rec := memory(t, "http://example.com")
	spy := &closeSpy{Reader: strings.NewReader("x")}
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/", spy)
	var none context.Context // the nil context is the case under test
	if _, err := c.Do(none, req); !errors.Is(err, ErrNilContext) || !spy.closed.Load() {
		t.Fatal(err)
	}
	if _, err := c.Do(t.Context(), nil); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	if _, err := c.Do(t.Context(), &http.Request{}); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	if rec.calls.Load() != 0 {
		t.Fatal("invalid call reached the transport")
	}
}

func TestOriginRule(t *testing.T) {
	c, rec := memory(t, "https://api.example.com:8443/v1")
	allowed := map[string]string{
		"/users":                              "https://api.example.com:8443/v1/users",
		"users?x=1":                           "https://api.example.com:8443/v1/users?x=1",
		"https://api.example.com:8443/other":  "https://api.example.com:8443/other",
		"HTTPS://API.EXAMPLE.COM:8443/shout":  "https://API.EXAMPLE.COM:8443/shout",
		"https://api.example.com:8443":        "https://api.example.com:8443",
		"https://api.example.com:8443/../etc": "https://api.example.com:8443/../etc",
	}
	for raw, want := range allowed {
		before := rec.calls.Load()
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.Do(t.Context(), req)
		if err != nil || rec.calls.Load() != before+1 || rec.last.Load().URL.String() != want {
			t.Fatalf("%q: %v, sent %v", raw, err, rec.last.Load().URL)
		}
		resp.Body.Close()
	}
	denied := []string{
		"https://evil.example:8443/v1/users",       // other host
		"https://sub.api.example.com:8443/v1",      // subdomain
		"https://api.example.com.:8443/v1",         // trailing dot is another spelling: fails closed
		"https://api.example.com/v1",               // default port instead of 8443
		"https://api.example.com:9443/v1",          // other port
		"http://api.example.com:8443/v1",           // other scheme
		"https://user:pw@api.example.com:8443/v1",  // userinfo on the right host
		"https://api.example.com:8443@evil.test/x", // userinfo hiding the real host
		"//evil.example/x",                         // scheme-relative
		"ftp://api.example.com:8443/v1",
		"https:api.example.com:8443",
	}
	for _, raw := range denied {
		before := rec.calls.Load()
		spy := &closeSpy{Reader: strings.NewReader("x")}
		req, err := http.NewRequest(http.MethodPost, raw, spy)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.Do(t.Context(), req)
		// The sentinel is returned bare, with no URL attached to it.
		if err != ErrOriginNotAllowed || resp != nil || rec.calls.Load() != before || !spy.closed.Load() {
			t.Fatalf("%q: resp=%v err=%v calls=%d", raw, resp, err, rec.calls.Load()-before)
		}
		// The shared client refuses the same URL before the transport too.
		req, _ = http.NewRequest(http.MethodGet, raw, nil)
		if _, err := c.HTTPClient().Do(req); !errors.Is(err, ErrOriginNotAllowed) || rec.calls.Load() != before {
			t.Fatalf("HTTPClient %q: %v", raw, err)
		}
	}
	// Default ports are the same origin whether written or not.
	plain, plainRec := memory(t, "http://example.com")
	for _, raw := range []string{"http://example.com:80/x", "http://EXAMPLE.com/x"} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		resp, err := plain.Do(t.Context(), req)
		if err != nil {
			t.Fatal(raw, err)
		}
		resp.Body.Close()
	}
	if plainRec.calls.Load() != 2 {
		t.Fatal("default port not treated as the same origin")
	}
	for _, raw := range []string{"/../admin", "/a/%2e%2e/b"} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if _, err := plain.Do(t.Context(), req); !errors.Is(err, ErrInvalidCall) {
			t.Fatal(raw, err)
		}
	}
}

func TestHTTPClientNeedsAbsoluteURL(t *testing.T) {
	c, rec := memory(t, "http://example.com")
	req, _ := http.NewRequest(http.MethodGet, "/relative", nil)
	if _, err := c.HTTPClient().Do(req); !errors.Is(err, ErrInvalidCall) || rec.calls.Load() != 0 {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, "http://example.com/absolute", nil)
	resp, err := c.HTTPClient().Do(req)
	if err != nil || rec.calls.Load() != 1 {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestDoUsesItsOwnContext(t *testing.T) {
	c, rec := memory(t, "http://example.com")
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := c.NewRequest(dead, http.MethodGet, "/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(context.WithValue(t.Context(), key{}, "live"), req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := rec.last.Load().Context().Value(key{}); got != "live" {
		t.Fatalf("transport saw context value %v", got)
	}
	// The caller's request is left as it was given.
	if req.Context() != dead || req.URL.String() != "http://example.com/x" {
		t.Fatal("caller request was modified")
	}
	if _, err := c.Do(dead, req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestNoImplicitHeaders(t *testing.T) {
	c, rec := memory(t, "http://example.com")
	req, _ := c.NewRequest(t.Context(), http.MethodPost, "/x", []byte("body"))
	resp, err := c.Do(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if sent := rec.last.Load().Header; len(sent) != 0 {
		t.Fatalf("headers added before the transport: %v", sent)
	}
}

func TestCheckStatus(t *testing.T) {
	if err := CheckStatus(nil); !errors.Is(err, ErrInvalidCall) {
		t.Fatal(err)
	}
	for _, code := range []int{200, 201, 204, 299} {
		if err := CheckStatus(&http.Response{StatusCode: code}); err != nil {
			t.Fatal(code, err)
		}
	}
	for _, code := range []int{100, 199, 300, 304, 400, 404, 429, 500, 503} {
		body := &closeSpy{Reader: strings.NewReader("secret-body token=abc")}
		req, _ := http.NewRequest(http.MethodGet, "http://example.com/private?token=abc", nil)
		resp := &http.Response{StatusCode: code, Status: "599 secret-status", Body: body, Request: req}
		err := CheckStatus(resp)
		var se *StatusError
		if !errors.As(err, &se) || se.StatusCode != code || se.Response != resp {
			t.Fatal(code, err)
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "httpclient: unexpected status ") || strings.Contains(msg, "secret") || strings.Contains(msg, "example.com") || strings.Contains(msg, "token") {
			t.Fatalf("message leaks: %q", msg)
		}
		// The body is still whole and still open: ownership did not move.
		if body.closed.Load() {
			t.Fatal("CheckStatus closed the body")
		}
		if data, _ := io.ReadAll(se.Response.Body); string(data) != "secret-body token=abc" {
			t.Fatalf("CheckStatus consumed the body: %q", data)
		}
	}
	if msg := CheckStatus(&http.Response{StatusCode: 503}).Error(); msg != "httpclient: unexpected status 503" {
		t.Fatal(msg)
	}
}

// stalled is a body that yields a prefix and then blocks until its request
// context ends, like a peer that stops sending.
type stalled struct {
	ctx    context.Context
	prefix []byte
	end    error
}

func (s *stalled) Read(p []byte) (int, error) {
	if len(s.prefix) > 0 {
		n := copy(p, s.prefix)
		s.prefix = s.prefix[n:]
		return n, nil
	}
	if s.end != nil {
		return 0, s.end
	}
	<-s.ctx.Done()
	return 0, s.ctx.Err()
}
func (s *stalled) Close() error { return nil }

func TestRequestTimeoutWhileWaitingForHeaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := borrowed("http://example.com")
		cfg.RequestTimeout = 3 * time.Second
		c, err := New(cfg, Dependencies{RoundTripper: rtFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})})
		if err != nil {
			t.Fatal(err)
		}
		req, _ := c.NewRequest(t.Context(), http.MethodGet, "/slow", nil)
		start := time.Now()
		resp, err := c.Do(t.Context(), req)
		var timeout net.Error
		if resp != nil || !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatal(resp, err)
		}
		if elapsed := time.Since(start); elapsed != 3*time.Second {
			t.Fatalf("timed out after %v", elapsed)
		}
	})
}

func TestRequestTimeoutCoversBodyAndIsReleasedOnClose(t *testing.T) {
	run := func(t *testing.T, end error) (*http.Response, context.Context, *Client) {
		var seen context.Context
		cfg := borrowed("http://example.com")
		cfg.RequestTimeout = 4 * time.Second
		c, err := New(cfg, Dependencies{RoundTripper: rtFunc(func(r *http.Request) (*http.Response, error) {
			seen = r.Context()
			return &http.Response{StatusCode: 200, ContentLength: -1, Request: r, Body: &stalled{ctx: r.Context(), prefix: []byte("partial"), end: end}}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		req, _ := c.NewRequest(t.Context(), http.MethodGet, "/stream", nil)
		resp, err := c.Do(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		// Headers are back and the budget is still running, not cancelled.
		synctest.Wait()
		if seen.Err() != nil {
			t.Fatal("context was cancelled when headers were returned")
		}
		return resp, seen, c
	}
	t.Run("expires during the body read", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			resp, seen, _ := run(t, nil)
			time.Sleep(time.Second)
			buf := make([]byte, 16)
			if n, err := resp.Body.Read(buf); err != nil || string(buf[:n]) != "partial" || seen.Err() != nil {
				t.Fatal(n, err)
			}
			_, err := resp.Body.Read(buf)
			var timeout net.Error
			if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &timeout) || !timeout.Timeout() {
				t.Fatal(err)
			}
			if elapsed := time.Since(start); elapsed != 4*time.Second {
				t.Fatalf("body timed out after %v", elapsed)
			}
			resp.Body.Close()
		})
	})
	t.Run("released by Close", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			resp, seen, _ := run(t, nil)
			resp.Body.Close()
			synctest.Wait()
			if !errors.Is(seen.Err(), context.Canceled) {
				t.Fatalf("Close did not release the request context: %v", seen.Err())
			}
		})
	})
	t.Run("released at EOF", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			resp, seen, _ := run(t, io.EOF)
			if data, err := io.ReadAll(resp.Body); err != nil || string(data) != "partial" {
				t.Fatal(data, err)
			}
			synctest.Wait()
			if !errors.Is(seen.Err(), context.Canceled) {
				t.Fatalf("EOF did not release the request context: %v", seen.Err())
			}
			resp.Body.Close()
		})
	})
	t.Run("caller deadline wins when shorter", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cfg := borrowed("http://example.com")
			c, err := New(cfg, Dependencies{RoundTripper: rtFunc(func(r *http.Request) (*http.Response, error) {
				<-r.Context().Done()
				return nil, context.Cause(r.Context())
			})})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			req, _ := c.NewRequest(ctx, http.MethodGet, "/slow", nil)
			start := time.Now()
			if _, err := c.Do(ctx, req); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != time.Second {
				t.Fatal(err, time.Since(start))
			}
		})
	})
}

func TestBorrowedTransportIsNeverClosed(t *testing.T) {
	closed := 0
	lent := &closable{closed: &closed}
	c, err := New(borrowed("http://example.com"), Dependencies{RoundTripper: lent})
	if err != nil {
		t.Fatal(err)
	}
	c.CloseIdleConnections()
	if closed != 0 || c.owned != nil {
		t.Fatal("borrowed transport was touched")
	}
}

type closable struct{ closed *int }

func (c *closable) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unused")
}
func (c *closable) CloseIdleConnections() { *c.closed++ }

func TestLimitedBodyUnit(t *testing.T) {
	for _, tc := range []struct {
		size, max int
		ok        bool
	}{{0, 1, true}, {1, 1, true}, {2, 1, false}, {1023, 1024, true}, {1024, 1024, true}, {1025, 1024, false}, {5000, 1024, false}} {
		for _, chunk := range []int{1, 7, 1024, 4096} {
			b := &limitedBody{body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), tc.size))), remaining: int64(tc.max), ctx: t.Context(), cancel: func() {}}
			var got []byte
			var err error
			buf := make([]byte, chunk)
			for err == nil {
				var n int
				n, err = b.Read(buf)
				got = append(got, buf[:n]...)
			}
			if tc.ok && (err != io.EOF || len(got) != tc.size) {
				t.Fatalf("%+v chunk %d: %d bytes, %v", tc, chunk, len(got), err)
			}
			if !tc.ok {
				// Never more than the limit is handed over, and the error
				// is sticky rather than turning into EOF on the next read.
				if err != ErrResponseTooLarge || len(got) != tc.max {
					t.Fatalf("%+v chunk %d: %d bytes, %v", tc, chunk, len(got), err)
				}
				if n, again := b.Read(buf); n != 0 || again != ErrResponseTooLarge {
					t.Fatal(n, again)
				}
			}
			if n, zero := b.Read(nil); n != 0 || (tc.ok && zero != nil) {
				t.Fatal(n, zero)
			}
		}
	}
}

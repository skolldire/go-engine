package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Client is safe for concurrent use; nothing in it changes after New.
type Client struct {
	base   *url.URL
	origin origin
	http   *http.Client
	// owned is nil when the transport is borrowed, so that closing idle
	// connections can never reach into a pool another component shares.
	owned *http.Transport
}

// New validates and assembles; it opens no connection, reads no file and
// starts no goroutine.
func New(cfg Config, deps Dependencies) (*Client, error) {
	borrowed := deps.RoundTripper != nil
	if borrowed {
		if isNil(deps.RoundTripper) {
			return nil, &ConfigError{"RoundTripper", "is a typed nil"}
		}
		// A borrowed transport is used exactly as given. Accepting pool or
		// TLS settings next to it would promise limits this module cannot
		// apply without modifying something it does not own. Checked before
		// Validate so a partial Transport is reported as the mix it is.
		if cfg.Transport != (TransportConfig{}) {
			return nil, &ConfigError{"Transport", "must be empty with a borrowed RoundTripper"}
		}
		if deps.TLSConfig != nil {
			return nil, &ConfigError{"TLSConfig", "must be nil with a borrowed RoundTripper"}
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	base, o, err := parseBase(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	c := &Client{base: base, origin: o}
	rt := deps.RoundTripper
	if !borrowed {
		if err := cfg.Transport.validate(); err != nil {
			return nil, err
		}
		c.owned = newTransport(cfg.Transport, deps)
		rt = c.owned
	}
	if deps.WrapTransport != nil {
		if rt = deps.WrapTransport(rt); isNil(rt) {
			return nil, &ConfigError{"WrapTransport", "returned nil"}
		}
	}
	c.http = &http.Client{
		// The guard is outermost so that a wrapper cannot skip it, and so
		// that code holding only HTTPClient() is bound by the same rules.
		Transport:     &guard{next: rt, origin: o, max: cfg.MaxResponseBytes, timeout: cfg.RequestTimeout},
		CheckRedirect: redirectPolicy(cfg.FollowRedirects, cfg.MaxRedirects),
	}
	return c, nil
}

func newTransport(cfg TransportConfig, deps Dependencies) *http.Transport {
	t := &http.Transport{
		DialContext:       (&net.Dialer{Timeout: cfg.ConnectTimeout}).DialContext,
		ForceAttemptHTTP2: true,
		// The zero value means no handshake limit at all; a peer that
		// accepts TCP and then stalls would otherwise be bounded only by
		// the whole-request timeout.
		TLSHandshakeTimeout:   cfg.ConnectTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		IdleConnTimeout:       cfg.IdleTimeout,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
	}
	if proxy, _ := cfg.proxy(); proxy != nil {
		t.Proxy = http.ProxyURL(proxy)
	}
	if deps.TLSConfig != nil {
		t.TLSClientConfig = deps.TLSConfig.Clone()
	}
	return t
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	switch r := reflect.ValueOf(v); r.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return r.IsNil()
	}
	return false
}

// NewRequest builds a request for a path under BaseURL. The body is kept as a
// byte slice so that GetBody can replay it: a retry or a 307/308 redirect
// then sends the whole payload again instead of a drained reader.
//
// body is not copied and must not be modified while the request is in use.
func (c *Client) NewRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if method == "" {
		return nil, fmt.Errorf("%w: empty method", ErrInvalidCall)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("%w: unparsable path", ErrInvalidCall)
	}
	if ref.Scheme != "" || ref.Host != "" || ref.User != nil || ref.Opaque != "" {
		// Both sentinels match: the call is malformed, and a caller that
		// watches for attempts to leave the origin sees this one too.
		return nil, fmt.Errorf("%w: path must be relative: %w", ErrInvalidCall, ErrOriginNotAllowed)
	}
	target, err := c.join(ref)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid method", ErrInvalidCall)
	}
	return req, nil
}

// Do sends req under ctx, which replaces the request's own context. A status
// outside 2xx is a response, not an error. The caller always closes the body.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if ctx == nil {
		closeRequest(req)
		return nil, ErrNilContext
	}
	if req == nil || req.URL == nil {
		closeRequest(req)
		return nil, ErrInvalidCall
	}
	target, err := c.resolve(req.URL)
	if err != nil {
		closeRequest(req)
		return nil, err
	}
	out := req.WithContext(ctx)
	out.URL = target
	resp, err := c.http.Do(out)
	if err != nil {
		// net/http hands back the last response, already closed, when a
		// redirect is refused. Returning it next to an error would invite
		// reading a closed body.
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	return resp, nil
}

// HTTPClient returns the one shared *http.Client. It carries every protection
// of Do except relative URL resolution, and must not be modified.
func (c *Client) HTTPClient() *http.Client { return c.http }

// CloseIdleConnections closes idle connections of the module's own transport
// and does nothing to a borrowed one.
func (c *Client) CloseIdleConnections() {
	if c.owned != nil {
		c.owned.CloseIdleConnections()
	}
}

func closeRequest(req *http.Request) {
	if req != nil && req.Body != nil {
		_ = req.Body.Close()
	}
}

// resolve returns the URL to call: a relative reference joined under BaseURL,
// or an absolute URL that is already on the allowed origin.
func (c *Client) resolve(u *url.URL) (*url.URL, error) {
	if u.Scheme == "" && u.Host == "" && u.Opaque == "" && u.User == nil {
		return c.join(u)
	}
	if err := c.origin.allows(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (o origin) allows(u *url.URL) error {
	// Userinfo is refused even on the right host: it becomes a Basic
	// Authorization header and makes "http://base@evil" style confusion
	// possible in whatever produced the string.
	if got, ok := originOf(u); !ok || got != o || u.Opaque != "" || u.User != nil {
		return ErrOriginNotAllowed
	}
	return nil
}

// join appends a relative reference to the base path. It concatenates rather
// than applying RFC 3986 resolution, where "/x" would discard the base prefix.
func (c *Client) join(ref *url.URL) (*url.URL, error) {
	for _, segment := range strings.Split(ref.Path, "/") {
		// Checked on the decoded path, so %2e%2e is caught as well: a
		// dot segment could climb out of the configured prefix.
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("%w: dot segment in path", ErrInvalidCall)
		}
	}
	out := *c.base
	path, raw := ref.Path, ref.RawPath
	if path != "" && path[0] != '/' {
		path = "/" + path
		if raw != "" {
			raw = "/" + raw
		}
	}
	out.Path = c.base.Path + path
	if raw != "" {
		out.RawPath = c.base.EscapedPath() + raw
	}
	out.RawQuery, out.ForceQuery = ref.RawQuery, ref.ForceQuery
	return &out, nil
}

// StatusError reports a status outside 2xx. Response is the caller's own
// response, untouched: its body is still open and still the caller's to close.
type StatusError struct {
	StatusCode int
	Response   *http.Response
}

// Error names the status code only. A response body or a URL can carry
// personal data or secrets, and error strings end up in logs.
func (e *StatusError) Error() string {
	return "httpclient: unexpected status " + strconv.Itoa(e.StatusCode)
}

// CheckStatus returns nil for 2xx and a *StatusError otherwise. It neither
// reads nor closes the body.
func CheckStatus(resp *http.Response) error {
	if resp == nil {
		return ErrInvalidCall
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	return &StatusError{StatusCode: resp.StatusCode, Response: resp}
}

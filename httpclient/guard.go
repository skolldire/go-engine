package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// guard is the RoundTripper every request crosses, whether it came through
// Do or through HTTPClient() in someone else's hands.
type guard struct {
	next    http.RoundTripper
	origin  origin
	max     int64
	timeout time.Duration
}

// deadlineKey carries the deadline of a logical request across its redirect
// hops, so that following redirects cannot extend the budget.
type deadlineKey struct{}

func (g *guard) RoundTrip(req *http.Request) (*http.Response, error) {
	deadline := time.Now().Add(g.timeout)
	if req.Response == nil {
		if err := g.admit(req); err != nil {
			closeRequest(req)
			return nil, err
		}
	} else if prev := req.Response.Request; prev != nil {
		// net/http sets Response only on the requests it creates to follow
		// a redirect. Those were already judged by the redirect policy, and
		// the caller opted into leaving the origin by enabling redirects.
		// They inherit the deadline fixed by the first hop.
		if first, ok := prev.Context().Value(deadlineKey{}).(time.Time); ok {
			deadline = first
		}
	}
	// http.Client.Timeout is not used: it keeps its timer until Close even
	// after EOF, and it needs a goroutine per request for a RoundTripper it
	// does not recognize. This context is released as soon as the body ends.
	ctx, cancel := context.WithDeadline(context.WithValue(req.Context(), deadlineKey{}, deadline), deadline)
	out := req.WithContext(ctx)
	resp, err := g.next.RoundTrip(out)
	if err != nil || resp == nil {
		cancel()
		return resp, err
	}
	// The next hop reads its deadline from here, whatever the wrapped
	// transport chose to report.
	resp.Request = out
	if resp.Body == nil {
		resp.Body = http.NoBody
	}
	// A HEAD response declares the size of a body it does not carry.
	if req.Method != http.MethodHead && resp.ContentLength > g.max {
		_ = resp.Body.Close()
		cancel()
		return nil, ErrResponseTooLarge
	}
	limited := &limitedBody{body: resp.Body, remaining: g.max, ctx: ctx, cancel: cancel}
	// A 101 or CONNECT response body is also the write side of the
	// connection; hiding that would break protocol upgrades.
	if w, ok := resp.Body.(io.Writer); ok {
		resp.Body = &limitedReadWriter{limitedBody: limited, w: w}
	} else {
		resp.Body = limited
	}
	return resp, nil
}

func (g *guard) admit(req *http.Request) error {
	if req.URL == nil {
		return ErrInvalidCall
	}
	if req.URL.Scheme == "" && req.URL.Host == "" {
		return fmt.Errorf("%w: HTTPClient needs an absolute URL; Do resolves relative ones", ErrInvalidCall)
	}
	return g.origin.allows(req.URL)
}

// limitedBody fails instead of truncating: a silently shortened body is
// indistinguishable from a complete one to a JSON decoder reading a prefix.
// It also owns the request deadline, which must outlive the headers.
type limitedBody struct {
	body      io.ReadCloser
	remaining int64
	err       error
	ctx       context.Context
	cancel    context.CancelFunc
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	// One byte past the limit is requested so that a body of exactly the
	// limit is told apart from a longer one without a second read.
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.body.Read(p)
	if int64(n) > b.remaining {
		n = int(b.remaining)
		b.remaining = 0
		b.err = ErrResponseTooLarge
		return n, b.err
	}
	b.remaining -= int64(n)
	if err != nil {
		// A transport may report its own error for a read cut by the
		// deadline; the caller is promised one it can recognize.
		if !errors.Is(err, io.EOF) && errors.Is(b.ctx.Err(), context.DeadlineExceeded) && !errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("%w: %w", context.DeadlineExceeded, err)
		}
		// Nothing more will be read. The transport has already settled the
		// connection by the time it reports the end of the body, so
		// releasing here cannot cost a reusable connection.
		b.cancel()
	}
	return n, err
}

// Close closes first and releases second: cancelling an unfinished exchange
// before closing it would discard a connection that Close could have kept.
func (b *limitedBody) Close() error {
	err := b.body.Close()
	b.cancel()
	return err
}

type limitedReadWriter struct {
	*limitedBody
	w io.Writer
}

func (b *limitedReadWriter) Write(p []byte) (int, error) { return b.w.Write(p) }

// redirectPolicy is installed as http.Client.CheckRedirect.
func redirectPolicy(follow bool, limit int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if !follow {
			return http.ErrUseLastResponse
		}
		if len(via) > limit {
			return fmt.Errorf("%w: limit reached", ErrRedirectNotAllowed)
		}
		target, ok := originOf(req.URL)
		if !ok {
			return fmt.Errorf("%w: not http or https", ErrRedirectNotAllowed)
		}
		first, _ := originOf(via[0].URL)
		if last, _ := originOf(via[len(via)-1].URL); last.scheme == "https" && target.scheme == "http" {
			return fmt.Errorf("%w: https to http", ErrRedirectNotAllowed)
		}
		// net/http keeps credentials for subdomains and ignores the port.
		// Here any other origin loses them, and once a chain has left the
		// first origin it does not get them back by being bounced home:
		// that hop was chosen by a server that was never given them.
		left := target != first
		for _, hop := range via[1:] {
			if o, _ := originOf(hop.URL); o != first {
				left = true
			}
		}
		if left {
			for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie"} {
				req.Header.Del(name)
			}
		}
		return nil
	}
}

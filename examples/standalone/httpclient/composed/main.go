// Command composed shows the recommended composition: httpclient does one
// HTTP exchange, resilience owns every retry, and client/logging observes the
// logical call once.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/skolldire/go-engine/client"
	clientlogging "github.com/skolldire/go-engine/client/logging"
	"github.com/skolldire/go-engine/httpclient"
	"github.com/skolldire/go-engine/logging"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
)

// attemptError is what one attempt reports. It carries what the classifier
// needs to decide and nothing else: no body, no URL.
type attemptError struct {
	method string
	keyed  bool // the request carried an Idempotency-Key
	err    error
}

func (e *attemptError) Error() string { return e.method + ": " + e.err.Error() }
func (e *attemptError) Unwrap() error { return e.err }

// classify is the caller's policy. httpclient never retries by itself, so
// this function is the only place where a replay is decided.
func classify(err error) retry.Decision {
	var attempt *attemptError
	if !errors.As(err, &attempt) {
		return retry.DecisionStop
	}
	// Failures that another attempt cannot change.
	if errors.Is(err, httpclient.ErrOriginNotAllowed) || errors.Is(err, httpclient.ErrResponseTooLarge) ||
		errors.Is(err, httpclient.ErrRedirectNotAllowed) || errors.Is(err, httpclient.ErrInvalidCall) || errors.Is(err, context.Canceled) {
		return retry.DecisionStop
	}
	transient, unknown := true, true // a transport error: nothing is known about the outcome
	var status *httpclient.StatusError
	if errors.As(err, &status) {
		switch status.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			transient, unknown = true, false
		case http.StatusInternalServerError:
			transient, unknown = false, true
		default:
			return retry.DecisionStop
		}
	}
	switch attempt.method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		if transient {
			return retry.DecisionRetry
		}
		return retry.DecisionStop
	}
	// POST and PATCH are replayed only when the caller made them idempotent
	// with a key the server deduplicates on.
	if attempt.keyed && transient {
		return retry.DecisionRetry
	}
	// Without a key, a 500 or a broken connection may hide an applied side
	// effect. It is never retried; someone has to find out what happened.
	if unknown {
		return retry.DecisionReconcile
	}
	return retry.DecisionStop
}

type api struct {
	http    *httpclient.Client
	policy  resilience.Config
	observe client.Middleware[[]byte]
}

// call is one logical call: logged once outside, retried inside.
func (a *api) call(ctx context.Context, method, path string, body []byte, idempotencyKey string) ([]byte, error) {
	// The request is created inside the operation. Every attempt therefore
	// gets a fresh body and a fresh context, and sends the whole payload.
	attempt := client.Operation[[]byte](func(ctx context.Context) ([]byte, error) {
		fail := func(err error) ([]byte, error) {
			return nil, &attemptError{method: method, keyed: idempotencyKey != "", err: err}
		}
		req, err := a.http.NewRequest(ctx, method, path, body)
		if err != nil {
			return fail(err)
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
		resp, err := a.http.Do(ctx, req)
		if err != nil {
			return fail(err)
		}
		// Closed before returning, on every path: the attempt context is
		// cancelled when this function returns, and an open body would
		// keep the connection out of the pool for the next attempt.
		defer func() { _ = resp.Body.Close() }()
		if err := httpclient.CheckStatus(resp); err != nil {
			// The classifier needs the code, not the response; drain a
			// little so the connection can be reused.
			_, _ = io.CopyN(io.Discard, resp.Body, 4096)
			return fail(err)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fail(err)
		}
		return data, nil
	})
	resilient, err := resilience.Wrap(attempt, a.policy, resilience.Dependencies{Classify: classify})
	if err != nil {
		return nil, err
	}
	return client.Do(ctx, client.Invocation{Component: "inventory", Operation: method + " " + path}, resilient, a.observe)
}

// upstream is a loopback server that fails the first request for each path
// with the status named in the query and then succeeds.
type upstream struct {
	mu     sync.Mutex
	bodies map[string][]string
	conns  int
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	u.bodies[r.URL.Path] = append(u.bodies[r.URL.Path], string(data))
	first := len(u.bodies[r.URL.Path]) == 1
	u.mu.Unlock()
	if first {
		code := http.StatusServiceUnavailable
		if r.URL.Path == "/broken" {
			code = http.StatusInternalServerError
		}
		http.Error(w, "private upstream detail", code)
		return
	}
	_, _ = fmt.Fprintf(w, "%s done", r.Method)
}

func (u *upstream) attempts(path string) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.bodies[path]...)
}

func run() error {
	up := &upstream{bodies: map[string][]string{}}
	srv := httptest.NewUnstartedServer(up)
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			up.mu.Lock()
			up.conns++
			up.mu.Unlock()
		}
	}
	srv.Start()
	defer srv.Close()

	var out bytes.Buffer
	logConfig := logging.DefaultConfig()
	logConfig.Level = "debug"
	logger, err := logging.New(logConfig, logging.Dependencies{Output: &out})
	if err != nil {
		return err
	}
	observe, err := clientlogging.New[[]byte](clientlogging.DefaultConfig(), clientlogging.Dependencies{Logger: logger})
	if err != nil {
		return err
	}
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.MaxResponseBytes = 1 << 16
	// One connection only: if any attempt left its body open, the next
	// attempt could not get a connection and the call would time out.
	cfg.Transport.MaxConnsPerHost = 1
	cfg.RequestTimeout = 2 * time.Second
	c, err := httpclient.New(cfg, httpclient.Dependencies{})
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	policy := resilience.DefaultConfig()
	policy.Retry.MaxRetries = 2
	policy.Retry.InitialBackoff = time.Millisecond
	policy.Retry.MaxBackoff = time.Millisecond
	policy.Retry.Jitter = retry.JitterNone
	a := &api{http: c, policy: policy, observe: observe}
	ctx := context.Background()

	// logged checks that the logical call produced exactly one record and
	// that nothing the upstream wrote reached it.
	logged := func(step string) error {
		defer out.Reset()
		if bytes.Count(out.Bytes(), []byte("\n")) != 1 || bytes.Contains(out.Bytes(), []byte("private")) {
			return fmt.Errorf("%s: expected one clean log record, got %q", step, out.String())
		}
		return nil
	}
	same := func(step string, got []string, want ...string) error {
		if len(got) != len(want) {
			return fmt.Errorf("%s: upstream saw %d attempts, want %d", step, len(got), len(want))
		}
		for i, body := range got {
			if body != want[i] {
				return fmt.Errorf("%s: attempt %d sent %q", step, i+1, body)
			}
		}
		return nil
	}

	// GET: a 503 followed by success is retried.
	data, err := a.call(ctx, http.MethodGet, "/items", nil, "")
	if err != nil || string(data) != "GET done" {
		return fmt.Errorf("GET: %q, %w", data, err)
	}
	if err := errors.Join(same("GET", up.attempts("/items"), "", ""), logged("GET")); err != nil {
		return err
	}

	// PUT: retried, and the second attempt carries the whole body again.
	if data, err = a.call(ctx, http.MethodPut, "/items/7", []byte(`{"qty":3}`), ""); err != nil || string(data) != "PUT done" {
		return fmt.Errorf("PUT: %q, %w", data, err)
	}
	if err := errors.Join(same("PUT", up.attempts("/items/7"), `{"qty":3}`, `{"qty":3}`), logged("PUT")); err != nil {
		return err
	}

	// POST without an idempotency key: the 503 is returned, not replayed.
	_, err = a.call(ctx, http.MethodPost, "/orders", []byte(`{"sku":"a"}`), "")
	var status *httpclient.StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusServiceUnavailable || errors.Is(err, retry.ErrReconciliationRequired) {
		return fmt.Errorf("POST without key: %w", err)
	}
	if err := errors.Join(same("POST without key", up.attempts("/orders"), `{"sku":"a"}`), logged("POST without key")); err != nil {
		return err
	}

	// POST with a key: an explicit decision makes it replayable.
	if data, err = a.call(ctx, http.MethodPost, "/payments", []byte(`{"amount":5}`), "key-1"); err != nil || string(data) != "POST done" {
		return fmt.Errorf("POST with key: %q, %w", data, err)
	}
	if err := errors.Join(same("POST with key", up.attempts("/payments"), `{"amount":5}`, `{"amount":5}`), logged("POST with key")); err != nil {
		return err
	}

	// POST without a key answered 500: the outcome is unknown, so it is
	// handed back for reconciliation instead of being sent twice.
	_, err = a.call(ctx, http.MethodPost, "/broken", []byte(`{"sku":"b"}`), "")
	if !errors.Is(err, retry.ErrReconciliationRequired) || !errors.As(err, &status) || status.StatusCode != http.StatusInternalServerError {
		return fmt.Errorf("POST answered 500: %w", err)
	}
	if err := errors.Join(same("POST answered 500", up.attempts("/broken"), `{"sku":"b"}`), logged("POST answered 500")); err != nil {
		return err
	}

	up.mu.Lock()
	conns := up.conns
	up.mu.Unlock()
	if conns != 1 {
		return fmt.Errorf("eight attempts should share one connection, opened %d", conns)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("httpclient composed: per-attempt operation, caller classifier, bodies resent and closed, one log record per call passed")
}

// Command composed retries a resty call through resilience. resty's own
// retry loop stays off; the classifier below is the only replay policy.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/httpclient"
	httpresty "github.com/skolldire/go-engine/httpclient/resty"
	"github.com/skolldire/go-engine/resilience"
	"github.com/skolldire/go-engine/retry"
)

type stock struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

// classify retries what a GET may safely repeat. Both clients report a
// non-2xx as *httpclient.StatusError, so one classifier serves either.
func classify(err error) retry.Decision {
	var status *httpclient.StatusError
	if errors.As(err, &status) {
		switch status.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return retry.DecisionRetry
		}
	}
	return retry.DecisionStop
}

func run() error {
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if r.URL.Path == "/gone" {
			http.Error(w, "private upstream detail", http.StatusGone)
			return
		}
		if n == 1 {
			http.Error(w, "private upstream detail", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"sku":"bolt","qty":9}`)
	}))
	defer upstream.Close()

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = upstream.URL
	cfg.MaxResponseBytes = 1 << 16
	cfg.Transport.MaxConnsPerHost = 1
	base, err := httpclient.New(cfg, httpclient.Dependencies{})
	if err != nil {
		return err
	}
	defer base.CloseIdleConnections()
	c, err := httpresty.New(httpresty.Dependencies{HTTPClient: base})
	if err != nil {
		return err
	}
	policy := resilience.DefaultConfig()
	policy.Retry.MaxRetries = 2
	policy.Retry.InitialBackoff = time.Millisecond
	policy.Retry.MaxBackoff = time.Millisecond
	policy.Retry.Jitter = retry.JitterNone

	read := func(path string) client.Operation[stock] {
		return func(ctx context.Context) (stock, error) {
			var out stock
			// resty buffers and closes the body, so each attempt is
			// complete when this returns.
			resp, err := c.R(ctx).SetResult(&out).Get(path)
			if err != nil {
				return stock{}, err
			}
			return out, httpresty.CheckStatus(resp)
		}
	}
	call := func(path string) (stock, error) {
		op, err := resilience.Wrap(read(path), policy, resilience.Dependencies{Classify: classify})
		if err != nil {
			return stock{}, err
		}
		return client.Do(context.Background(), client.Invocation{Component: "inventory", Operation: "read"}, op)
	}

	got, err := call("/stock/bolt")
	if err != nil || got != (stock{SKU: "bolt", Qty: 9}) || hits.Load() != 2 {
		return fmt.Errorf("503 then success: %+v, %w after %d requests", got, err, hits.Load())
	}
	hits.Store(1)
	_, err = call("/gone")
	var status *httpclient.StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusGone || hits.Load() != 2 {
		return fmt.Errorf("410 must not be retried: %w after %d requests", err, hits.Load()-1)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("httpclient/resty composed: resilience owns the retry, resty sends once per attempt passed")
}

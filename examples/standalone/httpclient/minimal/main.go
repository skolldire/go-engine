// Command minimal uses httpclient alone against a loopback server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/skolldire/go-engine/httpclient"
)

func run() error {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/items/7":
			body, _ := io.ReadAll(r.Body)
			_, _ = fmt.Fprintf(w, "%s %s", r.Method, body)
		case "/api/big":
			_, _ = io.WriteString(w, strings.Repeat("x", 4096))
		default:
			http.Error(w, "private upstream detail", http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = upstream.URL + "/api"
	// The two limits with no safe default: the caller states them.
	cfg.MaxResponseBytes = 1024
	cfg.Transport.MaxConnsPerHost = 4
	c, err := httpclient.New(cfg, httpclient.Dependencies{})
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	ctx := context.Background()

	// A request under BaseURL, with a body that can be replayed.
	req, err := c.NewRequest(ctx, http.MethodPut, "/items/7", []byte("payload"))
	if err != nil {
		return err
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || httpclient.CheckStatus(resp) != nil || string(data) != "PUT payload" {
		return fmt.Errorf("round trip: %q, %w", data, err)
	}

	// A non-2xx is a response. CheckStatus turns it into an error that
	// names the code and nothing the upstream wrote.
	req, _ = c.NewRequest(ctx, http.MethodGet, "/missing", nil)
	if resp, err = c.Do(ctx, req); err != nil {
		return err
	}
	err = httpclient.CheckStatus(resp)
	_ = resp.Body.Close()
	var status *httpclient.StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound || strings.Contains(err.Error(), "private") {
		return fmt.Errorf("status: %w", err)
	}

	// A body over MaxResponseBytes is an error, not a short read.
	req, _ = c.NewRequest(ctx, http.MethodGet, "/big", nil)
	resp, err = c.Do(ctx, req)
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	if !errors.Is(err, httpclient.ErrResponseTooLarge) {
		return fmt.Errorf("limit: %w", err)
	}

	// Another origin is refused before any connection is attempted.
	foreign, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://203.0.113.1/steal", nil)
	if _, err = c.Do(ctx, foreign); !errors.Is(err, httpclient.ErrOriginNotAllowed) {
		return fmt.Errorf("origin: %w", err)
	}
	if _, err = c.NewRequest(ctx, http.MethodGet, "//203.0.113.1/steal", nil); !errors.Is(err, httpclient.ErrOriginNotAllowed) {
		return fmt.Errorf("origin path: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("httpclient minimal: request, status, size limit and origin rule passed")
}

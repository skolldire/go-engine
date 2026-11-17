// Command minimal sends JSON both ways with resty over an httpclient.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/skolldire/go-engine/httpclient"
	httpresty "github.com/skolldire/go-engine/httpclient/resty"
)

type order struct {
	ID  int    `json:"id"`
	SKU string `json:"sku"`
}

func run() error {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders" {
			http.Error(w, "private upstream detail", http.StatusNotFound)
			return
		}
		var in order
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		in.ID = 42
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(in)
	}))
	defer upstream.Close()

	// Everything configurable lives in the httpclient; the adapter has no
	// configuration of its own.
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = upstream.URL + "/api"
	cfg.MaxResponseBytes = 1 << 16
	cfg.Transport.MaxConnsPerHost = 4
	base, err := httpclient.New(cfg, httpclient.Dependencies{})
	if err != nil {
		return err
	}
	defer base.CloseIdleConnections()
	c, err := httpresty.New(httpresty.Dependencies{HTTPClient: base})
	if err != nil {
		return err
	}
	ctx := context.Background()

	// resty marshals the request and unmarshals the result.
	var created order
	resp, err := c.R(ctx).SetBody(order{SKU: "bolt"}).SetResult(&created).Post("/orders")
	if err != nil {
		return err
	}
	if err := httpresty.CheckStatus(resp); err != nil || created != (order{ID: 42, SKU: "bolt"}) {
		return fmt.Errorf("round trip: %+v, %w", created, err)
	}

	// A non-2xx comes back as a response; CheckStatus yields the same
	// error type as httpclient, without the body.
	resp, err = c.Get(ctx, "/missing", nil)
	if err != nil {
		return err
	}
	var status *httpclient.StatusError
	if err := httpresty.CheckStatus(resp); !errors.As(err, &status) || status.StatusCode != http.StatusNotFound {
		return fmt.Errorf("status: %w", err)
	}

	// The origin rule of the httpclient holds through resty.
	if _, err := c.Get(ctx, "http://203.0.113.1/steal", nil); !errors.Is(err, httpclient.ErrOriginNotAllowed) {
		return fmt.Errorf("origin: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("httpclient/resty minimal: JSON round trip, status and inherited origin rule passed")
}

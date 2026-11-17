package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/skolldire/go-engine/presets/httpapi"
	routererrors "github.com/skolldire/go-engine/router/errors"
)

var (
	errInput    = errors.New("invalid order")
	errNotFound = errors.New("order not found")
)

type order struct {
	ID          string `json:"id"`
	Item        string `json:"item"`
	AmountCents int64  `json:"amount_cents"`
}
type orderStore interface {
	save(context.Context, order) error
	find(context.Context, string) (order, error)
}
type memoryStore struct {
	mu     sync.RWMutex
	orders map[string]order
}

func (m *memoryStore) save(ctx context.Context, o order) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[o.ID] = o
	return nil
}
func (m *memoryStore) find(ctx context.Context, id string) (order, error) {
	if ctx.Err() != nil {
		return order{}, ctx.Err()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok {
		return order{}, errNotFound
	}
	return o, nil
}
func classify(err error) routererrors.Problem {
	switch {
	case errors.Is(err, errInput):
		return routererrors.Problem{Status: 400, Code: "invalid_order", Title: "Invalid order"}
	case errors.Is(err, errNotFound):
		return routererrors.Problem{Status: 404, Code: "order_not_found", Title: "Order not found"}
	default:
		return routererrors.Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
	}
}
func routes(store orderStore, responder *routererrors.Responder, log *slog.Logger) []httpapi.Route {
	fail := func(w http.ResponseWriter, r *http.Request, err error) {
		log.DebugContext(r.Context(), "request failed")
		if e := responder.Write(r.Context(), w, err); e != nil {
			log.ErrorContext(r.Context(), "response write failed")
		}
	}
	write := func(w http.ResponseWriter, status int, o order) {
		body, err := json.Marshal(o)
		if err != nil {
			log.Error("encoding failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err = w.Write(body); err != nil {
			log.Error("response write failed")
		}
	}
	return []httpapi.Route{
		{Method: "POST", Pattern: "/orders", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var o order
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&o); err != nil || o.ID == "" || o.Item == "" || o.AmountCents < 0 {
				fail(w, r, errInput)
				return
			}
			if err := store.save(r.Context(), o); err != nil {
				fail(w, r, err)
				return
			}
			write(w, 201, o)
		})},
		{Method: "GET", Pattern: "/orders/{id}", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			o, err := store.find(r.Context(), r.PathValue("id"))
			if err != nil {
				fail(w, r, err)
				return
			}
			write(w, 200, o)
		})},
	}
}

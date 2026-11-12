// Package handler exposes the service over HTTP.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/skolldire/go-engine/example/internal/repository"
	"github.com/skolldire/go-engine/example/internal/usecase"
	"github.com/skolldire/go-engine/pkg/router"
)

// Order wires the order endpoints.
type Order struct {
	create *usecase.CreateOrder
	get    *usecase.GetOrder
}

// NewOrder builds the handler.
func NewOrder(create *usecase.CreateOrder, get *usecase.GetOrder) *Order {
	return &Order{create: create, get: get}
}

// Register mounts the routes on the engine's router. Taking router.Service
// rather than *chi.Mux keeps the handler independent of the router internals.
func (h *Order) Register(r router.Service) {
	r.AddRoute(http.MethodPost, "/orders", h.Create)
	r.AddRoute(http.MethodGet, "/orders/{id}", h.Get)
}

// Create handles POST /orders.
func (h *Order) Create(w http.ResponseWriter, r *http.Request) {
	var order repository.Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}

	if err := h.create.Execute(r.Context(), order); err != nil {
		// The use case reports a validation failure as a sentinel, so the
		// handler maps it to a status without matching on message text.
		if errors.Is(err, usecase.ErrInvalidOrder) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not store the order")
		return
	}

	writeJSON(w, http.StatusCreated, order)
}

// Get handles GET /orders/{id}.
func (h *Order) Get(w http.ResponseWriter, r *http.Request) {
	order, err := h.get.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidOrder) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	writeJSON(w, http.StatusOK, order)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

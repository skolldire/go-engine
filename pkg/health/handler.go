package health

import (
	"encoding/json"
	"net/http"
)

// HTTPHandler exposes health endpoints over HTTP.
type HTTPHandler struct {
	svc Service
}

// NewHTTPHandler creates an HTTPHandler backed by svc.
func NewHTTPHandler(svc Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// Routes returns an http.Handler with three endpoints:
//
//	GET /live  → 200 while the process is running
//	GET /ready → 200 if all dependencies are healthy, 503 otherwise
//	GET /deps  → JSON with per-dependency status; 503 if any is down
func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /live", h.liveHandler)
	mux.HandleFunc("GET /ready", h.readyHandler)
	mux.HandleFunc("GET /deps", h.depsHandler)
	return mux
}

func (h *HTTPHandler) liveHandler(w http.ResponseWriter, _ *http.Request) {
	if !h.svc.IsLive() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) readyHandler(w http.ResponseWriter, _ *http.Request) {
	if !h.svc.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) depsHandler(w http.ResponseWriter, _ *http.Request) {
	status := h.svc.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	if status.Status == StatusDown {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status) //nolint:errcheck
}

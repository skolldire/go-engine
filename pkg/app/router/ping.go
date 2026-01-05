package router

import (
	"encoding/json"
	"net/http"
)

type PingResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// pingHandler writes a JSON ping response with status "ok" and message "pong".
// It sets the Content-Type to "application/json" and sends HTTP 200 (OK).
func pingHandler(w http.ResponseWriter, r *http.Request) {
	response := PingResponse{
		Status:  "ok",
		Message: "pong",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

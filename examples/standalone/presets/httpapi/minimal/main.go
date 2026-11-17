package main

import (
	"fmt"
	"github.com/skolldire/go-engine/presets/httpapi"
	"github.com/skolldire/go-engine/router"
	"net/http"
	"net/http/httptest"
)

func run() error {
	cfg := httpapi.DefaultConfig()
	cfg.EnableOperations = false
	cfg.EnableRecovery = false
	cfg.Router = router.Config{AllowedHosts: []string{"example.test"}}
	cfg.Server.Listen = "127.0.0.1:0"
	app, e := httpapi.New(cfg, httpapi.Dependencies{Routes: []httpapi.Route{{Method: "GET", Pattern: "/", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })}}})
	if e != nil {
		return e
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest("GET", "http://example.test/", nil))
	if w.Code != 204 || app.OperationsHandler() != nil || w.Header().Get("X-Request-ID") == "" {
		return fmt.Errorf("unexpected assembly")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("httpapi minimal passed")
}

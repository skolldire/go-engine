package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/logging"
	"github.com/skolldire/go-engine/presets/httpapi"
	"github.com/skolldire/go-engine/router"
	routererrors "github.com/skolldire/go-engine/router/errors"
	"github.com/skolldire/go-engine/router/middleware"
	"net/http"
	"net/http/httptest"
)

func run() error {
	var logs bytes.Buffer
	log, e := logging.New(logging.DefaultConfig(), logging.Dependencies{Output: &logs})
	if e != nil {
		return e
	}
	reg, e := health.New(health.DefaultConfig())
	if e != nil {
		return e
	}
	if e = reg.Register("memory", health.CheckerFunc(func(context.Context) error { return errors.New("private memory failure") })); e != nil {
		return e
	}
	res, e := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: func(error) routererrors.Problem {
		return routererrors.Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
	}, RequestID: func(ctx context.Context) string { id, _ := middleware.RequestIDFrom(ctx); return id }})
	if e != nil {
		return e
	}
	cfg := httpapi.DefaultConfig()
	cfg.Router = router.Config{AllowedHosts: []string{"api.test"}}
	cfg.OperationsRouter = router.Config{AllowedHosts: []string{"internal.test"}}
	cfg.Server.Listen = "127.0.0.1:0"
	cfg.OperationsServer.Listen = "127.0.0.1:0"
	cfg.Operations.Pprof = true
	cfg.OperationsServer.WriteTimeout = 0
	app, e := httpapi.New(cfg, httpapi.Dependencies{Health: reg, Routes: []httpapi.Route{{Method: "GET", Pattern: "/panic", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private panic value") })}}, OnPanic: func(w http.ResponseWriter, r *http.Request, v any) {
		log.ErrorContext(r.Context(), "panic recovered")
		if err := res.Write(r.Context(), w, errors.New("private cause")); err != nil {
			log.ErrorContext(r.Context(), "response write failed")
		}
	}})
	if e != nil {
		return e
	}
	for _, tt := range []struct {
		h      http.Handler
		url    string
		status int
	}{{app.Handler(), "http://api.test/panic", 500}, {app.OperationsHandler(), "http://internal.test/ready", 503}, {app.OperationsHandler(), "http://internal.test/deps", 200}, {app.Handler(), "http://internal.test/ready", 421}} {
		w := httptest.NewRecorder()
		tt.h.ServeHTTP(w, httptest.NewRequest("GET", tt.url, nil))
		if w.Code != tt.status || bytes.Contains(w.Body.Bytes(), []byte("private")) {
			return fmt.Errorf("unsafe or unexpected response")
		}
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("httpapi composed passed")
}

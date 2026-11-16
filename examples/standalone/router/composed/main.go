package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/logging"
	"github.com/skolldire/go-engine/router"
	routererrors "github.com/skolldire/go-engine/router/errors"
	"github.com/skolldire/go-engine/router/middleware"
	"github.com/skolldire/go-engine/router/operations"
	"net"
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
	if e = reg.Register("memory", health.CheckerFunc(func(context.Context) error { return nil })); e != nil {
		return e
	}
	ops, e := operations.New(operations.DefaultConfig(), operations.Dependencies{Health: reg})
	if e != nil {
		return e
	}
	res, e := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: func(error) routererrors.Problem {
		return routererrors.Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
	}, RequestID: func(ctx context.Context) string { id, _ := middleware.RequestIDFrom(ctx); return id }})
	if e != nil {
		return e
	}
	m, e := router.New(router.Config{AllowedHosts: []string{"127.0.0.1", "example.com"}})
	if e != nil {
		return e
	}
	id, e := middleware.RequestID(middleware.RequestIDConfig{Generate: func() string { return "application-id" }})
	if e != nil {
		return e
	}
	recoverMW, e := middleware.Recover(func(w http.ResponseWriter, r *http.Request, v any) {
		log.ErrorContext(r.Context(), "operation panic")
		if err := res.Write(r.Context(), w, errors.New("private cause")); err != nil {
			log.ErrorContext(r.Context(), "response write failed")
		}
	})
	if e != nil {
		return e
	}
	if e = m.Use(id, recoverMW); e != nil {
		return e
	}
	if e = m.Handle("GET", "", "/business", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private panic") })); e != nil {
		return e
	}
	if e = m.Freeze(); e != nil {
		return e
	}
	w := httptest.NewRecorder()
	m.ServeHTTP(w, httptest.NewRequest("GET", "http://example.com/business", nil))
	if w.Code != 500 || bytes.Contains(w.Body.Bytes(), []byte("private")) {
		return fmt.Errorf("unsafe response")
	}
	w = httptest.NewRecorder()
	ops.ServeHTTP(w, httptest.NewRequest("GET", "http://internal/ready", nil))
	if w.Code != 200 {
		return fmt.Errorf("readiness failed")
	}
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg := router.DefaultServerConfig()
	cfg.Listen = ln.Addr().String()
	done := make(chan error, 1)
	go func() { done <- router.ServeListener(ctx, cfg, ln, m) }()
	transport := &http.Transport{}
	client := &http.Client{Transport: transport}
	resp, e := client.Get("http://" + ln.Addr().String() + "/business")
	if resp != nil {
		_ = resp.Body.Close()
	}
	transport.CloseIdleConnections()
	cancel()
	serveErr := <-done
	if e != nil {
		return e
	}
	return serveErr
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("router composed passed")
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/logging"
	routererrors "github.com/skolldire/go-engine/router/errors"
)

type requestIDKey struct{}

func run() error {
	var logs bytes.Buffer
	logger, err := logging.New(logging.DefaultConfig(), logging.Dependencies{Output: &logs})
	if err != nil {
		return err
	}
	registry, err := health.New(health.DefaultConfig())
	if err != nil {
		return err
	}
	private := errors.New("private-database-credential")
	if err = registry.Register("database", health.CheckerFunc(func(context.Context) error { return private })); err != nil {
		return err
	}
	if err = registry.Register("panic-check", health.CheckerFunc(func(context.Context) error { panic("private-panic-payload") })); err != nil {
		return err
	}
	responder, err := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: func(err error) routererrors.Problem {
		if errors.Is(err, health.ErrNotReady) {
			return routererrors.Problem{Status: 503, Code: "unavailable", Title: "Service Unavailable"}
		}
		return routererrors.Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
	}, RequestID: func(ctx context.Context) string { id, _ := ctx.Value(requestIDKey{}).(string); return id }})
	if err != nil {
		return err
	}
	var diagnostic health.Report
	var writeErr error
	mux := http.NewServeMux()
	mux.HandleFunc("GET /example", func(w http.ResponseWriter, req *http.Request) {
		report, probeErr := registry.Ready(req.Context())
		diagnostic = report
		for _, check := range report.Checks {
			if check.Err != nil {
				logger.LogAttrs(req.Context(), slog.LevelError, "dependency down", slog.String("check", check.Name), slog.Any("error", check.Err))
			}
		}
		if probeErr != nil {
			writeErr = responder.Write(req.Context(), w, probeErr)
		}
	})
	req := httptest.NewRequest(http.MethodGet, "/example", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "application-id"))
	req.Header.Set("X-Request-ID", "untrusted-header")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if writeErr != nil {
		return writeErr
	}
	if w.Code != 503 || w.Header().Get("Content-Type") != "application/problem+json" || diagnostic.Checks[0].Err != private {
		return errors.New("readiness mapping/original cause mismatch")
	}
	var body map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		return err
	}
	if len(body) != 5 || body["type"] != "about:blank" || body["code"] != "unavailable" || body["request_id"] != "application-id" {
		return errors.New("wire schema mismatch")
	}
	rawReport, err := json.Marshal(diagnostic)
	if err != nil {
		return err
	}
	for _, secret := range []string{private.Error(), "private-panic-payload", "Stack", "Value", "untrusted-header"} {
		if bytes.Contains(w.Body.Bytes(), []byte(secret)) || bytes.Contains(rawReport, []byte(secret)) {
			return errors.New("private data leaked")
		}
	}
	if !bytes.Contains(logs.Bytes(), []byte(private.Error())) {
		return errors.New("caller logging not explicit")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("router/errors composed: health-to-503 mapping, caller logging and private causes verified")
}

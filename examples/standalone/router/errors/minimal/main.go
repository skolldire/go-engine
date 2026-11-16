package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	routererrors "github.com/skolldire/go-engine/router/errors"
)

type failingWriter struct {
	header http.Header
	writes int
}

func (w *failingWriter) Header() http.Header            { return w.header }
func (*failingWriter) WriteHeader(int)                  {}
func (w *failingWriter) Write(body []byte) (int, error) { w.writes++; return len(body) / 2, nil }
func run() error {
	missing := errors.New("private database detail")
	classify := func(err error) routererrors.Problem {
		if errors.Is(err, missing) {
			return routererrors.Problem{Status: 404, Code: "not_found", Title: "Not Found"}
		}
		return routererrors.Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
	}
	r, err := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: classify, RequestID: func(context.Context) string { return "explicit-id" }})
	if err != nil {
		return err
	}
	for _, original := range []error{missing, errors.New("unknown private cause")} {
		w := httptest.NewRecorder()
		if err = r.Write(context.Background(), w, original); err != nil {
			return err
		}
		var body map[string]any
		if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			return err
		}
		if body["type"] != "about:blank" || body["request_id"] != "explicit-id" || strings.Contains(w.Body.String(), "private") {
			return errors.New("unsafe public response")
		}
	}
	if err = r.Write(context.Background(), nil, missing); !errors.Is(err, routererrors.ErrInvalidCall) {
		return errors.New("nil writer accepted")
	}
	writer := &failingWriter{header: make(http.Header)}
	if err = r.Write(context.Background(), writer, missing); !errors.Is(err, io.ErrShortWrite) || writer.writes != 1 {
		return errors.New("short-write mismatch")
	}
	bad, err := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: func(error) routererrors.Problem { return routererrors.Problem{Status: 999} }})
	if err != nil {
		return err
	}
	w := httptest.NewRecorder()
	if err = bad.Write(context.Background(), w, missing); !errors.Is(err, routererrors.ErrInvalidProblem) || w.Code != 500 {
		return errors.New("fallback mismatch")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("router/errors minimal: safe Problem Details and writer failures verified")
}

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/router/operations"
	"net/http/httptest"
	"strings"
)

func run() error {
	reg, e := health.New(health.DefaultConfig())
	if e != nil {
		return e
	}
	if e = reg.Register("memory", health.CheckerFunc(func(context.Context) error { return errors.New("private cause") })); e != nil {
		return e
	}
	h, e := operations.New(operations.DefaultConfig(), operations.Dependencies{Health: reg})
	if e != nil {
		return e
	}
	for path, want := range map[string]int{"/ping": 200, "/ready": 503, "/deps": 200} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://internal"+path, nil))
		if w.Code != want || strings.Contains(w.Body.String(), "private cause") {
			return fmt.Errorf("unsafe operations response")
		}
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("operations consumer passed")
}

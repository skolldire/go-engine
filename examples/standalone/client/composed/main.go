package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	c "github.com/skolldire/go-engine/client"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type repository struct{ transport http.RoundTripper }

func (r repository) fetch(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/record", nil)
	if err != nil {
		return "", err
	}
	response, err := r.transport.RoundTrip(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	return string(data), err
}

type contextKey struct{}

func run() error {
	var events []string
	original := errors.New("transport failure")
	fail := false
	transport := transportFunc(func(request *http.Request) (*http.Response, error) {
		events = append(events, "transport")
		if request.Context().Value(contextKey{}) != "kept" {
			return nil, errors.New("context value lost")
		}
		if _, ok := request.Context().Deadline(); !ok {
			return nil, errors.New("deadline missing")
		}
		if fail {
			return nil, original
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("record"))}, nil
	})
	repo := repository{transport: transport}
	record := func(next c.Handler[string]) c.Handler[string] {
		return func(ctx context.Context, inv c.Invocation, op c.Operation[string]) (string, error) {
			events = append(events, "outer.before")
			value, err := next(ctx, inv, op)
			events = append(events, "outer.after")
			return value, err
		}
	}
	bounded := func(next c.Handler[string]) c.Handler[string] {
		return func(ctx context.Context, inv c.Invocation, op c.Operation[string]) (string, error) {
			events = append(events, "inner.before")
			derived, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			value, err := next(derived, inv, op)
			events = append(events, "inner.after")
			return value, err
		}
	}
	ctx := context.WithValue(context.Background(), contextKey{}, "kept")
	invocation := c.Invocation{Component: "repository", Operation: "Fetch"}
	for _, failure := range []bool{false, true} {
		fail = failure
		events = nil
		result, err := c.Do(ctx, invocation, repo.fetch, record, bounded)
		if failure {
			if err != original {
				return errors.New("transport cause changed")
			}
		} else if err != nil || result != "record" {
			return errors.New("unexpected I/O result")
		}
		if !reflect.DeepEqual(events, []string{"outer.before", "inner.before", "transport", "inner.after", "outer.after"}) {
			return errors.New("middleware order changed")
		}
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("composed: injected in-memory I/O, two middleware, deadline and original cause OK")
}

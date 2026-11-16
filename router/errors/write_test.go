package errors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
)

type controlledWriter struct {
	headers                                    http.Header
	headersCalled, statusCalls, writes, closes int
	status                                     int
	body                                       []byte
	n                                          int
	err                                        error
	onWrite                                    func()
	panicHeader                                bool
}

func (w *controlledWriter) Header() http.Header {
	w.headersCalled++
	if w.panicHeader {
		panic("writer header panic")
	}
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}
func (w *controlledWriter) WriteHeader(code int) { w.status = code; w.statusCalls++ }
func (w *controlledWriter) Write(body []byte) (int, error) {
	w.writes++
	w.body = append(w.body, body...)
	if w.onWrite != nil {
		w.onWrite()
	}
	if w.n >= 0 {
		return w.n, w.err
	}
	return len(body), w.err
}
func (w *controlledWriter) Close() error { w.closes++; return nil }
func TestWriteValidationAndOrder(t *testing.T) {
	calls := 0
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { calls++; return valid() }, RequestID: func(context.Context) string { calls++; return "id" }})
	var typedNil *controlledWriter
	for _, tt := range []struct {
		ctx              context.Context
		w                http.ResponseWriter
		input, errorKind error
	}{{nil, nil, nil, ErrNilContext}, {context.Background(), nil, nil, ErrInvalidCall}, {context.Background(), typedNil, nil, ErrInvalidCall}, {context.Background(), &controlledWriter{}, nil, ErrInvalidCall}} {
		err := r.Write(tt.ctx, tt.w, tt.input)
		if !errors.Is(err, tt.errorKind) || calls != 0 {
			t.Fatal("validation order")
		}
		if w, ok := tt.w.(*controlledWriter); ok && w != nil && (w.headersCalled != 0 || w.writes != 0 || w.statusCalls != 0) {
			t.Fatal("invalid call wrote")
		}
	}
}
func TestWriteHeadersAndBusinessErrorNotReturned(t *testing.T) {
	original := fmt.Errorf("wrapped: %w", &typedError{})
	classified := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(err error) Problem {
		classified++
		if err != original {
			t.Fatal("original replaced")
		}
		var typed *typedError
		if !errors.As(err, &typed) {
			t.Fatal("typed cause")
		}
		return valid()
	}, RequestID: func(got context.Context) string {
		if got != ctx {
			t.Fatal("context")
		}
		return "body-id"
	}})
	w := &controlledWriter{n: -1, headers: http.Header{"X-Caller": []string{"kept"}, "X-Request-Id": []string{"caller-owned"}}}
	if err := r.Write(ctx, w, original); err != nil {
		t.Fatal("business error returned", err)
	}
	if classified != 1 || w.status != 404 || w.statusCalls != 1 || w.writes != 1 || w.closes != 0 {
		t.Fatal("write count/ownership")
	}
	if w.headers.Get("Content-Type") != "application/problem+json" || w.headers.Get("Cache-Control") != "no-store" || w.headers.Get("X-Content-Type-Options") != "nosniff" || w.headers.Get("X-Caller") != "kept" || w.headers.Get("X-Request-ID") != "caller-owned" {
		t.Fatal("headers")
	}
	if !strings.Contains(string(w.body), `"request_id":"body-id"`) || strings.Contains(string(w.body), "wrapped") {
		t.Fatal("body privacy/id")
	}
}

type typedError struct{}

func (*typedError) Error() string { return "private typed cause" }
func TestWriterFailuresAndFallback(t *testing.T) {
	original := &typedError{}
	for _, fallback := range []bool{false, true} {
		for _, failure := range []string{"full-error", "partial-error", "short"} {
			r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem {
				if fallback {
					return Problem{Status: 900, Title: "private", Details: "private"}
				}
				return valid()
			}})
			w := &controlledWriter{n: -1, err: original}
			if failure != "full-error" {
				w.n = 3
			}
			if failure == "short" {
				w.err = nil
			}
			err := r.Write(context.Background(), w, &panicTextError{})
			want := error(original)
			if failure == "short" {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) || errors.Is(err, ErrInvalidProblem) != fallback || w.writes != 1 || w.statusCalls != 1 {
				t.Fatal("combined cause/no retry", err)
			}
			if failure != "short" {
				var typed *typedError
				if !errors.As(err, &typed) {
					t.Fatal("typed writer cause")
				}
			}
			if fallback && (w.status != 500 || strings.Contains(string(w.body), "private")) {
				t.Fatal("fallback leaked")
			}
		}
	}
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { return Problem{} }})
	w := httptest.NewRecorder()
	if err := r.Write(context.Background(), w, &panicTextError{}); !errors.Is(err, ErrInvalidProblem) || w.Code != 500 {
		t.Fatal("fallback must write before returning sentinel")
	}
}
func TestCallbackAndWriterPanics(t *testing.T) {
	token := &struct{}{}
	for _, stage := range []string{"classify", "request-id", "write"} {
		t.Run(stage, func(t *testing.T) {
			w := &controlledWriter{n: -1}
			deps := Dependencies{Classify: func(error) Problem { return valid() }}
			switch stage {
			case "classify":
				deps.Classify = func(error) Problem { panic(token) }
			case "request-id":
				deps.RequestID = func(context.Context) string { panic(token) }
			case "write":
				w.onWrite = func() { panic(token) }
			}
			r := newResponder(t, DefaultConfig(), deps)
			func() {
				defer func() {
					if recover() != token {
						t.Error("panic changed")
					}
				}()
				_ = r.Write(context.Background(), w, &panicTextError{})
			}()
			if stage != "write" && (w.headersCalled != 0 || w.writes != 0) {
				t.Fatal("callback failure committed response")
			}
		})
	}
}
func TestBlockingWriterNotAbandoned(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, gate := make(chan struct{}), make(chan struct{})
		w := &controlledWriter{n: -1, onWrite: func() { close(entered); <-gate }}
		r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { return valid() }})
		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(context.Background())
		go func() { done <- r.Write(ctx, w, &panicTextError{}) }()
		<-entered
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("writer abandoned")
		default:
		}
		close(gate)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
func TestConcurrentResponsesAndInstances(t *testing.T) {
	type key struct{}
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { return valid() }, RequestID: func(ctx context.Context) string { return ctx.Value(key{}).(string) }})
	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Go(func() {
			for i := range 50 {
				id := fmt.Sprintf("id-%d-%d", worker, i)
				ctx := context.WithValue(context.Background(), key{}, id)
				w := httptest.NewRecorder()
				if err := r.Write(ctx, w, &panicTextError{}); err != nil || w.Code != 404 || !strings.Contains(w.Body.String(), `"request_id":"`+id+`"`) {
					t.Error("response contamination")
				}
			}
		})
	}
	wg.Wait()
	other := newResponder(t, Config{MaxDetailsBytes: 1}, Dependencies{Classify: func(error) Problem { return Problem{Status: 403, Code: "other", Title: "Forbidden"} }})
	w := httptest.NewRecorder()
	if err := other.Write(context.Background(), w, &panicTextError{}); err != nil || w.Code != 403 || strings.Contains(w.Body.String(), "request_id") {
		t.Fatal("instance contamination")
	}
}

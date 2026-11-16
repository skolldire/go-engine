package errors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
	"weak"
)

func TestHandlerIntegration(t *testing.T) {
	original := &typedError{}
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(err error) Problem {
		if !errors.Is(err, original) {
			t.Fatal("original error lost")
		}
		return Problem{Status: 422, Code: "invalid_order", Title: "Invalid order", Details: "Public field hint"}
	}})
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := r.Write(req.Context(), w, original); err != nil {
			t.Error(err)
		}
	})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/orders", nil))
	if w.Code != 422 || strings.Contains(w.Body.String(), original.Error()) {
		t.Fatal("unsafe HTTP response")
	}
}

type largeInputError struct{ data [64 << 10]byte }

func (*largeInputError) Error() string { panic("large error inspected") }
func TestNoRetainedInputOrGoroutineLeaks(t *testing.T) {
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { return valid() }})
	pointers := make([]weak.Pointer[largeInputError], 16)
	for i := range pointers {
		pointers[i] = func() weak.Pointer[largeInputError] {
			original := new(largeInputError)
			original.data[0] = 1
			ref := weak.Make(original)
			if err := r.Write(context.Background(), httptest.NewRecorder(), original); err != nil {
				t.Fatal(err)
			}
			runtime.KeepAlive(original)
			return ref
		}()
	}
	for range 3 {
		runtime.GC()
	}
	for _, ref := range pointers {
		if ref.Value() != nil {
			t.Fatal("responder retained original error")
		}
	}
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("standard leak profile missing")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "total 0") {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}
func FuzzConfig(f *testing.F) {
	f.Add(true, 4096)
	f.Add(false, 0)
	f.Fuzz(func(t *testing.T, enabled bool, limit int) {
		cfg := Config{enabled, limit}
		before := cfg
		r, err := NewResponder(cfg, Dependencies{Classify: func(error) Problem { return valid() }})
		if cfg != before || (err == nil) != (limit > 0) {
			t.Fatal("config invariant")
		}
		if limit > 0 && r == nil {
			t.Fatal("constructor")
		}
	})
}
func FuzzWrite(f *testing.F) {
	f.Add(404, "not_found", "Not found", "public", "id-1", true, uint16(128))
	f.Add(999, "private-invalid", "\xff", "private", "bad\n", true, uint16(1))
	f.Fuzz(func(t *testing.T, status int, code, title, detail, id string, include bool, rawLimit uint16) {
		if len(code)+len(title)+len(detail)+len(id) > 16384 {
			t.Skip()
		}
		limit := int(rawLimit%4096) + 1
		p := Problem{status, code, title, detail}
		originalProblem := p
		classifications, ids := 0, 0
		r := newResponder(t, Config{include, limit}, Dependencies{Classify: func(err error) Problem {
			classifications++
			if _, ok := err.(*panicTextError); !ok {
				t.Fatal("input changed")
			}
			return p
		}, RequestID: func(context.Context) string { ids++; return id }})
		for turn := range 2 {
			w := httptest.NewRecorder()
			err := r.Write(context.Background(), w, &panicTextError{})
			validCore := status >= 400 && status <= 599 && len(code) > 0 && len(code) <= 64 && len(title) > 0 && len(title) <= 256 && utf8.ValidString(title)
			for _, c := range []byte(code) {
				if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.-", rune(c)) {
					validCore = false
				}
			}
			if errors.Is(err, ErrInvalidProblem) == validCore || !json.Valid(w.Body.Bytes()) || w.Code < 400 || w.Code > 599 {
				t.Fatal("status/JSON/error invariant")
			}
			var got map[string]any
			if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
				t.Fatal(e)
			}
			expectedStatus := status
			expectedTitle, expectedCode := title, code
			if !validCore {
				expectedStatus = 500
				expectedTitle = "Internal Server Error"
				expectedCode = "internal_error"
			}
			if w.Code != expectedStatus || got["status"] != float64(expectedStatus) || got["title"] != expectedTitle || got["code"] != expectedCode || got["type"] != "about:blank" {
				t.Fatal("core wire invariant")
			}
			expectedFields := 4
			if validCore && detail != "" && len(detail) <= limit && utf8.ValidString(detail) {
				expectedFields++
				if got["detail"] != detail {
					t.Fatal("detail changed")
				}
			} else if _, found := got["detail"]; found {
				t.Fatal("detail not omitted")
			}
			validID := include && id != "" && len(id) <= 128 && utf8.ValidString(id)
			for _, c := range id {
				if unicode.IsControl(c) {
					validID = false
				}
			}
			if validID {
				expectedFields++
				if got["request_id"] != id {
					t.Fatal("ID changed")
				}
			} else if _, found := got["request_id"]; found {
				t.Fatal("ID not omitted")
			}
			if len(got) != expectedFields || w.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatal("extra members/format")
			}
			if classifications != turn+1 || ids != map[bool]int{false: 0, true: turn + 1}[include] || p != originalProblem {
				t.Fatal("callback/mutation invariant")
			}
		}
	})
}

type discardResponse struct{ header http.Header }

func (w discardResponse) Header() http.Header          { return w.header }
func (discardResponse) WriteHeader(int)                {}
func (discardResponse) Write(body []byte) (int, error) { return io.Discard.Write(body) }
func BenchmarkJSONEquivalent(b *testing.B) {
	w := discardResponse{make(http.Header)}
	dto := wireProblem{Type: "about:blank", Status: 404, Code: "not_found", Title: "Resource not found"}
	b.ReportAllocs()
	for b.Loop() {
		body, err := json.Marshal(dto)
		if err != nil {
			b.Fatal(err)
		}
		h := w.Header()
		h.Set("Content-Type", "application/problem+json")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(dto.Status)
		_, _ = w.Write(body)
	}
}
func BenchmarkProblemSimple(b *testing.B) {
	r := newResponder(b, DefaultConfig(), Dependencies{Classify: func(error) Problem { return valid() }})
	w := discardResponse{make(http.Header)}
	original := &panicTextError{}
	b.ReportAllocs()
	for b.Loop() {
		if err := r.Write(context.Background(), w, original); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkFallback(b *testing.B) {
	r := newResponder(b, DefaultConfig(), Dependencies{Classify: func(error) Problem { return Problem{} }})
	w := discardResponse{make(http.Header)}
	original := &panicTextError{}
	b.ReportAllocs()
	for b.Loop() {
		if err := r.Write(context.Background(), w, original); !errors.Is(err, ErrInvalidProblem) {
			b.Fatal(err)
		}
	}
}

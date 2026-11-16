package errors

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequestIDPolicyCaptureAndFallback(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "value"))
	cancel()
	for _, enabled := range []bool{false, true} {
		for _, id := range []string{"", "request-42", strings.Repeat("x", 128), strings.Repeat("x", 129), strings.Repeat("界", 42), "\xff", "a\nb", "a\x7fb", "a\u0085b"} {
			calls := 0
			cfg := Config{IncludeRequestID: enabled, MaxDetailsBytes: 4}
			deps := Dependencies{Classify: func(error) Problem { return valid() }, RequestID: func(got context.Context) string {
				calls++
				if got != ctx || got.Value(key{}) != "value" {
					t.Fatal("context replaced")
				}
				return id
			}}
			r := newResponder(t, cfg, deps)
			cfg.IncludeRequestID = !enabled
			deps.RequestID = func(context.Context) string { panic("mutated dependency used") }
			out, err := r.prepare(ctx, &panicTextError{})
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 0
			if enabled {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatal("request-id count")
			}
			want := id
			if !enabled || len(id) > 128 || id == "\xff" || strings.ContainsAny(id, "\n\x7f\u0085") {
				want = ""
			}
			if out.RequestID != want {
				t.Fatal("ID not omitted/preserved")
			}
		}
	}
	r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { return Problem{} }, RequestID: func(context.Context) string { return "fallback-id" }})
	out, err := r.prepare(ctx, &panicTextError{})
	if !errors.Is(err, ErrInvalidProblem) || out.RequestID != "fallback-id" {
		t.Fatal("fallback id")
	}
}

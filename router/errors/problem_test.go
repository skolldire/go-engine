package errors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type panicTextError struct{}

func (*panicTextError) Error() string { panic("error text inspected") }
func newResponder(t testing.TB, cfg Config, deps Dependencies) *Responder {
	t.Helper()
	r, err := NewResponder(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func valid() Problem { return Problem{Status: 404, Code: "not_found", Title: "Resource not found"} }
func TestProblemValidationFallback(t *testing.T) {
	original := &panicTextError{}
	cases := []Problem{{}, {Status: 399, Code: "ok", Title: "public"}, {Status: 600, Code: "ok", Title: "public"}, {Status: 99999, Code: "ok", Title: "public"}, {Status: 404, Code: strings.Repeat("a", 65), Title: "public"}, {Status: 404, Code: "bad code", Title: "public"}, {Status: 404, Code: "é", Title: "public"}, {Status: 404, Code: "ok", Title: ""}, {Status: 404, Code: "ok", Title: strings.Repeat("a", 257)}, {Status: 404, Code: "ok", Title: "\xff"}}
	for _, p := range cases {
		p.Details = "private detail must disappear"
		calls := 0
		r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(err error) Problem {
			calls++
			if err != original {
				t.Fatal("input changed")
			}
			return p
		}})
		out, err := r.prepare(context.Background(), original)
		if calls != 1 || !errors.Is(err, ErrInvalidProblem) || out.Type != "about:blank" || out.Status != 500 || out.Code != "internal_error" || out.Title != "Internal Server Error" || out.Detail != "" {
			t.Fatal("fallback")
		}
	}
	for _, status := range []int{400, 499, 599} {
		p := valid()
		p.Status = status
		p.Code = strings.Repeat("A", 61) + "_.-"
		p.Title = strings.Repeat("é", 128)
		r := newResponder(t, DefaultConfig(), Dependencies{Classify: func(error) Problem { return p }})
		out, err := r.prepare(context.Background(), original)
		if err != nil || out.Title != p.Title || out.Code != p.Code || out.Status != status {
			t.Fatal("boundary")
		}
	}
}
func TestDetailPolicyAndSchema(t *testing.T) {
	for _, detail := range []string{"", "abcd", "abcde", "界", "界界", "\xff", "\"<&\n"} {
		p := valid()
		p.Details = detail
		r := newResponder(t, Config{MaxDetailsBytes: 4}, Dependencies{Classify: func(error) Problem { return p }})
		out, err := r.prepare(context.Background(), &panicTextError{})
		if err != nil {
			t.Fatal(err)
		}
		want := detail
		if len(detail) > 4 || detail == "\xff" {
			want = ""
		}
		if out.Detail != want {
			t.Fatal("details omission")
		}
		raw, err := json.Marshal(out)
		if err != nil || !json.Valid(raw) {
			t.Fatal("JSON")
		}
		var got map[string]any
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		count := 4
		if want != "" {
			count++
		}
		if len(got) != count || got["type"] != "about:blank" || got["code"] != "not_found" || got["title"] != "Resource not found" || got["status"] != float64(404) {
			t.Fatal("schema")
		}
		if _, ok := got["details"]; ok {
			t.Fatal("plural detail")
		}
		if _, ok := got["instance"]; ok {
			t.Fatal("instance")
		}
	}
}

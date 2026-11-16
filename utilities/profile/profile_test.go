package profile

import (
	"errors"
	"strings"
	"testing"
)

func TestParseScope(t *testing.T) {
	for _, tt := range []struct {
		scope string
		want  Profile
		bad   bool
	}{{"", Local, false}, {"local", Local, false}, {"dev-local", Local, false}, {"api-prod", Prod, false}, {" API-PROD ", Prod, false}, {"contest", "", true}, {"prod-", "", true}, {"a-b-stage", Stage, false}, {"\t ", Local, false}, {"api-test", Test, false}, {"-", "", true}} {
		p, e := ParseScope(tt.scope)
		if p != tt.want || (e != nil) != tt.bad {
			t.Fatalf("scope %q: %q %v", tt.scope, p, e)
		}
		if tt.bad && !errors.Is(e, ErrUnknownProfile) {
			t.Fatal(e)
		}
	}
	for _, p := range []Profile{Local, Test, Stage, Prod, "", "PROD"} {
		if p.String() != string(p) || p.IsLocal() != (p == Local) || p.IsTest() != (p == Test) || p.IsStage() != (p == Stage) || p.IsProd() != (p == Prod) {
			t.Fatal("predicate")
		}
	}
}
func FuzzParseScope(f *testing.F) {
	for _, s := range []string{"", "dev-local", " API-PROD ", "contest", "prod-"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, e := ParseScope(s)
		q, other := ParseScope(s)
		if p != q || (e == nil) != (other == nil) {
			t.Fatal("nondeterministic")
		}
		if e == nil {
			switch p {
			case Local, Test, Stage, Prod:
			default:
				t.Fatal("unknown success")
			}
		} else if p != "" || !errors.Is(e, ErrUnknownProfile) {
			t.Fatal("failure contract")
		}
	})
}
func BenchmarkParseScope(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ParseScope(" API-PROD ")
	}
}
func BenchmarkStdlibToken(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		s := strings.ToLower(strings.TrimSpace(" API-PROD "))
		_ = s[strings.LastIndexByte(s, '-')+1:]
	}
}

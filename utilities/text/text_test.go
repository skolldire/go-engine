package text

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	for _, tt := range []struct {
		s    string
		n    int
		want string
	}{{"abcdef", -1, ""}, {"abcdef", 0, ""}, {"abcdef", 1, "a"}, {"abcdef", 2, "ab"}, {"abcdef", 3, "abc"}, {"abcdef", 4, "a..."}, {"a", 4, "a"}, {"é😊界ab", 4, "é..."}, {"é😊界", 3, "é😊界"}, {"\xffa", 2, "�a"}, {"\xffab", 2, "�a"}, {"", 9, ""}} {
		got := Truncate(tt.s, tt.n)
		if got != tt.want || !utf8.ValidString(got) {
			t.Fatalf("%q/%d: %q", tt.s, tt.n, got)
		}
	}
}
func TestTextOperations(t *testing.T) {
	if got := CleanSpaces(" \t a\u2003\u00a0b\n c "); got != "a b c" {
		t.Fatal(got)
	}
	invalid := "\xffhello\xfe"
	if CleanSpaces(invalid) != invalid || JoinNonEmpty(",", invalid) != invalid || RemoveChars(invalid, "x") != invalid {
		t.Fatal("invalid bytes changed")
	}
	if JoinNonEmpty("/", " ", " a ", "", " b\t") != "a/b" {
		t.Fatal("join")
	}
	if JoinNonEmpty(",") != "" || RemoveChars("abcabc", "ab", "c") != "" || RemoveChars("abc", "") != "abc" {
		t.Fatal("empty/literal removal")
	}
}
func FuzzTruncate(f *testing.F) {
	f.Add("界😊abc", 4)
	f.Add("\xffabc", 2)
	f.Fuzz(func(t *testing.T, s string, n int) {
		if len(s) > 65536 {
			t.Skip()
		}
		got := Truncate(s, n)
		limit := max(n, 0)
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) > limit || got != Truncate(s, n) {
			t.Fatal("rune/determinism invariant")
		}
		runes := []rune(s)
		if n > 3 && len(runes) > n && !strings.HasSuffix(got, "...") {
			t.Fatal("suffix")
		}
	})
}
func FuzzText(f *testing.F) {
	f.Add(" a\u2003b ", "a")
	f.Fuzz(func(t *testing.T, s, remove string) {
		if len(s)+len(remove) > 65536 {
			t.Skip()
		}
		got := CleanSpaces(s)
		if got != CleanSpaces(got) || got != strings.Join(strings.Fields(s), " ") {
			t.Fatal("whitespace invariant")
		}
		if RemoveChars(s, remove) != strings.ReplaceAll(s, remove, "") {
			t.Fatal("literal removal")
		}
	})
}
func BenchmarkCleanSpaces(b *testing.B) {
	s := strings.Repeat(" a\u2003b\t", 20)
	b.ReportAllocs()
	for b.Loop() {
		_ = CleanSpaces(s)
	}
}
func BenchmarkStdlibFieldsJoin(b *testing.B) {
	s := strings.Repeat(" a\u2003b\t", 20)
	b.ReportAllocs()
	for b.Loop() {
		_ = strings.Join(strings.Fields(s), " ")
	}
}
func BenchmarkTruncateUnicode(b *testing.B) {
	s := strings.Repeat("界😊abc", 20)
	b.ReportAllocs()
	for b.Loop() {
		_ = Truncate(s, 32)
	}
}

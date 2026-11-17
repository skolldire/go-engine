package yaml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/skolldire/go-engine/config"
	jsonsource "github.com/skolldire/go-engine/config/json"
)

// wellFormed checks the value domain this source promises: string-keyed
// maps, lists, strings, booleans, nil and valid JSON numbers. It returns
// the number of nodes.
func wellFormed(t *testing.T, v any) int {
	n := 1
	switch x := v.(type) {
	case nil, bool, string:
	case json.Number:
		if !json.Valid([]byte(x)) {
			t.Fatalf("number %q is not valid JSON", x)
		}
	case map[string]any:
		for _, child := range x {
			n += wellFormed(t, child)
		}
	case []any:
		for _, child := range x {
			n += wellFormed(t, child)
		}
	default:
		t.Fatalf("value of type %T escaped the source", v)
	}
	return n
}

func checkDocument(t *testing.T, text string) {
	source := fromText(t, text)
	doc, err := source.Load(context.Background())
	if err != nil {
		var ce *config.ConfigError
		if !errors.As(err, &ce) && !errors.Is(err, config.ErrLimitExceeded) {
			t.Fatalf("unclassified error %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "from yaml:app.yaml") {
			t.Fatalf("error does not name its source: %v", err)
		}
		return
	}
	if doc == nil {
		t.Fatal("nil document without an error")
	}
	if n := wellFormed(t, map[string]any(doc)); n > 2*maxExpandedNodes {
		t.Fatalf("document has %d nodes", n)
	}
	// Loading is deterministic.
	again, err := source.Load(context.Background())
	if err != nil || fmt.Sprint(again) != fmt.Sprint(doc) {
		t.Fatalf("second load differs: %v", err)
	}
}

// FuzzDocument gives the source arbitrary bytes. It must return a document
// in the promised value domain or a classified error, and never panic.
func FuzzDocument(f *testing.F) {
	for _, seed := range []string{
		"", "a: 1\n", "a: {b: [1, 2.5, true, ~, text]}\n", "---\na: 1\n---\nb: 2\n", "a: &a [*a]\n", "a: 1\na: 2\n", "? [k]\n: v\n",
		"a: !!binary aGk=\n", "a: 0x1F\nb: 0o17\nc: .inf\nd: 2001-01-01\n", "base: &b {k: 1}\nc: {<<: *b, k: 2}\n", "a: |\n  text\nb: >-\n  folded\n",
		"a: \"\\u00e9\\x41\"\n", "{a: 1, b: [x, y]}\n", "- 1\n", "a: !custom {k: v}\n", "%YAML 1.2\n---\na: 1\n", "a: 1 # comment\n", "\ufeffa: 1\n", "a: *b\n",
		"a: !!int 0b101\nb: -0o7\nc: +1_0.5e3\n", "<<: {a: 1}\n", "a: &x 1\n*x : 2\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 512 {
			return
		}
		checkDocument(t, string(raw))
	})
}

// FuzzAliases builds nested alias documents of fuzzed shape. Expansion is
// either within the limit or refused; it is never unbounded.
func FuzzAliases(f *testing.F) {
	f.Add(uint8(3), uint8(3), uint8(0), false)
	f.Add(uint8(9), uint8(10), uint8(1), true)
	f.Add(uint8(40), uint8(2), uint8(2), false)
	f.Add(uint8(1), uint8(200), uint8(3), true)
	f.Fuzz(func(t *testing.T, levels, width, shape uint8, recursive bool) {
		levels, width = levels%48, width%24+1
		var b strings.Builder
		b.WriteString("l0: &l0 {k: v}\n")
		for level := 1; level <= int(levels); level++ {
			fmt.Fprintf(&b, "l%d: &l%d ", level, level)
			open, closing := "[", "]"
			if shape%2 == 1 {
				open, closing = "{", "}"
			}
			b.WriteString(open)
			for i := range int(width) {
				if i > 0 {
					b.WriteString(", ")
				}
				switch {
				case shape%2 == 1 && shape%4 == 3 && i == 0:
					fmt.Fprintf(&b, "<<: *l%d", level-1)
				case shape%2 == 1:
					fmt.Fprintf(&b, "k%d: *l%d", i, level-1)
				default:
					fmt.Fprintf(&b, "*l%d", level-1)
				}
			}
			if recursive && level == int(levels) {
				if shape%2 == 1 {
					fmt.Fprintf(&b, ", self: *l%d", level)
				} else {
					fmt.Fprintf(&b, ", *l%d", level)
				}
			}
			b.WriteString(closing + "\n")
		}
		doc, err := parse([]byte(b.String()))
		if err != nil {
			if recursive && levels > 0 && !err.limit && err.reason != "recursive YAML alias" {
				t.Fatalf("unexpected refusal: %+v", err)
			}
			if !recursive && !err.limit {
				t.Fatalf("unexpected refusal: %+v\n%s", err, b.String())
			}
			return
		}
		if recursive && levels > 0 {
			t.Fatalf("a recursive alias was accepted:\n%s", b.String())
		}
		if n := wellFormed(t, map[string]any(doc)); n > 2*maxExpandedNodes {
			t.Fatalf("expanded to %d nodes", n)
		}
	})
}

// FuzzMergeKeys compares merge-key resolution against a direct model:
// explicit keys win, then the first map of the merge list, then the next.
func FuzzMergeKeys(f *testing.F) {
	f.Add([]byte{1, 2, 3}, []byte{2, 3, 4}, []byte{3, 9}, true)
	f.Add([]byte{}, []byte{0}, []byte{0}, false)
	f.Add([]byte{7, 7, 7}, []byte{}, []byte{1, 2, 3, 4, 5, 6}, true)
	f.Fuzz(func(t *testing.T, first, second, explicit []byte, mergeFirst bool) {
		if len(first)+len(second)+len(explicit) > 96 {
			return
		}
		// Each byte names a key (16 possible, so collisions are common) and
		// the value records which map it came from.
		keys := func(raw []byte, origin string) (map[string]string, string) {
			seen := map[string]string{}
			var b strings.Builder
			b.WriteString("{")
			for _, k := range raw {
				name := fmt.Sprintf("k%d", k%16)
				if _, dup := seen[name]; dup {
					continue
				}
				if len(seen) > 0 {
					b.WriteString(", ")
				}
				seen[name] = origin
				fmt.Fprintf(&b, "%s: %s", name, origin)
			}
			b.WriteString("}")
			return seen, b.String()
		}
		a, aText := keys(first, "first")
		b, bText := keys(second, "second")
		own, _ := keys(explicit, "explicit")
		var doc strings.Builder
		fmt.Fprintf(&doc, "a: &a %s\nb: &b %s\nchild:\n", aText, bText)
		if mergeFirst {
			doc.WriteString("  <<: [*a, *b]\n")
		}
		for name := range own {
			fmt.Fprintf(&doc, "  %s: explicit\n", name)
		}
		if !mergeFirst {
			doc.WriteString("  <<: [*a, *b]\n")
		}
		got := mustLoad(t, doc.String())["child"].(map[string]any)
		want := map[string]string{}
		for _, layer := range []map[string]string{b, a, own} {
			for name, origin := range layer {
				want[name] = origin
			}
		}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v\n%s", got, want, doc.String())
		}
		for name, origin := range want {
			if got[name] != origin {
				t.Fatalf("%s: got %v, want %s\n%s", name, got[name], origin, doc.String())
			}
		}
	})
}

func TestConcurrentLoads(t *testing.T) {
	source := fromText(t, "base: &b {host: h, port: 1}\nservices:\n  - {<<: *b, name: a}\n  - {<<: *b, name: b, port: 0x10}\n")
	want := fmt.Sprint(mustLoad(t, "base: &b {host: h, port: 1}\nservices:\n  - {<<: *b, name: a}\n  - {<<: *b, name: b, port: 0x10}\n"))
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				doc, err := source.Load(context.Background())
				if err != nil || fmt.Sprint(doc) != want {
					t.Error(doc, err)
					return
				}
			}
		})
	}
	wg.Wait()
}

// The benchmarks load the same settings from YAML and from JSON.
func benchContent() (yamlText, jsonText string) {
	var y, j strings.Builder
	j.WriteString("{")
	for i := range 40 {
		fmt.Fprintf(&y, "service%d:\n  name: svc-%d\n  port: %d\n  ratio: 0.%d\n  enabled: true\n  timeout: 30s\n  tags: [a, b, c]\n  labels:\n    tier: web\n    zone: z%d\n", i, i, 8000+i, i+1, i)
		if i > 0 {
			j.WriteString(",")
		}
		fmt.Fprintf(&j, `"service%d":{"name":"svc-%d","port":%d,"ratio":0.%d,"enabled":true,"timeout":"30s","tags":["a","b","c"],"labels":{"tier":"web","zone":"z%d"}}`, i, i, 8000+i, i+1, i)
	}
	j.WriteString("}")
	return y.String(), j.String()
}

func TestBenchmarkContentIsEquivalent(t *testing.T) {
	y, j := benchContent()
	fromJSON, err := jsonText(t, j).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(mustLoad(t, y)) != fmt.Sprint(fromJSON) {
		t.Fatal("benchmark inputs are not the same settings")
	}
}

func BenchmarkLoadYAML(b *testing.B) {
	text, _ := benchContent()
	cfg := DefaultConfig()
	cfg.Path = "app.yaml"
	raw := []byte(text)
	source, err := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }})
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := source.Load(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadJSON(b *testing.B) {
	_, text := benchContent()
	cfg := jsonsource.DefaultConfig()
	cfg.Path = "app.json"
	raw := []byte(text)
	source, err := jsonsource.New(cfg, jsonsource.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }})
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := source.Load(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

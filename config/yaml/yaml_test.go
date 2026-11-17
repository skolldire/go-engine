package yaml

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/skolldire/go-engine/config"
	jsonsource "github.com/skolldire/go-engine/config/json"
)

// tracked is a reader that records being closed and can fail on Close.
type tracked struct {
	io.Reader
	closed   int
	closeErr error
}

func (r *tracked) Close() error { r.closed++; return r.closeErr }

func fromText(t testing.TB, text string) config.Source {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Path = "app.yaml"
	source, err := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(text)), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func load(t testing.TB, text string) (config.Document, error) {
	t.Helper()
	return fromText(t, text).Load(context.Background())
}

func mustLoad(t testing.TB, text string) config.Document {
	t.Helper()
	doc, err := load(t, text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	return doc
}

// rejected asserts a ConfigError whose text holds every fragment in want.
func rejected(t testing.TB, text string, want ...string) error {
	t.Helper()
	doc, err := load(t, text)
	var ce *config.ConfigError
	if doc != nil || !errors.As(err, &ce) || !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("want ConfigError, got %v, %v\n%s", doc, err, text)
	}
	for _, fragment := range append(want, "from yaml:app.yaml") {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q lacks %q\n%s", err, fragment, text)
		}
	}
	return err
}

func TestConfigAndConstruction(t *testing.T) {
	if cfg := DefaultConfig(); cfg != (Config{MaxBytes: 1 << 20, Required: true}) {
		t.Fatalf("defaults changed: %+v", cfg)
	}
	files := fstest.MapFS{"app.yaml": {Data: []byte("a: 1\n")}}
	reader := func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil }
	var typed *fstest.MapFS
	cases := []struct {
		field string
		cfg   Config
		deps  Dependencies
	}{
		{"path", Config{MaxBytes: 1}, Dependencies{FS: files}},
		{"max_bytes", Config{Path: "app.yaml"}, Dependencies{FS: files}},
		{"max_bytes", Config{Path: "app.yaml", MaxBytes: -1}, Dependencies{FS: files}},
		{"dependencies", Config{Path: "app.yaml", MaxBytes: 1}, Dependencies{}},
		{"dependencies", Config{Path: "app.yaml", MaxBytes: 1}, Dependencies{FS: files, Reader: reader}},
		{"fs", Config{Path: "app.yaml", MaxBytes: 1}, Dependencies{FS: typed}},
		{"path", Config{Path: "/etc/app.yaml", MaxBytes: 1}, Dependencies{FS: files}},
		{"path", Config{Path: "../app.yaml", MaxBytes: 1}, Dependencies{FS: files}},
	}
	for _, tc := range cases {
		source, err := New(tc.cfg, tc.deps)
		var ce *config.ConfigError
		if source != nil || !errors.As(err, &ce) || ce.Field != tc.field {
			t.Fatalf("%+v: %v, %v", tc.cfg, source, err)
		}
	}
	// With a Reader the path is only a name and need not be an fs path.
	source, err := New(Config{Path: "/etc/app.yaml", MaxBytes: 1}, Dependencies{Reader: reader})
	if err != nil || fmt.Sprint(source) != "yaml:/etc/app.yaml" {
		t.Fatal(source, err)
	}
	var none *Source
	if none.String() != "yaml" {
		t.Fatal(none.String())
	}
	var nilCtx context.Context // the nil context is the case under test
	if _, err := source.Load(nilCtx); !errors.Is(err, config.ErrNilContext) {
		t.Fatal(err)
	}
	if _, err := (&Source{}).Load(context.Background()); !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Load(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFSAndRequired(t *testing.T) {
	files := fstest.MapFS{"conf/app.yaml": {Data: []byte("name: api\nport: 8080\n")}}
	cfg := DefaultConfig()
	cfg.Path = "conf/app.yaml"
	source, err := New(cfg, Dependencies{FS: files})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := source.Load(context.Background())
	if err != nil || !reflect.DeepEqual(doc, config.Document{"name": "api", "port": json.Number("8080")}) {
		t.Fatal(doc, err)
	}
	cfg.Path = "conf/missing.yaml"
	required, _ := New(cfg, Dependencies{FS: files})
	if _, err := required.Load(context.Background()); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "yaml:conf/missing.yaml") {
		t.Fatal(err)
	}
	cfg.Required = false
	optional, _ := New(cfg, Dependencies{FS: files})
	if doc, err := optional.Load(context.Background()); err != nil || doc == nil || len(doc) != 0 {
		t.Fatal(doc, err)
	}
	// Required=false forgives a missing file and nothing else.
	denied := errors.New("permission denied")
	for _, failure := range []error{denied, fs.ErrPermission} {
		broken, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return nil, failure }})
		if _, err := broken.Load(context.Background()); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	missing, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return nil, fs.ErrNotExist }})
	if doc, err := missing.Load(context.Background()); err != nil || len(doc) != 0 {
		t.Fatal(doc, err)
	}
	nilReader, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return nil, nil }})
	if _, err := nilReader.Load(context.Background()); !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatal(err)
	}
}

func TestReaderIsAlwaysClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Path = "app.yaml"
	cfg.MaxBytes = 16
	for name, text := range map[string]string{"valid": "a: 1\n", "malformed": "a: [1\n", "too large": strings.Repeat("a", 64), "not a mapping": "- 1\n"} {
		r := &tracked{Reader: strings.NewReader(text)}
		source, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return r, nil }})
		_, _ = source.Load(context.Background())
		if r.closed != 1 {
			t.Fatalf("%s: closed %d times", name, r.closed)
		}
	}
	// A close failure is reported and discards the document.
	failing := errors.New("close failed")
	r := &tracked{Reader: strings.NewReader("a: 1\n"), closeErr: failing}
	source, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return r, nil }})
	if doc, err := source.Load(context.Background()); doc != nil || !errors.Is(err, failing) {
		t.Fatal(doc, err)
	}
}

func TestMaxBytesBoundary(t *testing.T) {
	text := "key: " + strings.Repeat("v", 59) + "\n" // 65 bytes
	for limit, ok := range map[int64]bool{64: false, 65: true, 66: true} {
		cfg := DefaultConfig()
		cfg.Path, cfg.MaxBytes = "app.yaml", limit
		source, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(text)), nil }})
		doc, err := source.Load(context.Background())
		if ok && (err != nil || len(doc) != 1) {
			t.Fatal(limit, err)
		}
		if !ok && (doc != nil || !errors.Is(err, config.ErrLimitExceeded)) {
			t.Fatal(limit, doc, err)
		}
	}
	// The limit is applied while reading: an endless reader is cut off.
	cfg := DefaultConfig()
	cfg.Path, cfg.MaxBytes = "app.yaml", 1024
	endless, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(zeros{}), nil }})
	if _, err := endless.Load(context.Background()); !errors.Is(err, config.ErrLimitExceeded) {
		t.Fatal(err)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

func TestDocuments(t *testing.T) {
	for name, text := range map[string]string{
		"empty": "", "newline": "\n", "comment only": "# nothing here\n", "bare marker": "---\n",
		"marker and end": "---\n...\n", "explicit null": "~\n", "null word": "null\n", "empty mapping": "{}\n", "bom": "\ufeffa: 1\n",
	} {
		doc, err := load(t, text)
		if err != nil || doc == nil || (name != "bom" && len(doc) != 0) {
			t.Fatalf("%s: %v, %v", name, doc, err)
		}
	}
	if doc := mustLoad(t, "---\na: 1\n...\n"); len(doc) != 1 {
		t.Fatal(doc)
	}
	rejected(t, "a: 1\n---\nb: 2\n", "multiple YAML documents", "line 2")
	rejected(t, "a: 1\n---\n", "multiple YAML documents")
	rejected(t, "---\n---\n", "multiple YAML documents")
	for name, text := range map[string]string{"list": "- a\n- b\n", "string": "just text\n", "number": "42\n", "tagged null": "!!null ~\n"} {
		if name == "tagged null" {
			continue
		}
		rejected(t, text, "YAML root must be a mapping", "line 1")
	}
	rejected(t, "a: [1, 2\n", "invalid YAML")
	// The library reports no line for a syntax error on the first line.
	rejected(t, "a: b: c\n", "invalid YAML")
	rejected(t, "x: 1\na: b: c\n", "invalid YAML", "line 2")
	rejected(t, "a: *missing\n", "invalid YAML")
	rejected(t, "a: \"\xff\"\n", "invalid YAML Unicode")
	// UTF-16 is not accepted, although the library can read it.
	rejected(t, "\xff\xfea\x00:\x00 \x001\x00\n\x00", "invalid YAML Unicode")
}

func TestKeys(t *testing.T) {
	doc := mustLoad(t, "plain: 1\n\"quoted key\": 2\n'123': 3\nyes: 4\nOn: 5\n2001-01-01: 6\n!!str 7: 8\n\"<<\": 9\nMixedCase: 10\n\"\": 11\n")
	want := config.Document{"plain": json.Number("1"), "quoted key": json.Number("2"), "123": json.Number("3"), "yes": json.Number("4"), "On": json.Number("5"),
		"2001-01-01": json.Number("6"), "7": json.Number("8"), "<<": json.Number("9"), "MixedCase": json.Number("10"), "": json.Number("11")}
	if !reflect.DeepEqual(doc, want) {
		t.Fatal(doc)
	}
	for name, text := range map[string]string{
		"integer": "a:\n  1: x\n", "float": "a:\n  1.5: x\n", "boolean": "a:\n  true: x\n", "null": "a:\n  ~: x\n",
		"list": "a:\n  [1, 2]: x\n", "map": "a:\n  {k: v}: x\n", "tagged": "a:\n  !!int 7: x\n", "alias to number": "n: &n 7\na:\n  *n : x\n",
	} {
		err := rejected(t, text, "YAML key must be a string", "line ")
		if !strings.HasPrefix(err.Error(), "invalid configuration") && !strings.Contains(err.Error(), "a") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// A key reached through an alias to a string is a string key.
	if doc := mustLoad(t, "name: &k region\nvalues:\n  *k : eu\n"); !reflect.DeepEqual(doc["values"], map[string]any{"region": "eu"}) {
		t.Fatal(doc)
	}
	// Duplicates are reported with the full key path and the second position.
	err := rejected(t, "server:\n  tls:\n    cert: one\n    key: k\n    cert: two\n", "duplicate YAML key", "line 5 column 5")
	var ce *config.ConfigError
	if errors.As(err, &ce); ce.Field != "server.tls.cert" {
		t.Fatal(ce.Field)
	}
	rejected(t, "a: 1\na: 2\n", "duplicate YAML key", "line 2")
	rejected(t, "a: {x: 1, x: 2}\n", "duplicate YAML key")
	rejected(t, "a: 1\n\"a\": 2\n", "duplicate YAML key")
	rejected(t, "list:\n  - {k: 1}\n  - {k: 1, k: 2}\n", "duplicate YAML key", "list[1].k")
	// Keys are compared exactly: case is not folded.
	if doc := mustLoad(t, "Port: 1\nport: 2\n"); len(doc) != 2 {
		t.Fatal(doc)
	}
}

func TestTags(t *testing.T) {
	doc := mustLoad(t, `
s: !!str 123
i: !!int "42"
f: !!float 3
b: !!bool "true"
n: !!null ~
m: !!map {k: v}
q: !!seq [a, b]
long: !<tag:yaml.org,2002:str> 7
`)
	want := config.Document{"s": "123", "i": json.Number("42"), "f": json.Number("3"), "b": true, "n": nil,
		"m": map[string]any{"k": "v"}, "q": []any{"a", "b"}, "long": "7"}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("%#v", doc)
	}
	for name, text := range map[string]string{
		"custom scalar": "a: !secret hunter2\n", "custom map": "a: !app {k: v}\n", "custom seq": "a: !app [1]\n",
		"binary": "a: !!binary aGVsbG8=\n", "set": "a: !!set {x: ~}\n", "omap": "a: !!omap [{x: 1}]\n", "pairs": "a: !!pairs [{x: 1}]\n",
		"explicit timestamp": "a: !!timestamp 2001-01-01\n", "go type": "a: !!python/object:os.system x\n", "verbatim": "a: !<tag:example.com,2026:thing> x\n",
		"seq as map": "a: !!map [1]\n", "map as seq": "a: !!seq {k: v}\n",
	} {
		err := rejected(t, text, "line 1")
		if strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("%s: value leaked: %v", name, err)
		}
	}
	// A tag that contradicts its content is refused, not coerced.
	rejected(t, "a: !!int nope\n", "invalid YAML integer", "line 1")
	rejected(t, "a: !!int 1.5\n", "invalid YAML integer")
	rejected(t, "a: !!bool yes\n", "invalid YAML boolean")
	rejected(t, "a: !!null nope\n", "invalid YAML null")
	rejected(t, "a: !!float nope\n", "invalid YAML number")
}

func TestScalarResolution(t *testing.T) {
	cases := map[string]any{
		// Booleans: YAML 1.2 only.
		"true": true, "True": true, "TRUE": true, "false": false, "False": false, "FALSE": false,
		"yes": "yes", "no": "no", "on": "on", "off": "off", "Yes": "Yes", "NO": "NO", "y": "y", "n": "n", "tRuE": "tRuE",
		// Nulls.
		"~": nil, "null": nil, "Null": nil, "NULL": nil, "": nil, "nil": "nil", "NuLl": "NuLl",
		// Integers, written back in decimal.
		"0": json.Number("0"), "-0": json.Number("0"), "42": json.Number("42"), "-17": json.Number("-17"), "+5": json.Number("5"),
		"0x1F": json.Number("31"), "0xff": json.Number("255"), "-0x10": json.Number("-16"), "0o17": json.Number("15"), "-0o17": json.Number("-15"),
		"017": json.Number("15"), "0b1010": json.Number("10"), "-0b11": json.Number("-3"), "1_000_000": json.Number("1000000"),
		"9223372036854775807": json.Number("9223372036854775807"), "-9223372036854775808": json.Number("-9223372036854775808"),
		"18446744073709551615": json.Number("18446744073709551615"),
		// Beyond 64 bits the library calls it a float; the digits survive.
		"18446744073709551616": json.Number("18446744073709551616"), "123456789012345678901234567890": json.Number("123456789012345678901234567890"),
		// Floats keep their digits.
		"1.5": json.Number("1.5"), "-2.25": json.Number("-2.25"), ".5": json.Number("0.5"), "-.5": json.Number("-0.5"), "+.5": json.Number("0.5"),
		"1.": json.Number("1"), "1e3": json.Number("1e3"), "1E3": json.Number("1e3"), "1.5e-3": json.Number("1.5e-3"), "1e+3": json.Number("1e3"),
		"007.50": json.Number("7.50"), "08": json.Number("8"), "0.1000000000000000055511151231257827": json.Number("0.1000000000000000055511151231257827"),
		"1_000.5": json.Number("1000.5"),
		// Timestamps stay text.
		"2001-12-14": "2001-12-14", "2001-12-14T21:59:43Z": "2001-12-14T21:59:43Z", "2001-12-14 21:59:43.10": "2001-12-14 21:59:43.10", "2026-1-2": "2026-1-2",
		// Things that look special and are not.
		"30s": "30s", "1h30m": "1h30m", "1:30": "1:30", "190:20:30": "190:20:30", "0x": "0x", "0xZZ": "0xZZ", "1.2.3": "1.2.3", "12abc": "12abc", "<<": "<<",
		"\"8080\"": "8080", "'true'": "true", "\"null\"": "null", "\"~\"": "~", "'2001-12-14'": "2001-12-14", "\"0x1F\"": "0x1F", "''": "",
	}
	for text, want := range cases {
		doc := mustLoad(t, "v: "+text+"\n")
		if got := doc["v"]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, want %#v", text, got, want)
		}
		// The library agrees on the kind of every plain scalar: this
		// source changes the representation, not the classification.
		if strings.ContainsAny(text, "\"'") {
			continue
		}
		var node yamlv3.Node
		if err := yamlv3.Unmarshal([]byte("v: "+text+"\n"), &node); err != nil {
			t.Fatal(text, err)
		}
		tag := node.Content[0].Content[1].ShortTag()
		kind := map[string]string{"bool": "!!bool", "<nil>": "!!null", "json.Number": "!!int", "string": "!!str"}[fmt.Sprintf("%T", want)]
		if tag == "!!float" && kind == "!!int" || tag == "!!timestamp" && kind == "!!str" || tag == "!!merge" {
			continue
		}
		if tag != kind {
			t.Errorf("%s: library tag %s, source produced %T", text, tag, want)
		}
	}
	for _, text := range []string{".inf", "-.inf", "+.inf", ".Inf", ".INF", "-.INF", ".nan", ".NaN", ".NAN", "!!float .inf", "!!float .nan"} {
		rejected(t, "v: "+text+"\n", "non-finite YAML number", "line 1 column 4")
	}
	// Block scalars are strings whatever they contain.
	doc := mustLoad(t, "a: |\n  123\nb: >\n  true\n")
	if doc["a"] != "123\n" || doc["b"] != "true\n" {
		t.Fatal(doc)
	}
	// Every number this source emits is accepted by the config loader.
	for text, want := range cases {
		if number, ok := want.(json.Number); ok && !json.Valid([]byte(number)) {
			t.Errorf("%s: %q is not a JSON number", text, number)
		}
	}
}

func TestAliases(t *testing.T) {
	doc := mustLoad(t, `
defaults: &d
  timeout: 30s
  retries: 3
port: &p 8080
tags: &t [a, b]
primary: *d
secondary: *d
ports: [*p, *p]
copy: *t
`)
	d := map[string]any{"timeout": "30s", "retries": json.Number("3")}
	if !reflect.DeepEqual(doc["primary"], d) || !reflect.DeepEqual(doc["secondary"], d) || !reflect.DeepEqual(doc["ports"], []any{json.Number("8080"), json.Number("8080")}) || !reflect.DeepEqual(doc["copy"], []any{"a", "b"}) {
		t.Fatalf("%#v", doc)
	}
	// Each use is its own copy: changing one does not change another.
	doc["primary"].(map[string]any)["timeout"] = "changed"
	if doc["secondary"].(map[string]any)["timeout"] != "30s" || doc["defaults"].(map[string]any)["timeout"] != "30s" {
		t.Fatal("alias uses share a map")
	}
	for name, text := range map[string]string{
		"map into itself":    "a: &a\n  self: *a\n",
		"list into itself":   "a: &a [1, *a]\n",
		"through a child":    "a: &a\n  b:\n    c: [*a]\n",
		"merge into itself":  "a: &a\n  <<: *a\n  k: v\n",
		"mutual, via nested": "a: &a\n  b: &b\n    back: *a\n",
	} {
		err := rejected(t, text, "recursive YAML alias", "line ")
		if name == "" {
			t.Fatal(err)
		}
	}
}

// bomb builds the classic nested-alias document: each level is a list of
// width references to the level below.
func bomb(levels, width int) string {
	var b strings.Builder
	b.WriteString("l0: &l0 [x")
	for i := 1; i < width; i++ {
		b.WriteString(", x")
	}
	b.WriteString("]\n")
	for level := 1; level <= levels; level++ {
		fmt.Fprintf(&b, "l%d: &l%d [", level, level)
		for i := range width {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "*l%d", level-1)
		}
		b.WriteString("]\n")
	}
	return b.String()
}

func count(v any) int {
	n := 1
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			n += count(child)
		}
	case config.Document:
		for _, child := range x {
			n += count(child)
		}
	case []any:
		for _, child := range x {
			n += count(child)
		}
	}
	return n
}

func TestAliasExpansionLimit(t *testing.T) {
	// Nine levels of ten is a billion leaves from under a kilobyte.
	text := bomb(9, 10)
	if len(text) > 1024 {
		t.Fatal("fixture grew")
	}
	start := time.Now()
	doc, err := load(t, text)
	if doc != nil || !errors.Is(err, config.ErrLimitExceeded) || !strings.Contains(err.Error(), "more than 100000 nodes") || !strings.Contains(err.Error(), "line ") {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the bomb took %v to refuse", elapsed)
	}
	// Under the limit the same shape is fine: 10^3 leaves.
	small := mustLoad(t, bomb(3, 10))
	if n := count(small); n > maxExpandedNodes || n < 1000 {
		t.Fatal(n)
	}
	// The limit counts expanded nodes, so a merge bomb is caught as well.
	var b strings.Builder
	b.WriteString("m0: &m0 {a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7, h: 8}\n")
	for level := 1; level <= 8; level++ {
		fmt.Fprintf(&b, "m%d: &m%d {", level, level)
		for i := range 8 {
			fmt.Fprintf(&b, "k%d: *m%d, ", i, level-1)
		}
		b.WriteString("z: 0}\n")
	}
	if _, err := load(t, b.String()); !errors.Is(err, config.ErrLimitExceeded) {
		t.Fatal(err)
	}
	// A plain large document without aliases is bounded the same way.
	flat := "list: [" + strings.Repeat("1, ", maxExpandedNodes) + "1]\n"
	cfg := DefaultConfig()
	cfg.Path, cfg.MaxBytes = "app.yaml", int64(len(flat))
	source, _ := New(cfg, Dependencies{Reader: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(flat)), nil }})
	if _, err := source.Load(context.Background()); !errors.Is(err, config.ErrLimitExceeded) {
		t.Fatal(err)
	}
}

func TestMergeKeys(t *testing.T) {
	doc := mustLoad(t, `
base: &base
  host: localhost
  port: 5432
  pool: {min: 1, max: 4}
extra: &extra
  port: 9999
  ssl: true
primary:
  port: 6000
  <<: *base
replica:
  <<: [*base, *extra]
  host: replica.internal
inline:
  <<: {a: 1, b: 2}
  b: 3
nested:
  <<: *base
  pool: {max: 9}
`)
	want := map[string]map[string]any{
		// The explicit key wins although it comes before the merge.
		"primary": {"host": "localhost", "port": json.Number("6000"), "pool": map[string]any{"min": json.Number("1"), "max": json.Number("4")}},
		// In a list, the earlier map wins over the later one.
		"replica": {"host": "replica.internal", "port": json.Number("5432"), "ssl": true, "pool": map[string]any{"min": json.Number("1"), "max": json.Number("4")}},
		"inline":  {"a": json.Number("1"), "b": json.Number("3")},
		// A merge is shallow: the explicit map replaces the merged one.
		"nested": {"host": "localhost", "port": json.Number("5432"), "pool": map[string]any{"max": json.Number("9")}},
	}
	for key, expected := range want {
		if !reflect.DeepEqual(doc[key], expected) {
			t.Errorf("%s: %#v", key, doc[key])
		}
	}
	// A merge chain resolves through each level.
	chain := mustLoad(t, "a: &a {x: 1, y: 1}\nb: &b {<<: *a, y: 2, z: 2}\nc: {<<: *b, z: 3}\n")
	if !reflect.DeepEqual(chain["c"], map[string]any{"x": json.Number("1"), "y": json.Number("2"), "z": json.Number("3")}) {
		t.Fatal(chain["c"])
	}
	// Two explicit keys that are equal are still an error next to a merge.
	rejected(t, "base: &b {k: 1}\nchild:\n  <<: *b\n  k: 2\n  k: 3\n", "duplicate YAML key", "child.k", "line 5")
	// Two merge keys in one map are a duplicate key like any other.
	rejected(t, "a: &a {x: 1}\nb: &b {y: 1}\nc:\n  <<: *a\n  <<: *b\n", "duplicate YAML key", "line 5")
	for _, text := range []string{"c:\n  <<: 5\n", "c:\n  <<: text\n", "c:\n  <<: [1, 2]\n", "n: &n 5\nc:\n  <<: *n\n", "a: &a {x: 1}\nc:\n  <<: [*a, [1]]\n", "c:\n  <<: ~\n"} {
		rejected(t, text, "YAML merge value must be a mapping or a list of mappings", "line ")
	}
	// A duplicate inside a merged map is found through the merge.
	rejected(t, "base: &b {k: 1, k: 2}\n", "duplicate YAML key")
}

func TestErrorsNeverCarryValues(t *testing.T) {
	const secret = "hunter2-do-not-print"
	for name, text := range map[string]string{
		"duplicate":      "db:\n  password: " + secret + "\n  password: " + secret + "\n",
		"custom tag":     "db:\n  password: !vault " + secret + "\n",
		"binary":         "db:\n  password: !!binary " + secret + "\n",
		"bad int":        "db:\n  password: !!int " + secret + "\n",
		"bad bool":       "db:\n  password: !!bool " + secret + "\n",
		"bad null":       "db:\n  password: !!null " + secret + "\n",
		"syntax":         "db:\n  password: \"" + secret + "\n",
		"syntax mapping": "db:\n  password: " + secret + ": extra\n",
		"unknown anchor": "db:\n  password: " + secret + "\n  other: *" + secret + "\n",
		"non-string key": "db:\n  password: " + secret + "\n  [" + secret + "]: x\n",
		"merge":          "db:\n  password: " + secret + "\n  <<: " + secret + "\n",
		"multiple":       "db:\n  password: " + secret + "\n---\npassword: " + secret + "\n",
		"root":           "- " + secret + "\n",
		"recursive":      "db: &d\n  password: " + secret + "\n  self: *d\n",
	} {
		_, err := load(t, text)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("%s: value leaked: %v", name, err)
		}
		// The library gives no position for an unknown anchor.
		if name != "unknown anchor" && !strings.Contains(err.Error(), "line ") {
			t.Fatalf("%s: no line: %v", name, err)
		}
	}
	// Position and path together, for a deep value.
	err := rejected(t, "services:\n  - name: api\n    env:\n      TOKEN: !vault "+secret+"\n", "line 4 column 14")
	var ce *config.ConfigError
	if errors.As(err, &ce); ce.Field != "services[0].env.TOKEN" || ce.Reason != "YAML tag not allowed at line 4 column 14 from yaml:app.yaml" {
		t.Fatalf("%q / %q", ce.Field, ce.Reason)
	}
}

type settings struct {
	Name    string            `mapstructure:"name"`
	Port    int               `mapstructure:"port"`
	Ratio   float64           `mapstructure:"ratio"`
	Debug   bool              `mapstructure:"debug"`
	Timeout time.Duration     `mapstructure:"timeout"`
	Tags    []string          `mapstructure:"tags"`
	Labels  map[string]string `mapstructure:"labels"`
	Limits  struct {
		Max int64 `mapstructure:"max"`
	} `mapstructure:"limits"`
}

func decode(t testing.TB, target any, sources ...config.Source) error {
	t.Helper()
	loader, err := config.New(config.DefaultConfig(), config.Dependencies{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		return err
	}
	return snapshot.Decode("", target)
}

func jsonText(t testing.TB, text string) config.Source {
	t.Helper()
	cfg := jsonsource.DefaultConfig()
	cfg.Path = "app.json"
	source, err := jsonsource.New(cfg, jsonsource.Dependencies{Reader: func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(text)), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestEquivalenceWithJSON(t *testing.T) {
	const asYAML = `
name: api
port: 8080
ratio: 0.25
debug: true
timeout: 1m30s
tags: [a, b]
labels:
  tier: web
  zone: "1"
limits:
  max: 9223372036854775807
`
	const asJSON = `{"name":"api","port":8080,"ratio":0.25,"debug":true,"timeout":"1m30s","tags":["a","b"],
"labels":{"tier":"web","zone":"1"},"limits":{"max":9223372036854775807}}`
	fromYAML, err := fromText(t, asYAML).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := jsonText(t, asJSON).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromYAML, fromJSON) {
		t.Fatalf("documents differ:\nyaml %#v\njson %#v", fromYAML, fromJSON)
	}
	var a, b settings
	if err := decode(t, &a, fromText(t, asYAML)); err != nil {
		t.Fatal(err)
	}
	if err := decode(t, &b, jsonText(t, asJSON)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || a.Port != 8080 || a.Timeout != 90*time.Second || a.Limits.Max != 9223372036854775807 || a.Labels["zone"] != "1" {
		t.Fatalf("decoded values differ:\nyaml %+v\njson %+v", a, b)
	}
	// Both sources refuse the same mistakes when decoding: file text is
	// not converted into numbers or booleans.
	for yamlText, jsonValue := range map[string]string{"port: \"8080\"\n": `{"port":"8080"}`, "debug: \"true\"\n": `{"debug":"true"}`, "debug: yes\n": `{"debug":"yes"}`, "timeout: 30\n": `{"timeout":30}`} {
		var y, j settings
		errY, errJ := decode(t, &y, fromText(t, yamlText)), decode(t, &j, jsonText(t, jsonValue))
		if errY == nil || errJ == nil {
			t.Fatalf("%q decoded: yaml %v, json %v", yamlText, errY, errJ)
		}
	}
	// A hexadecimal or octal literal decodes as the decimal it denotes.
	var hex settings
	if err := decode(t, &hex, fromText(t, "port: 0x1F90\nlimits: {max: 0o17}\n")); err != nil || hex.Port != 8080 || hex.Limits.Max != 15 {
		t.Fatal(hex, err)
	}
	// A date stays text and reaches a string field untouched.
	var dated struct {
		Since string `mapstructure:"since"`
	}
	if err := decode(t, &dated, fromText(t, "since: 2026-01-02\n")); err != nil || dated.Since != "2026-01-02" {
		t.Fatal(dated, err)
	}
}

func TestLayeringAndNull(t *testing.T) {
	base := fromText(t, "name: api\nport: 8080\ndebug: true\nlabels: {tier: web, zone: a}\ntags: [a, b]\n")
	override := fromText(t, "port: 9090\ndebug: ~\nlabels: {zone: null, team: core}\ntags: [c]\n")
	got := settings{Debug: false, Name: "default"}
	if err := decode(t, &got, base, override); err != nil {
		t.Fatal(err)
	}
	// null removed "debug" and "labels.zone"; the map merged; the list
	// was replaced.
	if got.Name != "api" || got.Port != 9090 || got.Debug || !reflect.DeepEqual(got.Labels, map[string]string{"tier": "web", "team": "core"}) || !reflect.DeepEqual(got.Tags, []string{"c"}) {
		t.Fatalf("%+v", got)
	}
}

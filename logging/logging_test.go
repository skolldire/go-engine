package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	logging "github.com/skolldire/go-engine/logging"
)

func newLogger(t testing.TB, cfg logging.Config, deps logging.Dependencies) *slog.Logger {
	t.Helper()
	l, err := logging.New(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func record(t testing.TB, buf *bytes.Buffer) map[string]any {
	t.Helper()
	if !json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("invalid JSON: %q", buf.String())
	}
	var r map[string]any
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func object(t testing.TB, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not object: %#v", v)
	}
	return m
}
func TestFormatsAndLevels(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		for _, level := range []string{"debug", "info", "warn", "error"} {
			t.Run(format+"/"+level, func(t *testing.T) {
				cfg := logging.DefaultConfig()
				cfg.Format = format
				cfg.Level = level
				var b bytes.Buffer
				l := newLogger(t, cfg, logging.Dependencies{Output: &b})
				for _, lev := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
					l.Log(context.Background(), lev, "message", "key", "value")
				}
				expected := map[string]int{"debug": 4, "info": 3, "warn": 2, "error": 1}[level]
				if strings.Count(b.String(), "\n") != expected {
					t.Fatal(b.String())
				}
				if format == "json" {
					for _, line := range bytes.Split(bytes.TrimSpace(b.Bytes()), []byte("\n")) {
						if !json.Valid(line) {
							t.Fatal(string(line))
						}
					}
				}
			})
		}
	}
}

type logValue struct {
	calls *atomic.Int64
	value slog.Value
}

func (v logValue) LogValue() slog.Value { v.calls.Add(1); return v.value }
func TestMaterializationAndDisabledRecords(t *testing.T) {
	var b bytes.Buffer
	var calls atomic.Int64
	source := map[string]any{"value": []any{"before"}, "password": "hidden"}
	attrs := []slog.Attr{slog.Any("base", source)}
	cfg := logging.DefaultConfig()
	l := newLogger(t, cfg, logging.Dependencies{Output: &b, Attrs: attrs})
	attrs[0] = slog.String("replaced", "bad")
	source["value"].([]any)[0] = "after"
	cfg.RedactKeys[0] = "changed"
	payload := map[string]any{"message": "before"}
	derived := l.With("fixed", payload, "lazy", logValue{&calls, slog.StringValue("resolved")})
	payload["message"] = "after"
	if calls.Load() != 1 {
		t.Fatal("With must resolve once")
	}
	derived.Debug("skip", "value", logValue{&calls, slog.StringValue("disabled")})
	if calls.Load() != 1 || b.Len() != 0 {
		t.Fatal("disabled record evaluated")
	}
	derived.Info("emit", "event", logValue{&calls, slog.StringValue("event")})
	r := record(t, &b)
	if object(t, r["base"])["password"] != "[REDACTED]" || object(t, r["base"])["value"].([]any)[0] != "before" || object(t, r["fixed"])["message"] != "before" || calls.Load() != 2 {
		t.Fatalf("materialization: %#v calls=%d", r, calls.Load())
	}
	b.Reset()
	l.Info("base")
	if _, ok := record(t, &b)["fixed"]; ok {
		t.Fatal("derived attrs contaminated parent")
	}
}
func TestGroupsAndRedactor(t *testing.T) {
	var b bytes.Buffer
	var seen []string
	redact := func(groups []string, a slog.Attr) slog.Attr {
		seen = append(seen, strings.Join(groups, "/")+":"+a.Key)
		if len(groups) > 0 {
			groups[0] = "mutated"
		}
		switch a.Key {
		case "drop":
			return slog.Attr{}
		case "replace":
			return slog.String("TOKEN", "hidden")
		case "expand":
			return slog.Group("made", slog.String("secret", "hidden"), slog.Int("child", 1))
		}
		return a
	}
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b, Redactor: redact})
	l.WithGroup("outer").With("fixed", 1).LogAttrs(context.Background(), slog.LevelInfo, "groups", slog.Group("inner", slog.String("replace", "x"), slog.Int("keep", 2)), slog.Any("map", map[string]any{"secret": "hidden", "ordinary": 3}), slog.String("drop", "x"), slog.String("expand", "x"))
	outer := object(t, record(t, &b)["outer"])
	if object(t, outer["inner"])["TOKEN"] != "[REDACTED]" || object(t, outer["map"])["secret"] != "[REDACTED]" || object(t, outer["made"])["secret"] != "[REDACTED]" {
		t.Fatal(outer)
	}
	if _, ok := outer["drop"]; ok {
		t.Fatal("not dropped")
	}
	expected := []string{"outer:fixed", "outer/inner:replace", "outer/inner:keep", "outer:map", "outer:drop", "outer:expand", "outer/made:secret", "outer/made:child"}
	if !reflect.DeepEqual(seen, expected) {
		t.Fatalf("redactor paths: %v", seen)
	}
}
func TestSensitiveGroupAndPayloadNotTraversed(t *testing.T) {
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	l.LogAttrs(context.Background(), slog.LevelInfo, "record", slog.Any("password", struct{ Child panicString }{panicString{}}), slog.Group("TOKEN", slog.Any("child", panicString{})))
	r := record(t, &b)
	if r["password"] != "[REDACTED]" || r["TOKEN"] != "[REDACTED]" {
		t.Fatal(r)
	}
	b.Reset()
	l.WithGroup("secret").With("child", panicString{}).WithGroup("nested").Info("safe", "bad", panicString{})
	if record(t, &b)["secret"] != "[REDACTED]" {
		t.Fatal(b.String())
	}
}

type panicString struct{}

func (panicString) String() string { panic("caller string panic") }

type serialized string

func (s serialized) MarshalJSON() ([]byte, error) { return []byte(s), nil }

type failedJSON struct{}

func (failedJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("must-not-be-printed") }

type namedKey string
type textValue string

func (s textValue) MarshalText() ([]byte, error) { return []byte(s), nil }
func TestStructuredTypes(t *testing.T) {
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	type Embedded struct{ Secret string }
	type Container struct {
		Embedded
		Password string `json:"pwd"`
		Omitted  string `json:"-"`
		Name     string `json:"alias,omitempty,string"`
		private  string
	}
	l.Info("types",
		"struct", Container{Embedded{"hidden"}, "hidden", "hidden", "kept", "hidden"},
		"json", serialized(`{"password":"hidden","number":9007199254740993}`),
		"hidden", serialized(`{"safe":"not-a-secret"}`),
		"bad", failedJSON{}, "extra", serialized(`{} {}`),
		"map", map[namedKey]any{"Token": "hidden", "A": 1},
		"ints", map[int]any{-3: "negative", 10: "ten"},
		"badmap", map[bool]string{true: "hidden"}, "fn", func() {}, "ch", make(chan int),
		"nil", (*int)(nil), "bytes", []byte("abc"), "text", textValue("plain"),
		"error", errors.New("opaque password=visible"), "nested", []any{map[string]any{"authorization": "hidden"}},
	)
	r := record(t, &b)
	s := object(t, r["struct"])
	if s["pwd"] != "[REDACTED]" || object(t, s["Embedded"])["Secret"] != "[REDACTED]" || s["alias"] != "kept" {
		t.Fatal(s)
	}
	if _, ok := s["Omitted"]; ok {
		t.Fatal("json - not omitted")
	}
	if _, ok := s["private"]; ok {
		t.Fatal("private field exposed")
	}
	if object(t, r["json"])["password"] != "[REDACTED]" || !bytes.Contains(b.Bytes(), []byte("9007199254740993")) {
		t.Fatal(b.String())
	}
	if r["bad"] != "[UNSERIALIZABLE]" || r["extra"] != "[UNSERIALIZABLE]" || r["badmap"] != "[UNSUPPORTED]" || r["fn"] != "[UNSUPPORTED]" || r["ch"] != "[UNSUPPORTED]" || r["nil"] != nil || r["bytes"] != "YWJj" || r["text"] != "plain" {
		t.Fatal(r)
	}
	if object(t, r["map"])["Token"] != "[REDACTED]" || object(t, r["ints"])["-3"] != "negative" || r["error"] != "opaque password=visible" {
		t.Fatal(r)
	}
	if bytes.Contains(b.Bytes(), []byte("must-not-be-printed")) {
		t.Fatal(b.String())
	}
}
func TestTruncationAndBuiltins(t *testing.T) {
	marker := "…(truncated)"
	for size := 1; size <= 24; size++ {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			cfg := logging.DefaultConfig()
			cfg.MaxValueBytes = size
			cfg.RedactKeys = append(cfg.RedactKeys, "msg", "time", "level", "source")
			var b bytes.Buffer
			l := newLogger(t, cfg, logging.Dependencies{Output: &b})
			l.Info("message stays complete", "value", strings.Repeat("界", 20), "invalid", string([]byte{0xff, 0xff, 0x61}), "password", "hidden", "msg", "private", "number", 123456789, "duration", time.Hour)
			r := record(t, &b)
			for _, key := range []string{"value", "invalid"} {
				s := r[key].(string)
				if len(s) > size || !utf8.ValidString(s) {
					t.Fatalf("size %d: %q", size, s)
				}
			}
			if size >= len(marker) && !strings.HasSuffix(r["value"].(string), marker) {
				t.Fatal(r)
			}
			if r["password"] != "[REDACTED]" || r["msg"] != "[REDACTED]" || r["number"] != float64(123456789) || r["duration"] != float64(time.Hour) {
				t.Fatal(r)
			}
			if !bytes.Contains(b.Bytes(), []byte(`"msg":"message stays complete"`)) || !bytes.Contains(b.Bytes(), []byte(`"level":"INFO"`)) {
				t.Fatal(b.String())
			}
		})
	}
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	l.Info("invalid", "v", string([]byte{0xff, 'a'}))
	if record(t, &b)["v"] != "\uFFFDa" {
		t.Fatal(b.String())
	}
}

type contextKey struct{}

func TestSourceAndContext(t *testing.T) {
	cfg := logging.DefaultConfig()
	cfg.AddSource = true
	var b bytes.Buffer
	l := newLogger(t, cfg, logging.Dependencies{Output: &b})
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "not-extracted"))
	cancel()
	_, _, line, _ := runtime.Caller(0)
	l.InfoContext(ctx, "after cancellation")
	r := record(t, &b)
	source := object(t, r["source"])
	if !strings.HasSuffix(source["file"].(string), "logging_test.go") || source["line"] != float64(line+1) || strings.Contains(b.String(), "not-extracted") {
		t.Fatal(r)
	}
}

type closeWriter struct {
	closed bool
	bytes.Buffer
}

func (w *closeWriter) Close() error { w.closed = true; return nil }
func TestWriterOwnershipAndErrors(t *testing.T) {
	w := &closeWriter{}
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: w})
	l.Info("ok")
	if w.closed {
		t.Fatal("borrowed writer closed")
	}
	sentinel := errors.New("writer failure")
	h := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: writeFunc(func([]byte) (int, error) { return 0, sentinel })}).Handler()
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "failure", 0)
	if err := h.Handle(context.Background(), r); err != sentinel {
		t.Fatalf("writer identity: %v", err)
	}
	partial := writeFunc(func(p []byte) (int, error) { return len(p) / 2, io.ErrShortWrite })
	h = newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: partial}).Handler()
	if err := h.Handle(context.Background(), r); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }
func TestConcurrentBranchesAndCallbackOutsideWriteLock(t *testing.T) {
	var b bytes.Buffer
	var calls atomic.Int64
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b, Redactor: func(_ []string, a slog.Attr) slog.Attr { calls.Add(1); return a }})
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			branch := l.With("worker", i).WithGroup("event")
			for j := range 25 {
				branch.Info("concurrent", "seq", j)
			}
		})
	}
	wg.Wait()
	lines := bytes.Split(bytes.TrimSpace(b.Bytes()), []byte("\n"))
	if len(lines) != 300 {
		t.Fatal(len(lines))
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatal(string(line))
		}
	}
	if calls.Load() != 312 {
		t.Fatal(calls.Load())
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var writes atomic.Int64
	writer := writeFunc(func(p []byte) (int, error) {
		if writes.Add(1) == 1 {
			close(entered)
			<-release
		}
		return len(p), nil
	})
	var transformed atomic.Int64
	l = newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: writer, Redactor: func(_ []string, a slog.Attr) slog.Attr { transformed.Add(1); return a }})
	done := make(chan struct{})
	go func() { defer close(done); l.Info("blocked", "x", 1) }()
	<-entered
	derived := l.With("outside_lock", 1)
	if transformed.Load() != 2 {
		t.Fatal("callback blocked by writer lock")
	}
	close(release)
	<-done
	derived.Info("finished")
}
func TestCallerPanicsPropagate(t *testing.T) {
	token := &struct{}{}
	cases := []logging.Dependencies{
		{Output: io.Discard, Redactor: func([]string, slog.Attr) slog.Attr { panic(token) }},
		{Output: writeFunc(func([]byte) (int, error) { panic(token) })},
	}
	for _, deps := range cases {
		func() {
			defer func() {
				if recover() != token {
					t.Fatal("panic changed")
				}
			}()
			newLogger(t, logging.DefaultConfig(), deps).Info("panic", "field", 1)
			t.Fatal("panic missing")
		}()
	}
	func() {
		defer func() {
			if recover() != "caller string panic" {
				t.Fatal("Stringer panic changed")
			}
		}()
		newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard}).Info("panic", "field", panicString{})
		t.Fatal("panic missing")
	}()
}
func TestNoCallerMutationAndDeterminism(t *testing.T) {
	payload := map[string]any{"z": 1, "a": 2, "password": "hidden", "nested": []any{"before"}}
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "stable", 0)
	r.AddAttrs(slog.Any("payload", payload))
	if err := l.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	first := b.String()
	b.Reset()
	if err := l.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if b.String() != first || payload["password"] != "hidden" || payload["nested"].([]any)[0] != "before" {
		t.Fatal("mutation or non-deterministic encoding")
	}
	if strings.Index(first, `"a":2`) > strings.Index(first, `"z":1`) {
		t.Fatal(first)
	}
}

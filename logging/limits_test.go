package logging_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	logging "github.com/skolldire/go-engine/logging"
)

func TestDepthAndCycles(t *testing.T) {
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	var v any = "leaf"
	for range 16 {
		v = map[string]any{"next": v}
	}
	l.Info("depth", "root", v)
	r := record(t, &b)
	v = r["root"]
	for range 16 {
		v = object(t, v)["next"]
	}
	if v != "[TRUNCATED_DEPTH]" {
		t.Fatal(v)
	}
	b.Reset()
	v = "leaf"
	for range 15 {
		v = map[string]any{"next": v}
	}
	l.Info("depth16", "root", v)
	v = record(t, &b)["root"]
	for range 15 {
		v = object(t, v)["next"]
	}
	if v != "leaf" {
		t.Fatal(v)
	}
	m := map[string]any{}
	m["self"] = m
	s := make([]any, 1)
	s[0] = s
	type linked struct{ Next *linked }
	p := &linked{}
	p.Next = p
	b.Reset()
	l.Info("cycles", "map", m, "slice", s, "pointer", p)
	r = record(t, &b)
	if object(t, r["map"])["self"] != "[CYCLE]" || r["slice"].([]any)[0] != "[CYCLE]" || object(t, r["pointer"])["Next"] != "[CYCLE]" {
		t.Fatal(r)
	}
	b.Reset()
	shared := map[string]any{"value": 1}
	l.Info("aliases", "tree", map[string]any{"a": shared, "b": shared})
	tree := object(t, record(t, &b)["tree"])
	if object(t, tree["a"])["value"] != float64(1) || object(t, tree["b"])["value"] != float64(1) {
		t.Fatal(tree)
	}
}
func TestItemsAndTotalNodes(t *testing.T) {
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	for _, count := range []int{255, 256, 257, 1000} {
		b.Reset()
		values := make([]int, count)
		m := map[string]any{}
		for i := range count {
			m[fmt.Sprintf("k%04d", i)] = i
		}
		l.Info("items", "slice", values, "array", [257]int{}, "map", m)
		r := record(t, &b)
		list := r["slice"].([]any)
		want := count
		if count > 256 {
			want = 257
		}
		if len(list) != want {
			t.Fatal(len(list))
		}
		obj := object(t, r["map"])
		if len(obj) != want {
			t.Fatal(len(obj))
		}
		if count > 256 && (list[256] != "[TRUNCATED_ITEMS]" || obj["[TRUNCATED_ITEMS]"] != "[TRUNCATED_ITEMS]") {
			t.Fatal(r)
		}
	}
	b.Reset()
	children := make([]any, 256)
	for i := range children {
		children[i] = make([]int, 256)
	}
	l.Info("nodes", "tree", children, "sibling", "untouched")
	r := record(t, &b)
	if !strings.Contains(b.String(), "[TRUNCATED_SIZE]") || r["sibling"] != "untouched" {
		t.Fatal("size bound missing or leaked into sibling")
	}
	var visit func(any) int
	visit = func(v any) int {
		total := 1
		switch x := v.(type) {
		case []any:
			for _, child := range x {
				total += visit(child)
			}
		case map[string]any:
			for _, child := range x {
				total += visit(child)
			}
		}
		return total
	}
	if count := visit(r["tree"]); count > 4096+16 {
		t.Fatalf("unbounded output: %d", count)
	}
}
func TestNativeLeavesAndOpaqueTypes(t *testing.T) {
	cfg := logging.DefaultConfig()
	cfg.MaxValueBytes = 1
	var b bytes.Buffer
	l := newLogger(t, cfg, logging.Dependencies{Output: &b})
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	l.Info("unbounded message", "timevalue", now, "duration", time.Hour, "bool", true, "number", 12345, "bytes", []byte("abc"))
	r := record(t, &b)
	if r["timevalue"] != "2026-01-02T03:04:05Z" || r["bool"] != true || r["number"] != float64(12345) || r["duration"] != float64(time.Hour) {
		t.Fatal(r)
	}
}
func TestLogValuerResolutionAndEmptyAttributes(t *testing.T) {
	var calls atomic.Int64
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	l.LogAttrs(context.Background(), slog.LevelInfo, "group", slog.Attr{}, slog.Group("empty"), slog.Any("lazy", logValue{&calls, slog.GroupValue(slog.String("password", "hidden"), slog.String("value", "ok"))}))
	r := record(t, &b)
	if calls.Load() != 1 || object(t, r["lazy"])["password"] != "[REDACTED]" || len(r) != 4 {
		t.Fatal(r, calls.Load())
	}
}
func TestRedactorIndependentInstances(t *testing.T) {
	var a, b bytes.Buffer
	cfg := logging.DefaultConfig()
	l1 := newLogger(t, cfg, logging.Dependencies{Output: &a})
	cfg.RedactKeys = nil
	l2 := newLogger(t, cfg, logging.Dependencies{Output: &b})
	l1.Info("a", "password", "plain")
	l2.Info("b", "password", "plain")
	if record(t, &a)["password"] != "[REDACTED]" || record(t, &b)["password"] != "plain" {
		t.Fatal("instances/config copy")
	}
}
func TestReentrantRedactorDoesNotHoldWriterLock(t *testing.T) {
	var l *slog.Logger
	var calls atomic.Int64
	l = newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard, Redactor: func(_ []string, a slog.Attr) slog.Attr {
		if calls.Add(1) == 1 {
			l.Info("no attrs recursion terminates")
		}
		return a
	}})
	l.Info("outer", "value", 1)
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

type nilLogValue struct{}

func (*nilLogValue) LogValue() slog.Value { panic("nil pointer evaluated") }
func TestNilLogValuerAndDepthIndependentGroups(t *testing.T) {
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	l.Info("nil", "value", (*nilLogValue)(nil))
	if record(t, &b)["value"] != nil {
		t.Fatal(b.String())
	}
	b.Reset()
	for range 20 {
		l = l.WithGroup("g")
	}
	l.Info("deep path", "value", "leaf")
	var value any = record(t, &b)
	for range 20 {
		value = object(t, value)["g"]
	}
	if object(t, value)["value"] != "leaf" {
		t.Fatal(value)
	}
}
func TestExactNodeBudget(t *testing.T) {
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	values := make([]any, 16)
	for i := range values {
		values[i] = make([]int, 256)
	}
	l.Info("4096 nodes", "tree", values)
	tree := record(t, &b)["tree"].([]any)
	for i := range 15 {
		if len(tree[i].([]any)) != 256 {
			t.Fatal("early truncation")
		}
	}
	last := tree[15].([]any)
	if len(last) != 240 || last[239] != "[TRUNCATED_SIZE]" {
		t.Fatalf("node boundary: %d %#v", len(last), last[len(last)-1])
	}
}

func TestNativePointersKeepNativeEncoding(t *testing.T) {
	cfg := logging.DefaultConfig()
	cfg.MaxValueBytes = 1
	var b bytes.Buffer
	l := newLogger(t, cfg, logging.Dependencies{Output: &b})
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	duration := time.Hour
	l.Info("native pointers", "time", &now, "duration", &duration)
	r := record(t, &b)
	if r["time"] != "2026-01-02T03:04:05Z" || r["duration"] != float64(time.Hour) {
		t.Fatal(r)
	}
}

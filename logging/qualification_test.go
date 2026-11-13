package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	logging "github.com/skolldire/go-engine/logging"
)

func checkLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Fatal("goroutineleak profile missing")
	}
	var report bytes.Buffer
	if err := p.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "total 0") {
		t.Fatal(report.String())
	}
}
func TestNoGoroutineLeaks(t *testing.T) {
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard})
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 30 {
				l.WithGroup("branch").With("id", 1).Info("record", "password", "hidden")
			}
		})
	}
	wg.Wait()
	checkLeaks(t)
}
func stressOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("LOGGING_STRESS") == "" && os.Getenv("LOGGINGSTRESS") == "" {
		t.Skip("set LOGGING_STRESS=1")
	}
}
func TestStressConcurrentRecords(t *testing.T) {
	stressOnly(t)
	var b bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &b})
	const workers, events = 24, 300
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			branch := l.With("worker", worker).WithGroup("fields")
			for seq := range events {
				branch.Info("event", "seq", seq, "object", map[string]any{"password": "unique-hidden-value", "list": []any{1, 2, 3}})
			}
		})
	}
	wg.Wait()
	seen := make(map[[2]int]bool)
	for _, line := range bytes.Split(bytes.TrimSpace(b.Bytes()), []byte("\n")) {
		var r map[string]any
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatal(err)
		}
		f := object(t, r["fields"])
		key := [2]int{int(r["worker"].(float64)), int(f["seq"].(float64))}
		if seen[key] || object(t, f["object"])["password"] != "[REDACTED]" {
			t.Fatal("duplicate or leak")
		}
		seen[key] = true
	}
	if len(seen) != workers*events || bytes.Contains(b.Bytes(), []byte("unique-hidden-value")) {
		t.Fatal("missing record or sensitive value")
	}
	checkLeaks(t)
	t.Logf("%d callers, %d complete records", workers, len(seen))
}
func TestStressRetention(t *testing.T) {
	stressOnly(t)
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	baseline := mem.HeapAlloc
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard})
	const size = 64 << 10
	for round := range 8 {
		for range 150 {
			payload := strings.Repeat("x", size)
			l.With("payload", map[string]any{"large": payload, "secret": payload}).WithGroup("group").Info("event", "round", round)
		}
		runtime.GC()
		runtime.ReadMemStats(&mem)
		if mem.HeapAlloc > baseline+8<<20 {
			t.Fatalf("retained heap baseline=%d now=%d fixture=%d", baseline, mem.HeapAlloc, size)
		}
	}
	runtime.KeepAlive(l)
	checkLeaks(t)
	t.Logf("1200 abandoned derivatives, 64KiB payload; baseline=%d final=%d, allowance8MiB", baseline, mem.HeapAlloc)
}
func TestStressInstancesAndBounds(t *testing.T) {
	stressOnly(t)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 50 {
				l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard})
				m := map[string]any{}
				m["cycle"] = m
				l.Info("bounds", "tree", []any{m, make([]int, 300)}, "secret", m)
			}
		})
	}
	wg.Wait()
	checkLeaks(t)
}
func FuzzConfiguration(f *testing.F) {
	f.Add("info", "json", 4096, "password")
	f.Add("", "", 0, "")
	f.Fuzz(func(t *testing.T, level, format string, max int, key string) {
		if len(level)+len(format)+len(key) > 2048 {
			return
		}
		cfg := logging.Config{Level: level, Format: format, MaxValueBytes: max, RedactKeys: []string{key}}
		_, err := logging.New(cfg, logging.Dependencies{Output: io.Discard})
		valid := (level == "debug" || level == "info" || level == "warn" || level == "error") && (format == "json" || format == "text") && max > 0 && key != ""
		if (err == nil) != valid {
			t.Fatalf("validation mismatch %v", err)
		}
	})
}
func FuzzTextAndKeys(f *testing.F) {
	f.Add("password", "input", uint16(15))
	f.Add("ordinary", string([]byte{0xff, 0x61}), uint16(1))
	f.Add("TOKEN", "anything", uint16(0))
	f.Fuzz(func(t *testing.T, key, text string, size uint16) {
		if len(key) > 256 || len(text) > 16384 {
			return
		}
		cfg := logging.DefaultConfig()
		cfg.MaxValueBytes = int(size%512) + 1
		var b bytes.Buffer
		l := newLogger(t, cfg, logging.Dependencies{Output: &b})
		l.Info("fuzz", "object", map[string]any{key: text})
		r := record(t, &b)
		obj := object(t, r["object"])
		encoded, err := json.Marshal(key)
		if err != nil {
			t.Fatal(err)
		}
		var normalized string
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			t.Fatal(err)
		}
		value, ok := obj[normalized].(string)
		if !ok {
			t.Fatal(obj)
		}
		sensitive := false
		for _, s := range cfg.RedactKeys {
			if strings.EqualFold(s, key) {
				sensitive = true
			}
		}
		if sensitive {
			if value != "[REDACTED]" {
				t.Fatal("sensitive key not protected")
			}
		} else if len(value) > cfg.MaxValueBytes || !utf8.ValidString(value) {
			t.Fatal("invalid text bound")
		}
	})
}
func FuzzStructures(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5})
	f.Add([]byte{0, 0, 0})
	f.Add([]byte{255, 3, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			return
		}
		var payload any = "leaf"
		for _, b := range data {
			switch b % 4 {
			case 0:
				payload = map[string]any{"child": payload, "password": "hidden-fuzz-sentinel"}
			case 1:
				payload = []any{payload, payload}
			case 2:
				payload = struct {
					Token string
					Value any
				}{"hidden-fuzz-sentinel", payload}
			case 3:
				payload = slog.GroupValue(slog.Any("value", payload), slog.String("secret", "hidden-fuzz-sentinel"))
			}
		}
		var out bytes.Buffer
		l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &out})
		l.Info("fuzz", "payload", payload)
		if !json.Valid(bytes.TrimSpace(out.Bytes())) || bytes.Contains(out.Bytes(), []byte("hidden-fuzz-sentinel")) {
			t.Fatal("invalid JSON or leak")
		}
		if out.Len() > 1<<20 {
			t.Fatal("unbounded normalized fixture")
		}
	})
}
func FuzzDerivation(f *testing.F) {
	f.Add([]byte{1, 2, 0, 3})
	f.Add([]byte{2, 2, 1})
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 48 {
			return
		}
		var out bytes.Buffer
		l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &out})
		for _, op := range ops {
			switch op % 4 {
			case 0:
				l = l.WithGroup("group")
			case 1:
				m := map[string]any{"password": "hidden-fuzz-sentinel", "value": "before"}
				l = l.With("object", m)
				m["value"] = "after"
			case 2:
				l = l.With("token", "hidden-fuzz-sentinel")
			case 3:
				l = l.WithGroup("")
			}
		}
		l.Info("derived", "secret", "hidden-fuzz-sentinel")
		if !json.Valid(bytes.TrimSpace(out.Bytes())) || bytes.Contains(out.Bytes(), []byte("hidden-fuzz-sentinel")) || bytes.Contains(out.Bytes(), []byte("after")) {
			t.Fatal("derivation mutation or leak")
		}
	})
}
func BenchmarkOutput(b *testing.B) {
	for _, format := range []string{"json", "text"} {
		cfg := logging.DefaultConfig()
		cfg.Format = format
		l := newLogger(b, cfg, logging.Dependencies{Output: io.Discard})
		var standard *slog.Logger
		if format == "json" {
			standard = slog.New(slog.NewJSONHandler(io.Discard, nil))
		} else {
			standard = slog.New(slog.NewTextHandler(io.Discard, nil))
		}
		b.Run(format+"/standard", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				standard.LogAttrs(context.Background(), slog.LevelInfo, "bench", slog.Int("value", 1))
			}
		})
		b.Run(format+"/logging", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				l.LogAttrs(context.Background(), slog.LevelInfo, "bench", slog.Int("value", 1))
			}
		})
		b.Run(format+"/with", func(b *testing.B) {
			derived := l.With("component", "bench")
			b.ReportAllocs()
			for b.Loop() {
				derived.Info("bench")
			}
		})
	}
}
func BenchmarkTransform(b *testing.B) {
	l := newLogger(b, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard})
	for _, tc := range []struct {
		name string
		attr slog.Attr
	}{
		{"flat", slog.String("password", "hidden")},
		{"nested", slog.Any("payload", map[string]any{"list": []any{1, 2, 3}, "password": "hidden"})},
		{"truncation", slog.String("long", strings.Repeat("x", 16384))},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				l.LogAttrs(context.Background(), slog.LevelInfo, "bench", tc.attr)
			}
		})
	}
	b.Run("disabled", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			l.Debug("disabled", "payload", 1)
		}
	})
	b.Run("construction", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, err := logging.New(logging.DefaultConfig(), logging.Dependencies{Output: io.Discard})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("parallel-shared", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				l.Info("parallel", "value", 1)
			}
		})
	})
	b.Run("parallel-independent", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			local := newLogger(b, logging.DefaultConfig(), logging.Dependencies{Output: io.Discard})
			for pb.Next() {
				local.Info("parallel", "value", 1)
			}
		})
	})
}
func TestIntegrationStructuredOperation(t *testing.T) {
	var out bytes.Buffer
	l := newLogger(t, logging.DefaultConfig(), logging.Dependencies{Output: &out, Attrs: []slog.Attr{slog.String("component", "transport")}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l.WithGroup("response").InfoContext(ctx, "completed", "request", map[string]any{"authorization": "hidden"}, "result", struct{ OK bool }{true})
	r := record(t, &out)
	if r["component"] != "transport" || object(t, object(t, r["response"])["request"])["authorization"] != "[REDACTED]" {
		t.Fatal(r)
	}
	h := l.Handler()
	event := slog.NewRecord(time.Time{}, slog.LevelInfo, "done", 0)
	if err := h.Handle(ctx, event); err != nil {
		t.Fatal(err)
	}
}

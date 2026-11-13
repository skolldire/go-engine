package config_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"weak"

	c "github.com/skolldire/go-engine/config"
	j "github.com/skolldire/go-engine/config/json"
)

func TestConfigurationMatrix(t *testing.T) {
	for _, depth := range []int{2, 4, 8} {
		for _, size := range []int{0, 8, 256} {
			for _, count := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("depth%d/size%d/sources%d", depth, size, count), func(t *testing.T) {
					cfg := c.DefaultConfig()
					cfg.MaxDepth = depth
					var sources []c.Source
					for i := range count {
						sources = append(sources, source(c.Document{"value": strings.Repeat("x", size), "number": i}))
					}
					l, err := c.New(cfg, c.Dependencies{Sources: sources})
					if err != nil {
						t.Fatal(err)
					}
					s, err := l.Load(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					var target struct {
						Value  string
						Number int
					}
					if err := s.Decode("", &target); err != nil || target.Number != count-1 || len(target.Value) != size {
						t.Fatal(target, err)
					}
				})
			}
		}
	}
}
func TestRepeatedLoadRecovery(t *testing.T) {
	expected := errors.New("injected source failure")
	var fail bool
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{sourceFunc(func(context.Context) (c.Document, error) {
		if fail {
			return nil, expected
		}
		return c.Document{"x": 1}, nil
	})}})
	first, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		fail = true
		s, err := l.Load(context.Background())
		if s != nil {
			t.Fatal("partial snapshot")
		}
		requireError(t, err, expected)
		fail = false
		s, err = l.Load(context.Background())
		if err != nil || s.Revision() == first.Revision() {
			t.Fatal(err)
		}
		var target struct{ X int }
		if err := first.Decode("", &target); err != nil || target.X != 1 {
			t.Fatal(err)
		}
	}
}
func TestStressConcurrentLoadAndDecode(t *testing.T) {
	if os.Getenv("CONFIG_STRESS") != "1" {
		t.Skip("set CONFIG_STRESS=1")
	}
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": 42, "list": []any{1, 2, 3}})}})
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 500 {
				s, err := l.Load(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				var target struct {
					X    int
					List []int
				}
				if err = s.Decode("", &target); err != nil || target.X != 42 || len(target.List) != 3 {
					t.Error(target, err)
					return
				}
			}
		})
	}
	wg.Wait()
	assertLeaks(t)
}
func abandonedSnapshots(t *testing.T, l *c.Loader) []weak.Pointer[c.Snapshot] {
	t.Helper()
	var pointers []weak.Pointer[c.Snapshot]
	for range 16 {
		s, err := l.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var got struct{ Payload string }
		if err := s.Decode("", &got); err != nil || len(got.Payload) != (64<<10) {
			t.Fatal(err)
		}
		pointers = append(pointers, weak.Make(s))
	}
	return pointers
}
func TestStressSnapshotRetention(t *testing.T) {
	if os.Getenv("CONFIG_STRESS") != "1" {
		t.Skip("set CONFIG_STRESS=1")
	}
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{sourceFunc(func(context.Context) (c.Document, error) {
		return c.Document{"payload": strings.Repeat("x", 64<<10)}, nil
	})}})
	var baseline, peak uint64
	for cycle := range 60 {
		pointers := abandonedSnapshots(t, l)
		runtime.GC()
		for _, pointer := range pointers {
			if pointer.Value() != nil {
				t.Fatal("abandoned snapshot retained")
			}
		}
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		if cycle == 9 {
			baseline = memory.HeapAlloc
			peak = baseline
		}
		if cycle >= 10 {
			if memory.HeapAlloc > peak {
				peak = memory.HeapAlloc
			}
			if memory.HeapAlloc > baseline+(4<<20) {
				t.Fatalf("heap growth: baseline=%d current=%d", baseline, memory.HeapAlloc)
			}
		}
	}
	runtime.KeepAlive(l)
	assertLeaks(t)
	t.Logf("960 snapshots of 64KiB; baseline=%d peak=%d", baseline, peak)
}
func TestNoGoroutineLeaks(t *testing.T) {
	for range 20 {
		_ = load(t, source(c.Document{"x": 1}))
	}
	assertLeaks(t)
}
func assertLeaks(t *testing.T) {
	t.Helper()
	runtime.GC()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("goroutineleak profile unavailable")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if profile.Count() != 0 {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}

func FuzzJSON(f *testing.F) {
	for _, input := range []string{`{}`, `{"x":1}`, `{"x":1,"x":2}`, `{"x":[1,null]}`, `{"v":1e999}`} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			return
		}
		cfg := j.DefaultConfig()
		cfg.Path = "fuzz"
		cfg.MaxBytes = 4096
		src, _ := j.New(cfg, j.Dependencies{FS: fstest.MapFS{"fuzz": {Data: []byte(input)}}})
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
		s, err := l.Load(context.Background())
		if err != nil {
			return
		}
		var target map[string]string
		_ = s.Decode("", &target)
		if s.Revision() == "" {
			t.Fatal("missing revision")
		}
	})
}
func FuzzReferences(f *testing.F) {
	for _, input := range []string{"${X}", "${X:-${Y:-ok}}", "$${X}", "${bad", "${X:-${X}}", "plain"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 2048 {
			return
		}
		cfg := c.DefaultConfig()
		cfg.MaxSourceBytes = 4096
		l, _ := c.New(cfg, c.Dependencies{Sources: []c.Source{source(c.Document{"v": input})}, Resolver: resolverFunc(func(_ context.Context, key string) (string, error) {
			if key == "X" {
				return "${Y}", nil
			}
			return "", c.ErrReferenceNotFound
		})})
		s, err := l.Load(context.Background())
		if err == nil {
			var target struct{ V string }
			if err := s.Decode("", &target); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func FuzzDecodeAndPaths(f *testing.F) {
	f.Add("42", "v")
	f.Add("0.1", "")
	f.Add("1e-99999999", "v")
	f.Add("0e-99999999", "v")
	f.Fuzz(func(t *testing.T, raw, path string) {
		if len(raw) > 1024 || len(path) > 128 {
			return
		}
		cfg := j.DefaultConfig()
		cfg.Path = "fuzz"
		src, _ := j.New(cfg, j.Dependencies{FS: fstest.MapFS{"fuzz": {Data: []byte(`{"v":` + raw + `}`)}}})
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{src}})
		s, err := l.Load(context.Background())
		if err != nil {
			return
		}
		var small float32
		_ = s.Decode(path, &small)
		s.Has(path)
		var target struct{ V int }
		before := target
		err = s.Decode("", &target)
		if err != nil && !reflect.DeepEqual(target, before) {
			t.Fatal("decode not atomic")
		}
	})
}
func FuzzMerge(f *testing.F) {
	f.Add(`{"x":1}`, `{"x":2}`)
	f.Add(`{"x":{"a":1}}`, `{"x":null}`)
	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a)+len(b) > 4096 {
			return
		}
		var first, second c.Document
		decoder := json.NewDecoder(strings.NewReader(a))
		decoder.UseNumber()
		if decoder.Decode(&first) != nil {
			return
		}
		decoder = json.NewDecoder(strings.NewReader(b))
		decoder.UseNumber()
		if decoder.Decode(&second) != nil {
			return
		}
		l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(first), source(second)}})
		s, err := l.Load(context.Background())
		again, otherErr := l.Load(context.Background())
		if (err == nil) != (otherErr == nil) {
			t.Fatal("nondeterministic merge")
		}
		if err == nil {
			var left, right map[string]string
			leftErr := s.Decode("", &left)
			rightErr := again.Decode("", &right)
			if (leftErr == nil) != (rightErr == nil) || !reflect.DeepEqual(left, right) {
				t.Fatal("nondeterministic snapshot")
			}
		}
	})
}

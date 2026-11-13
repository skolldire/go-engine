package viper_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	c "github.com/skolldire/go-engine/config"
	v "github.com/skolldire/go-engine/config/viper"
)

func TestStressSharedSource(t *testing.T) {
	if os.Getenv("CONFIG_STRESS") != "1" {
		t.Skip("set CONFIG_STRESS=1")
	}
	cfg := v.DefaultConfig()
	cfg.Files = []string{"base.yaml", "prod.yaml"}
	source, err := v.New(cfg, v.Dependencies{FS: fstest.MapFS{"base.yaml": {Data: []byte("app:\n  count: 1\n  label: base\n")}, "prod.yaml": {Data: []byte("app:\n  count: 2\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source}})
			if err != nil {
				t.Error(err)
				return
			}
			for range 200 {
				snapshot, err := loader.Load(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				var target struct {
					Count int
					Label string
				}
				if err := snapshot.Decode("app", &target); err != nil || target.Count != 2 || target.Label != "base" {
					t.Error(target, err)
					return
				}
			}
		})
	}
	wg.Wait()
	runtime.GC()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("missing goroutineleak profile")
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
func FuzzFiles(f *testing.F) {
	f.Add("yaml", "app:\n  count: 1\n")
	f.Add("json", `{"app":{"count":1}}`)
	f.Add("toml", "[app]\ncount=1")
	f.Fuzz(func(t *testing.T, extension, body string) {
		if len(body) > 4096 {
			return
		}
		switch extension {
		case "yaml", "yml", "json", "toml":
		default:
			return
		}
		cfg := v.DefaultConfig()
		name := "app." + extension
		cfg.Files = []string{name}
		cfg.MaxBytes = 4096
		source, err := v.New(cfg, v.Dependencies{FS: fstest.MapFS{name: {Data: []byte(body)}}})
		if err != nil {
			t.Fatal(err)
		}
		loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source}})
		if err != nil {
			t.Fatal(err)
		}
		first, err := loader.Load(context.Background())
		second, secondErr := loader.Load(context.Background())
		if (err == nil) != (secondErr == nil) {
			t.Fatal("non-deterministic load")
		}
		if err != nil {
			return
		}
		var a, b map[string]string
		err = first.Decode("", &a)
		secondErr = second.Decode("", &b)
		if (err == nil) != (secondErr == nil) || !reflect.DeepEqual(a, b) {
			t.Fatal("non-deterministic decode")
		}
	})
}
func BenchmarkLoadFiles(b *testing.B) {
	for _, size := range []int{16, 4096, 65536} {
		for _, count := range []int{1, 4} {
			b.Run(fmt.Sprintf("bytes%d/files%d", size, count), func(b *testing.B) {
				cfg := v.DefaultConfig()
				files := fstest.MapFS{}
				for i := range count {
					name := fmt.Sprintf("app%d.yaml", i)
					cfg.Files = append(cfg.Files, name)
					files[name] = &fstest.MapFile{Data: []byte("payload: " + strings.Repeat("x", size) + "\n")}
				}
				source, err := v.New(cfg, v.Dependencies{FS: files})
				if err != nil {
					b.Fatal(err)
				}
				loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source}})
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := loader.Load(context.Background()); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

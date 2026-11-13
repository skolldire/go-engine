package config_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	c "github.com/skolldire/go-engine/config"
	j "github.com/skolldire/go-engine/config/json"
)

func BenchmarkLoad(b *testing.B) {
	for _, size := range []int{16, 4096, 65536} {
		for _, count := range []int{1, 4} {
			b.Run(fmt.Sprintf("bytes=%d/sources=%d", size, count), func(b *testing.B) {
				var sources []c.Source
				for range count {
					sources = append(sources, source(c.Document{"payload": strings.Repeat("x", size)}))
				}
				l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: sources})
				b.ReportAllocs()
				for b.Loop() {
					if _, err := l.Load(context.Background()); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
func BenchmarkDecode(b *testing.B) {
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"port": 8080, "names": []any{"a", "b"}, "labels": map[string]any{"a": "b"}})}})
	s, _ := l.Load(context.Background())
	b.ReportAllocs()
	for b.Loop() {
		target := struct {
			Port   int
			Names  []string
			Labels map[string]string
		}{Labels: map[string]string{"seed": "x"}}
		if err := s.Decode("", &target); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkResolve(b *testing.B) {
	l, _ := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source(c.Document{"x": "${X}", "y": "prefix-${X}"})}, Resolver: resolverFunc(func(context.Context, string) (string, error) { return "resolved", nil })})
	b.ReportAllocs()
	for b.Loop() {
		if _, err := l.Load(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkJSONLimits(b *testing.B) {
	for _, size := range []int{16, 4096, 65536} {
		for _, exceeded := range []bool{false, true} {
			b.Run(fmt.Sprintf("bytes=%d/exceeded=%t", size, exceeded), func(b *testing.B) {
				cfg := j.DefaultConfig()
				cfg.Path = "bench"
				if exceeded {
					cfg.MaxBytes = int64(size)
				}
				src, _ := j.New(cfg, j.Dependencies{FS: fstest.MapFS{"bench": {Data: []byte(`{"payload":"` + strings.Repeat("x", size) + `"}`)}}})
				b.ReportAllocs()
				for b.Loop() {
					_, err := src.Load(context.Background())
					if (err != nil) != exceeded {
						b.Fatalf("exceeded=%t error=%v", exceeded, err)
					}
				}
			})
		}
	}
}

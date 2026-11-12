package engine

import (
	"context"
	"fmt"
	"testing"
)

// These benchmarks exist to make the cost of the Provider architecture visible
// and comparable over time. The numbers that matter are per-operation
// retrieval (Get is on the hot path for any application that resolves a client
// per request) and per-engine assembly (New runs once, but at container start).

func benchProviders(n int) []Provider {
	out := make([]Provider, n)
	for i := range out {
		out[i] = &benchProvider{name: fmt.Sprintf("component-%d", i)}
	}
	return out
}

type benchProvider struct{ name string }

func (b *benchProvider) Name() string      { return b.name }
func (b *benchProvider) ConfigKey() string { return b.name }
func (b *benchProvider) Init(context.Context, RawConfig, Deps) (any, error) {
	return b, nil
}
func (b *benchProvider) Close(context.Context) error { return nil }

// BenchmarkNew measures engine assembly against a growing provider count, which
// is what tells us whether registration is linear or worse.
func BenchmarkNew(b *testing.B) {
	for _, n := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("providers=%d", n), func(b *testing.B) {
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				providers := benchProviders(n)
				b.StartTimer()

				eng, err := New(ctx, WithConfig(&Config{}), WithProvider(providers...))
				if err != nil {
					b.Fatal(err)
				}

				b.StopTimer()
				_ = eng.Close(ctx)
				b.StartTimer()
			}
		})
	}
}

// BenchmarkGet measures typed component retrieval, the replacement for the
// old hand-written per-adapter getters on Engine.
func BenchmarkGet(b *testing.B) {
	ctx := context.Background()
	eng, err := New(ctx, WithConfig(&Config{}), WithProvider(benchProviders(32)...))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = eng.Close(ctx) })

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Get[*benchProvider](eng, "component-17"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComponent isolates the map lookup from the type assertion in Get.
func BenchmarkComponent(b *testing.B) {
	ctx := context.Background()
	eng, err := New(ctx, WithConfig(&Config{}), WithProvider(benchProviders(32)...))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = eng.Close(ctx) })

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, ok := eng.Component("component-17"); !ok {
			b.Fatal("missing")
		}
	}
}

// BenchmarkDecode measures per-section decoding, which every provider pays once
// at startup and which now also validates unknown keys.
func BenchmarkDecode(b *testing.B) {
	raw := NewRawConfig("widget", map[string]any{
		"endpoint": "http://localhost:4566",
		"retries":  3,
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var cfg fakeConfig
		if err := raw.Decode(&cfg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeNamed measures instance selection from a multi-instance
// section, the shape used by every named adapter.
func BenchmarkDecodeNamed(b *testing.B) {
	section := make([]any, 0, 16)
	for i := 0; i < 16; i++ {
		section = append(section, map[string]any{
			fmt.Sprintf("instance-%d", i): map[string]any{"endpoint": "http://x"},
		})
	}
	raw := NewRawConfig("widgets", section)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := DecodeNamed[fakeConfig](raw, "instance-9"); err != nil {
			b.Fatal(err)
		}
	}
}

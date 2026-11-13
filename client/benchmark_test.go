package client_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	c "github.com/skolldire/go-engine/client"
)

func benchmarkMiddleware[T any](next c.Handler[T]) c.Handler[T] {
	return func(ctx context.Context, inv c.Invocation, op c.Operation[T]) (T, error) { return next(ctx, inv, op) }
}
func BenchmarkScalar(b *testing.B) {
	ctx := context.Background()
	inv := info()
	op := c.Operation[int](func(context.Context) (int, error) { return 42, nil })
	b.Run("operation", func(b *testing.B) {
		b.ReportAllocs()
		var result int
		for b.Loop() {
			var err error
			result, err = op(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
		runtime.KeepAlive(result)
	})
	b.Run("terminal", func(b *testing.B) {
		b.ReportAllocs()
		var result int
		var handler c.Handler[int] = terminal[int]
		for b.Loop() {
			var err error
			result, err = handler(ctx, inv, op)
			if err != nil {
				b.Fatal(err)
			}
		}
		runtime.KeepAlive(result)
	})
	b.Run("do/none", func(b *testing.B) {
		b.ReportAllocs()
		var result int
		for b.Loop() {
			var err error
			result, err = c.Do(ctx, inv, op)
			if err != nil {
				b.Fatal(err)
			}
		}
		runtime.KeepAlive(result)
	})
	for _, count := range []int{1, 4, 16} {
		middleware := make([]c.Middleware[int], count)
		for i := range middleware {
			middleware[i] = benchmarkMiddleware[int]
		}
		b.Run(fmt.Sprintf("chain/%d", count), func(b *testing.B) {
			chain, err := c.Chain(c.Handler[int](terminal[int]), middleware...)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			var result int
			for b.Loop() {
				result, err = chain(ctx, inv, op)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(result)
		})
		b.Run(fmt.Sprintf("do/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			var result int
			for b.Loop() {
				var err error
				result, err = c.Do(ctx, inv, op, middleware...)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(result)
		})
	}
}

type benchmarkResponse struct{ Status, Size int }

func BenchmarkStruct(b *testing.B) {
	ctx := context.Background()
	op := func(context.Context) (benchmarkResponse, error) { return benchmarkResponse{200, 42}, nil }
	for _, count := range []int{0, 1, 4, 16} {
		middleware := make([]c.Middleware[benchmarkResponse], count)
		for i := range middleware {
			middleware[i] = benchmarkMiddleware[benchmarkResponse]
		}
		b.Run(fmt.Sprintf("chain/%d", count), func(b *testing.B) {
			chain, err := c.Chain(c.Handler[benchmarkResponse](terminal[benchmarkResponse]), middleware...)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			var value benchmarkResponse
			for b.Loop() {
				value, err = chain(ctx, info(), op)
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(value)
		})
	}
}
func BenchmarkConcurrentChain(b *testing.B) {
	chain, err := c.Chain(c.Handler[int](terminal[int]), benchmarkMiddleware[int], benchmarkMiddleware[int], benchmarkMiddleware[int], benchmarkMiddleware[int])
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		inv := info()
		operation := c.Operation[int](func(context.Context) (int, error) { return 42, nil })
		for pb.Next() {
			got, err := chain(ctx, inv, operation)
			if err != nil || got != 42 {
				b.Error(got, err)
				return
			}
		}
	})
}
func BenchmarkTypedBoundaryComparison(b *testing.B) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()
	operation := func(ctx context.Context) (benchmarkResponse, error) {
		if err := ctx.Err(); err != nil {
			return benchmarkResponse{}, err
		}
		return benchmarkResponse{200, 42}, nil
	}
	inv := c.Invocation{Component: "io", Operation: "Read"}
	b.ReportAllocs()
	b.ResetTimer()
	var result benchmarkResponse
	for b.Loop() {
		var err error
		result, err = c.Do(ctx, inv, operation)
		if err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(result)
}

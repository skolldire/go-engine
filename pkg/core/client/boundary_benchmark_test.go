package client

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func BenchmarkLegacyBoundaryComparison(b *testing.B) {
	type response struct{ Status, Size int }
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()
	base := NewBaseClientWithName(BaseConfig{Timeout: time.Hour}, nil, "io")
	operation := func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return response{200, 42}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	var result any
	for b.Loop() {
		var err error
		result, err = base.Execute(ctx, "Read", operation)
		if err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(result)
}

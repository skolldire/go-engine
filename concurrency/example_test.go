package concurrency_test

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/concurrency"
)

func ExampleMap() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// One worker preserves order in this example. More workers emit in completion order.
	squared, err := concurrency.Map(ctx, concurrency.From(ctx, 1, 2, 3),
		concurrency.Config{Workers: 1, Buffer: 1},
		func(_ context.Context, n int) (int, error) { return n * n, nil })
	if err != nil {
		panic(err)
	}
	// A second stage consumes Result so that upstream errors remain explicit.
	formatted, err := concurrency.Map(ctx, squared, concurrency.Config{Workers: 1},
		func(_ context.Context, r concurrency.Result[int]) (string, error) {
			if r.Err != nil {
				return "", r.Err
			}
			return fmt.Sprintf("value=%d", r.Value), nil
		})
	if err != nil {
		panic(err)
	}
	for r := range formatted {
		if r.Err != nil {
			panic(r.Err)
		}
		fmt.Println(r.Value)
	}
	// Output:
	// value=1
	// value=4
	// value=9
}

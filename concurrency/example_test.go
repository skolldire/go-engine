package concurrency_test

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/concurrency"
)

func ExampleMap() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, err := concurrency.From(ctx, 1, 2, 3)
	if err != nil {
		panic(err)
	}
	out, err := concurrency.Map(ctx, input, concurrency.StreamConfig{Workers: 1}, func(_ context.Context, n int) (int, error) { return n * n, nil })
	if err != nil {
		panic(err)
	}
	for r := range out {
		if r.Err != nil {
			panic(r.Err)
		}
		fmt.Println(r.Value)
	}
	// Output:
	// 1
	// 4
	// 9
}
func ExampleNewPool() {
	pool, err := concurrency.NewPool(concurrency.DefaultPoolConfig(), func(_ context.Context, n int) (int, error) { return n * 2, nil })
	if err != nil {
		panic(err)
	}
	future, err := pool.Submit(context.Background(), concurrency.Job[int]{ID: "answer", Weight: 1, Value: 21})
	if err != nil {
		panic(err)
	}
	r, err := future.Wait(context.Background())
	if err != nil || r.Err != nil {
		panic("job failed")
	}
	if err := pool.Shutdown(context.Background(), concurrency.ShutdownDrain); err != nil {
		panic(err)
	}
	fmt.Println(r.ID, r.Value, r.State)
	// Output: answer 42 succeeded
}

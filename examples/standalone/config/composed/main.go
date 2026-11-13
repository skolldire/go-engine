package main

import (
	"context"
	"errors"
	"fmt"
	"testing/fstest"
	"time"

	"github.com/skolldire/go-engine/concurrency"
	c "github.com/skolldire/go-engine/config"
	j "github.com/skolldire/go-engine/config/json"
)

func run() error {
	sourceCfg := j.DefaultConfig()
	sourceCfg.Path = "pool.json"
	source, err := j.New(sourceCfg, j.Dependencies{FS: fstest.MapFS{"pool.json": {Data: []byte(`{"pool":{"workers":2,"queue_capacity":4,"max_in_flight_weight":6,"task_timeout":"1s"}}`)}}})
	if err != nil {
		return err
	}
	loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source}})
	if err != nil {
		return err
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		return err
	}
	cfg := concurrency.DefaultPoolConfig()
	if err := snapshot.Decode("pool", &cfg); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	pool, err := concurrency.NewPool(cfg, func(ctx context.Context, n int) (int, error) { return n * 2, ctx.Err() })
	if err != nil {
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	defer pool.Shutdown(shutdownCtx, concurrency.ShutdownCancel)
	for i := range 8 {
		future, err := pool.Submit(context.Background(), concurrency.Job[int]{ID: fmt.Sprint(i), Weight: 1, Value: i})
		if err != nil {
			return err
		}
		result, err := future.Wait(context.Background())
		if err != nil {
			return err
		}
		if result.Err != nil || result.Value != i*2 {
			return errors.New("incorrect task result")
		}
	}
	return pool.Shutdown(shutdownCtx, concurrency.ShutdownDrain)
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("composed: config decoded concurrency defaults; application constructed/executed/closed Pool OK")
}

package main

import (
	"context"
	"errors"
	"fmt"
	"testing/fstest"
	"time"

	"github.com/skolldire/go-engine/concurrency"
	c "github.com/skolldire/go-engine/config"
	e "github.com/skolldire/go-engine/config/env"
	v "github.com/skolldire/go-engine/config/viper"
)

func run() error {
	sourceCfg := v.DefaultConfig()
	sourceCfg.Files = []string{"base.yaml", "prod.yaml"}
	source, err := v.New(sourceCfg, v.Dependencies{FS: fstest.MapFS{"base.yaml": {Data: []byte("pool:\n  workers: 1\n  queue_capacity: 4\n  max_in_flight_weight: 6\n  task_timeout: ${TIMEOUT}\n")}, "prod.yaml": {Data: []byte("pool:\n  workers: 2\n")}}})
	if err != nil {
		return err
	}
	captured := map[string]string{"TIMEOUT": "1s", "WORKERS": "2"}
	lookup := func(name string) (string, bool) { value, ok := captured[name]; return value, ok }
	resolver, err := e.Resolver(lookup)
	if err != nil {
		return err
	}
	override, err := e.New(e.Config{Bindings: map[string]string{"pool.workers": "WORKERS"}}, e.Dependencies{Env: lookup})
	if err != nil {
		return err
	}
	loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{source, override}, Resolver: resolver})
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
	fmt.Println("composed: YAML base/override + env/Resolver decoded concurrency defaults; application constructed/executed/closed Pool OK")
}

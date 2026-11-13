package main

import (
	"context"
	"errors"
	"fmt"
	"testing/fstest"
	"time"

	c "github.com/skolldire/go-engine/config"
	e "github.com/skolldire/go-engine/config/env"
	j "github.com/skolldire/go-engine/config/json"
)

type resolver struct{ lookup func(string) (string, bool) }

func (r resolver) Resolve(_ context.Context, name string) (string, error) {
	value, ok := r.lookup(name)
	if !ok {
		return "", c.ErrReferenceNotFound
	}
	return value, nil
}
func run() error {
	captured := map[string]string{"PORT": "0", "TOKEN": ""}
	lookup := func(name string) (string, bool) { value, ok := captured[name]; return value, ok }
	jsonCfg := j.DefaultConfig()
	jsonCfg.Path = "app.json"
	base, err := j.New(jsonCfg, j.Dependencies{FS: fstest.MapFS{"app.json": {Data: []byte(`{"app":{"port":8080,"enabled":false,"timeout":"1s","token":"${TOKEN:-fallback}"}}`)}}})
	if err != nil {
		return err
	}
	overrides, err := e.New(e.Config{Bindings: map[string]string{"app.port": "PORT"}, Required: []string{"app.port"}}, e.Dependencies{Env: lookup})
	if err != nil {
		return err
	}
	loader, err := c.New(c.DefaultConfig(), c.Dependencies{Sources: []c.Source{base, overrides}, Resolver: resolver{lookup}})
	if err != nil {
		return err
	}
	snapshot, err := loader.Load(context.Background())
	if err != nil {
		return err
	}
	app := struct {
		Port    int
		Enabled bool
		Timeout time.Duration
		Token   string
	}{Port: 80, Enabled: true, Timeout: 5 * time.Second, Token: "seed"}
	if err := snapshot.Decode("app", &app); err != nil {
		return err
	}
	if app.Port != 0 || app.Enabled || app.Timeout != time.Second || app.Token != "" {
		return errors.New("defaults or overrides incorrect")
	}
	if err := snapshot.Decode("missing", &app); !errors.Is(err, c.ErrSectionMissing) {
		return errors.New("missing section contract")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loader.Load(ctx); !errors.Is(err, context.Canceled) {
		return errors.New("cancel contract")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("minimal: JSON + env + references + seeded defaults + errors/cancellation OK")
}

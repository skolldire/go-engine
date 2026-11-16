package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/skolldire/go-engine/health"
)

func run() error {
	r, err := health.New(health.DefaultConfig())
	if err != nil {
		return err
	}
	var fail atomic.Bool
	original := errors.New("private-backend-cause")
	err = r.Register("database", health.CheckerFunc(func(context.Context) error {
		if fail.Load() {
			return original
		}
		return nil
	}))
	if err != nil {
		return err
	}
	ready, err := r.Ready(context.Background())
	if err != nil || !ready.Live || !ready.Ready {
		return errors.New("healthy probe failed")
	}
	fail.Store(true)
	diagnostic, err := r.Check(context.Background())
	if err != nil || diagnostic.Ready || len(diagnostic.Checks) != 1 || diagnostic.Checks[0].Err != original {
		return errors.New("diagnostic mismatch")
	}
	if _, err = r.Ready(context.Background()); !errors.Is(err, health.ErrNotReady) {
		return errors.New("unhealthy probe accepted")
	}
	raw, err := json.Marshal(diagnostic)
	if err != nil {
		return err
	}
	if strings.Contains(string(raw), original.Error()) || strings.Contains(string(raw), "Err") {
		return errors.New("private error serialized")
	}
	if err = r.Register("late", health.CheckerFunc(func(context.Context) error { return nil })); !errors.Is(err, health.ErrRegistryFrozen) {
		return errors.New("registry not frozen")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("health minimal: liveness, readiness, diagnostic and private error verified")
}

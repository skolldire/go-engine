package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/skolldire/go-engine/client"
	"github.com/skolldire/go-engine/logging"
)

type adapter struct {
	transport func(context.Context) (int, error)
}

func (a adapter) Read(ctx context.Context) (int, error) { return a.transport(ctx) }
func verify() error {
	var out bytes.Buffer
	logger, err := logging.New(logging.DefaultConfig(), logging.Dependencies{Output: &out})
	if err != nil {
		return err
	}
	sentinel := errors.New("transport failure")
	calls := 0
	transport := adapter{transport: func(context.Context) (int, error) { calls++; return 7, sentinel }}
	observe := func(next client.Handler[int]) client.Handler[int] {
		return func(ctx context.Context, info client.Invocation, op client.Operation[int]) (int, error) {
			result, err := next(ctx, info, op)
			logger.LogAttrs(ctx, slog.LevelInfo, "operation completed", slog.String("component", info.Component), slog.String("operation", info.Operation), slog.Bool("success", err == nil))
			return result, err
		}
	}
	result, err := client.Do(context.Background(), client.Invocation{Component: "memory", Operation: "read"}, client.Operation[int](transport.Read), observe)
	if result != 7 || err != sentinel || calls != 1 {
		return errors.New("composition changed result or cause")
	}
	if bytes.Count(out.Bytes(), []byte("\n")) != 1 {
		return errors.New("expected one observation")
	}
	var record map[string]any
	if err = json.Unmarshal(out.Bytes(), &record); err != nil {
		return err
	}
	if record["component"] != "memory" || record["operation"] != "read" || record["success"] != false || bytes.Contains(out.Bytes(), []byte(sentinel.Error())) {
		return errors.New("unexpected observation")
	}
	return nil
}
func main() {
	if err := verify(); err != nil {
		panic(err)
	}
	fmt.Println("logging composed with client: passed")
}

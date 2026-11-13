package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/skolldire/go-engine/client"
	clientlogging "github.com/skolldire/go-engine/client/logging"
)

type ownLogger struct {
	enabled bool
	records [][]slog.Attr
}

func (l *ownLogger) Enabled(context.Context, slog.Level) bool { return l.enabled }
func (l *ownLogger) LogAttrs(_ context.Context, _ slog.Level, _ string, a ...slog.Attr) {
	l.records = append(l.records, append([]slog.Attr(nil), a...))
}
func verify() error {
	var output bytes.Buffer
	native := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := clientlogging.DefaultConfig()
	observe, err := clientlogging.New[int](cfg, clientlogging.Dependencies{Logger: native})
	if err != nil {
		return err
	}
	sentinel := errors.New("must-not-be-logged")
	calls := 0
	result, err := client.Do(context.Background(), client.Invocation{Component: "memory", Operation: "read"}, func(context.Context) (int, error) { calls++; return 7, sentinel }, observe)
	if result != 7 || err != sentinel || calls != 1 {
		return errors.New("changed I/O result")
	}
	var record map[string]any
	if err = json.Unmarshal(output.Bytes(), &record); err != nil {
		return err
	}
	if record["msg"] != "operation completed" || record["success"] != false || record["duration"] == nil || bytes.Contains(output.Bytes(), []byte(sentinel.Error())) {
		return errors.New("unexpected native observation")
	}
	custom := &ownLogger{enabled: true}
	cfg = clientlogging.Config{Level: "info", IncludeDuration: false}
	observe, err = clientlogging.New[int](cfg, clientlogging.Dependencies{Logger: custom})
	if err != nil {
		return err
	}
	terminal := func(ctx context.Context, _ client.Invocation, op client.Operation[int]) (int, error) { return op(ctx) }
	chain, err := client.Chain(client.Handler[int](terminal), observe)
	if err != nil {
		return err
	}
	result, err = chain(context.Background(), client.Invocation{Component: "memory", Operation: "read"}, func(context.Context) (int, error) { return 9, nil })
	if result != 9 || err != nil || len(custom.records) != 1 || len(custom.records[0]) != 3 {
		return errors.New("custom backend mismatch")
	}
	custom.enabled = false
	_, err = chain(context.Background(), client.Invocation{Component: "memory", Operation: "read"}, func(context.Context) (int, error) { return 9, nil })
	if err != nil || len(custom.records) != 1 {
		return errors.New("disabled backend emitted")
	}
	invalid := cfg
	invalid.Level = "error"
	if _, err = clientlogging.New[int](invalid, clientlogging.Dependencies{Logger: custom}); !errors.Is(err, clientlogging.ErrInvalidConfig) {
		return errors.New("invalid config accepted")
	}
	return nil
}
func main() {
	if err := verify(); err != nil {
		panic(err)
	}
	fmt.Println("client/logging minimal: native JSON and own Logger passed")
}

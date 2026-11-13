package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/skolldire/go-engine/logging"
)

func verify() error {
	cfg := logging.DefaultConfig()
	cfg.MaxValueBytes = 64
	var out bytes.Buffer
	logger, err := logging.New(cfg, logging.Dependencies{Output: &out})
	if err != nil {
		return err
	}
	logger.With("component", "minimal").WithGroup("request").InfoContext(context.Background(), "completed", "payload", map[string]any{"password": "hidden", "ok": true})
	var r struct {
		Component string
		Request   struct {
			Payload struct {
				Password string
				OK       bool
			}
		}
	}
	if err = json.Unmarshal(out.Bytes(), &r); err != nil {
		return err
	}
	if r.Component != "minimal" || r.Request.Payload.Password != "[REDACTED]" || !r.Request.Payload.OK {
		return errors.New("unexpected structured output")
	}
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		return errors.New("unexpected level")
	}
	invalid := cfg
	invalid.MaxValueBytes = 0
	if _, err = logging.New(invalid, logging.Dependencies{Output: &out}); !errors.Is(err, logging.ErrInvalidConfig) {
		return errors.New("invalid configuration accepted")
	}
	return nil
}
func main() {
	if err := verify(); err != nil {
		panic(err)
	}
	fmt.Println("logging minimal: passed")
}

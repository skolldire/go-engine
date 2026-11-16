package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/logging"
)

func run() error {
	var logs bytes.Buffer
	logger, err := logging.New(logging.DefaultConfig(), logging.Dependencies{Output: &logs})
	if err != nil {
		return err
	}
	r, err := health.New(health.DefaultConfig())
	if err != nil {
		return err
	}
	original := errors.New("private-database-detail")
	if err = r.Register("database", health.CheckerFunc(func(context.Context) error { return original })); err != nil {
		return err
	}
	if err = r.Register("panic-check", health.CheckerFunc(func(context.Context) error { panic("private-panic-payload") })); err != nil {
		return err
	}
	report, err := r.Check(context.Background())
	if err != nil || report.Ready || report.Checks[0].Err != original {
		return errors.New("diagnostic mismatch")
	}
	for _, check := range report.Checks {
		if check.Err != nil {
			logger.LogAttrs(context.Background(), slog.LevelError, "dependency down", slog.String("name", check.Name), slog.Any("error", check.Err))
		}
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	for _, secret := range []string{original.Error(), "private-panic-payload", "Stack", "Value", "Err"} {
		if bytes.Contains(raw, []byte(secret)) {
			return errors.New("private report field leaked")
		}
	}
	if !bytes.Contains(logs.Bytes(), []byte(original.Error())) || bytes.Contains(logs.Bytes(), []byte("private-panic-payload")) {
		return errors.New("explicit logging policy mismatch")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("health composed: caller logs private errors, public report stays safe")
}

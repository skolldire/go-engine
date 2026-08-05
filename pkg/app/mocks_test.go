package app

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

type mockLogger struct{}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]any)     {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)      {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]any)      {}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]any)      {}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (m *mockLogger) WrapError(err error, msg string) error                            { return err }
func (m *mockLogger) WithField(key string, value any) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]any) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                              { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                   { return nil }

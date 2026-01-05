package testutil

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// MockLogger is a shared stub logger implementation for tests
type MockLogger struct{}

func (m *MockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{})     {}
func (m *MockLogger) Info(ctx context.Context, msg string, fields map[string]interface{})      {}
func (m *MockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{})      {}
func (m *MockLogger) Error(ctx context.Context, err error, fields map[string]interface{})      {}
func (m *MockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *MockLogger) WrapError(err error, msg string) error                                    { return err }
func (m *MockLogger) WithField(key string, value interface{}) logger.Service                   { return m }
func (m *MockLogger) WithFields(fields map[string]interface{}) logger.Service                  { return m }
func (m *MockLogger) GetLogLevel() string                                                      { return "info" }
func (m *MockLogger) SetLogLevel(level string) error                                           { return nil }

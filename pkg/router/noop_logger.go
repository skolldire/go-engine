package router

import (
	"context"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

// noopLogger is the fallback used when NewService is called without WithLogger.
// Constructing a router without a logger is a valid use of the API, so the
// server must start and shut down silently rather than panic on the first
// a.logger call.
type noopLogger struct{}

var _ logger.Service = (*noopLogger)(nil)

func (noopLogger) Info(context.Context, string, map[string]any)      {}
func (noopLogger) Error(context.Context, error, map[string]any)      {}
func (noopLogger) Debug(context.Context, string, map[string]any)     {}
func (noopLogger) Warn(context.Context, string, map[string]any)      {}
func (noopLogger) FatalError(context.Context, error, map[string]any) {}
func (noopLogger) WrapError(err error, _ string) error               { return err }
func (n noopLogger) WithField(string, any) logger.Service            { return n }
func (n noopLogger) WithFields(map[string]any) logger.Service        { return n }
func (noopLogger) GetLogLevel() string                               { return "info" }
func (noopLogger) SetLogLevel(string) error                          { return nil }

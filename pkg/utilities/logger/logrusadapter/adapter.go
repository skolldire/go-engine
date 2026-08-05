package logrusadapter

import (
	"github.com/sirupsen/logrus"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

var _ logger.LogWriter = (*adapter)(nil)
var _ logger.LogWriter = (*entryAdapter)(nil)

// adapter wraps *logrus.Logger and satisfies logger.LogWriter.
type adapter struct {
	l *logrus.Logger
}

// entryAdapter wraps *logrus.Entry and satisfies logger.LogWriter,
// enabling correct chaining after WithField / WithFields calls.
type entryAdapter struct {
	e *logrus.Entry
}

// New returns a logger.LogWriter backed by l.
func New(l *logrus.Logger) logger.LogWriter {
	return &adapter{l: l}
}

// UnwrapLogrus exposes the underlying *logrus.Logger so that
// logger.NewService can apply its format/output/caller configuration.
func (a *adapter) UnwrapLogrus() *logrus.Logger { return a.l }

// ── adapter ───────────────────────────────────────────────────────────────────

func (a *adapter) Info(args ...any)  { a.l.Info(args...) }
func (a *adapter) Warn(args ...any)  { a.l.Warn(args...) }
func (a *adapter) Error(args ...any) { a.l.Error(args...) }
func (a *adapter) Debug(args ...any) { a.l.Debug(args...) }
func (a *adapter) Fatal(args ...any) { a.l.Fatal(args...) }

func (a *adapter) Infof(format string, args ...any)  { a.l.Infof(format, args...) }
func (a *adapter) Warnf(format string, args ...any)  { a.l.Warnf(format, args...) }
func (a *adapter) Errorf(format string, args ...any) { a.l.Errorf(format, args...) }
func (a *adapter) Debugf(format string, args ...any) { a.l.Debugf(format, args...) }

func (a *adapter) WithField(key string, value any) logger.LogWriter {
	return &entryAdapter{e: a.l.WithField(key, value)}
}

func (a *adapter) WithFields(fields logrus.Fields) logger.LogWriter {
	return &entryAdapter{e: a.l.WithFields(fields)}
}

// ── entryAdapter ──────────────────────────────────────────────────────────────

func (ea *entryAdapter) Info(args ...any)  { ea.e.Info(args...) }
func (ea *entryAdapter) Warn(args ...any)  { ea.e.Warn(args...) }
func (ea *entryAdapter) Error(args ...any) { ea.e.Error(args...) }
func (ea *entryAdapter) Debug(args ...any) { ea.e.Debug(args...) }
func (ea *entryAdapter) Fatal(args ...any) { ea.e.Fatal(args...) }

func (ea *entryAdapter) Infof(format string, args ...any)  { ea.e.Infof(format, args...) }
func (ea *entryAdapter) Warnf(format string, args ...any)  { ea.e.Warnf(format, args...) }
func (ea *entryAdapter) Errorf(format string, args ...any) { ea.e.Errorf(format, args...) }
func (ea *entryAdapter) Debugf(format string, args ...any) { ea.e.Debugf(format, args...) }

func (ea *entryAdapter) WithField(key string, value any) logger.LogWriter {
	return &entryAdapter{e: ea.e.WithField(key, value)}
}

func (ea *entryAdapter) WithFields(fields logrus.Fields) logger.LogWriter {
	return &entryAdapter{e: ea.e.WithFields(fields)}
}

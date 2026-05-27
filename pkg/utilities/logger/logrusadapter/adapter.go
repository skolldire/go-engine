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

func (a *adapter) Info(args ...interface{})  { a.l.Info(args...) }
func (a *adapter) Warn(args ...interface{})  { a.l.Warn(args...) }
func (a *adapter) Error(args ...interface{}) { a.l.Error(args...) }
func (a *adapter) Debug(args ...interface{}) { a.l.Debug(args...) }
func (a *adapter) Fatal(args ...interface{}) { a.l.Fatal(args...) }

func (a *adapter) Infof(format string, args ...interface{})  { a.l.Infof(format, args...) }
func (a *adapter) Warnf(format string, args ...interface{})  { a.l.Warnf(format, args...) }
func (a *adapter) Errorf(format string, args ...interface{}) { a.l.Errorf(format, args...) }
func (a *adapter) Debugf(format string, args ...interface{}) { a.l.Debugf(format, args...) }

func (a *adapter) WithField(key string, value interface{}) logger.LogWriter {
	return &entryAdapter{e: a.l.WithField(key, value)}
}

func (a *adapter) WithFields(fields logrus.Fields) logger.LogWriter {
	return &entryAdapter{e: a.l.WithFields(fields)}
}

// ── entryAdapter ──────────────────────────────────────────────────────────────

func (ea *entryAdapter) Info(args ...interface{})  { ea.e.Info(args...) }
func (ea *entryAdapter) Warn(args ...interface{})  { ea.e.Warn(args...) }
func (ea *entryAdapter) Error(args ...interface{}) { ea.e.Error(args...) }
func (ea *entryAdapter) Debug(args ...interface{}) { ea.e.Debug(args...) }
func (ea *entryAdapter) Fatal(args ...interface{}) { ea.e.Fatal(args...) }

func (ea *entryAdapter) Infof(format string, args ...interface{})  { ea.e.Infof(format, args...) }
func (ea *entryAdapter) Warnf(format string, args ...interface{})  { ea.e.Warnf(format, args...) }
func (ea *entryAdapter) Errorf(format string, args ...interface{}) { ea.e.Errorf(format, args...) }
func (ea *entryAdapter) Debugf(format string, args ...interface{}) { ea.e.Debugf(format, args...) }

func (ea *entryAdapter) WithField(key string, value interface{}) logger.LogWriter {
	return &entryAdapter{e: ea.e.WithField(key, value)}
}

func (ea *entryAdapter) WithFields(fields logrus.Fields) logger.LogWriter {
	return &entryAdapter{e: ea.e.WithFields(fields)}
}

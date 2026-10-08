package logger

import "github.com/sirupsen/logrus"

// LogWriter is the low-level log-writing interface used internally by the framework.
// It mirrors the subset of *logrus.Logger methods that the framework relies on.
//
// *logrus.Logger satisfies all methods of this interface except WithField and
// WithFields, whose concrete implementations return *logrus.Entry rather than
// LogWriter. A thin adapter wrapping *logrus.Logger provides full satisfaction
// without modifying logrus itself.
type LogWriter interface {
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Debug(args ...any)
	Fatal(args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Debugf(format string, args ...any)
	WithField(key string, value any) LogWriter
	WithFields(fields logrus.Fields) LogWriter
}

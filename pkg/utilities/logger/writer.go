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
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Debug(args ...interface{})
	Fatal(args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	WithField(key string, value interface{}) LogWriter
	WithFields(fields logrus.Fields) LogWriter
}

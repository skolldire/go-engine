package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var _ Service = (*service)(nil)

var defaultContextExtractor = func(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{}
}

func NewService(c Config, l *logrus.Logger) Service {
	if l == nil {
		l = logrus.New()
	}

	configureLogger(l, c)

	svc := &service{
		Log:              l,
		fields:           logrus.Fields{},
		contextExtractor: getContextExtractor(c.ContextExtractor),
	}

	l.Info("Logger service initialized")
	return svc
}

func configureLogger(l *logrus.Logger, c Config) {
	l.Level = loggerLevel(c.Level)

	configureFormatter(l, c.Format)

	l.ReportCaller = c.ReportCaller

	configureOutput(l, c)

	if c.ExitFunc != nil {
		l.ExitFunc = c.ExitFunc
	}
}

func configureFormatter(l *logrus.Logger, format string) {
	if strings.ToLower(format) == "json" {
		l.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "severity",
				logrus.FieldKeyMsg:   "message",
			},
		})
	} else {
		l.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: time.RFC3339Nano,
			FullTimestamp:   true,
		})
	}
}

func configureOutput(l *logrus.Logger, c Config) {
	if len(c.OutputWriters) > 0 {
		configureMultipleOutputs(l, c)
	} else if c.Path != "" {
		configureFileOutput(l, c.Path)
	} else {
		l.SetOutput(os.Stdout)
	}
}

func configureMultipleOutputs(l *logrus.Logger, c Config) {
	writers := make([]io.Writer, 0, len(c.OutputWriters)+1)
	writers = append(writers, c.OutputWriters...)

	if c.Path != "" {
		file := openLogFile(l, c.Path)
		if file != nil {
			writers = append(writers, file)
		}
	}

	if len(writers) == 0 {
		l.SetOutput(os.Stdout)
	} else {
		l.SetOutput(io.MultiWriter(writers...))
	}
}

func configureFileOutput(l *logrus.Logger, path string) {
	file := openLogFile(l, path)
	if file != nil {
		l.SetOutput(file)
	} else {
		l.SetOutput(os.Stdout)
	}
}

func openLogFile(l *logrus.Logger, path string) io.Writer {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		l.Warnf("Could not create log directory: %v", err)
		return nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		l.Warnf("Could not open log file %s: %v, using standard output", path, err)
		return nil
	}

	return file
}

func getContextExtractor(extractor ContextExtractor) ContextExtractor {
	if extractor != nil {
		return extractor
	}
	return defaultContextExtractor
}

func (l *service) WithField(key string, value interface{}) Service {
	newFields := make(logrus.Fields, len(l.fields)+1)
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value

	return &service{
		Log:              l.Log,
		fields:           newFields,
		contextExtractor: l.contextExtractor,
	}
}

func (l *service) WithFields(fields map[string]interface{}) Service {
	if len(fields) == 0 {
		return l
	}

	newFields := make(logrus.Fields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &service{
		Log:              l.Log,
		fields:           newFields,
		contextExtractor: l.contextExtractor,
	}
}

func (l *service) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	entry := l.createEntry(ctx, fields)
	entry.Info(msg)
}

func (l *service) Error(ctx context.Context, err error, fields map[string]interface{}) {
	entry := l.createEntry(ctx, fields)

	if err != nil {
		entry = entry.WithError(err)
		entry.Error(err)
	} else {
		entry.Error("Error desconocido")
	}
}

func (l *service) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	entry := l.createEntry(ctx, fields)
	entry.Debug(msg)
}

func (l *service) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	entry := l.createEntry(ctx, fields)
	entry.Warn(msg)
}

func (l *service) FatalError(ctx context.Context, err error, fields map[string]interface{}) {
	entry := l.createEntry(ctx, fields)

	if err != nil {
		entry = entry.WithError(err)
		entry.Fatal(err)
	} else {
		entry.Fatal("Error fatal desconocido")
	}
}

func (l *service) WrapError(err error, msg string) error {
	if err == nil {
		return errors.New(msg)
	}
	return errors.Wrap(err, msg)
}

func (l *service) GetLogLevel() string {
	return l.Log.GetLevel().String()
}

func (l *service) SetLogLevel(level string) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("nivel de log inválido: %s", level)
	}
	l.Log.SetLevel(lvl)
	return nil
}

func (l *service) createEntry(ctx context.Context, fields map[string]interface{}) *logrus.Entry {
	entry := l.Log.WithFields(l.fields)

	if l.contextExtractor != nil && ctx != nil {
		ctxFields := l.contextExtractor(ctx)
		for k, v := range ctxFields {
			entry = entry.WithField(k, v)
		}
	}

	for k, v := range fields {
		entry = entry.WithField(k, v)
	}

	return entry
}

func loggerLevel(level string) logrus.Level {
	switch strings.ToLower(level) {
	case "panic":
		return logrus.PanicLevel
	case "fatal":
		return logrus.FatalLevel
	case "error":
		return logrus.ErrorLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "info":
		return logrus.InfoLevel
	case "debug":
		return logrus.DebugLevel
	case "trace":
		return logrus.TraceLevel
	default:
		return logrus.InfoLevel
	}
}

package logger

import (
	"context"
	"io"

	"github.com/sirupsen/logrus"
)

// Config contiene la configuración para el servicio de logger
type Config struct {
	Level            string           `mapstructure:"level"`
	Path             string           `mapstructure:"path"`
	Format           string           `mapstructure:"format"`
	ReportCaller     bool             `mapstructure:"report_caller"`
	ExitFunc         func(int)        `mapstructure:"-"`
	OutputWriters    []io.Writer      `mapstructure:"-"`
	ContextExtractor ContextExtractor `mapstructure:"-"`
}

type ContextExtractor func(ctx context.Context) map[string]interface{}

type Service interface {
	Info(ctx context.Context, msg string, fields map[string]interface{})
	Error(ctx context.Context, err error, fields map[string]interface{})
	Debug(ctx context.Context, msg string, fields map[string]interface{})
	Warn(ctx context.Context, msg string, fields map[string]interface{})
	FatalError(ctx context.Context, err error, fields map[string]interface{})
	WrapError(err error, msg string) error
	WithField(key string, value interface{}) Service
	WithFields(fields map[string]interface{}) Service
	GetLogLevel() string
	SetLogLevel(level string) error
}

type service struct {
	Log              *logrus.Logger
	fields           logrus.Fields
	contextExtractor ContextExtractor
}

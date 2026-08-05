package logger

import (
	"context"
	"io"

	"github.com/sirupsen/logrus"
)

type Config struct {
	Level            string           `mapstructure:"level" json:"level"`
	Path             string           `mapstructure:"path" json:"path"`
	Format           string           `mapstructure:"format" json:"format"`
	ReportCaller     bool             `mapstructure:"report_caller" json:"report_caller"`
	ExitFunc         func(int)        `mapstructure:"-" json:"-"`
	OutputWriters    []io.Writer      `mapstructure:"-" json:"-"`
	ContextExtractor ContextExtractor `mapstructure:"-" json:"-"`
}

type ContextExtractor func(ctx context.Context) map[string]any

type Service interface {
	Info(ctx context.Context, msg string, fields map[string]any)
	Error(ctx context.Context, err error, fields map[string]any)
	Debug(ctx context.Context, msg string, fields map[string]any)
	Warn(ctx context.Context, msg string, fields map[string]any)
	FatalError(ctx context.Context, err error, fields map[string]any)
	WrapError(err error, msg string) error
	WithField(key string, value any) Service
	WithFields(fields map[string]any) Service
	GetLogLevel() string
	SetLogLevel(level string) error
}

type service struct {
	Log              *logrus.Logger
	fields           logrus.Fields
	contextExtractor ContextExtractor
}

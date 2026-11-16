package errors

import (
	"context"
	stderrors "errors"
)

var (
	ErrInvalidConfig  = stderrors.New("invalid router error configuration")
	ErrInvalidCall    = stderrors.New("invalid router error call")
	ErrNilContext     = stderrors.New("router/errors: nil context")
	ErrInvalidProblem = stderrors.New("router/errors: invalid classified problem")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "router/errors: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	IncludeRequestID bool `mapstructure:"include_request_id" json:"include_request_id"`
	MaxDetailsBytes  int  `mapstructure:"max_details_bytes" json:"max_details_bytes"`
}
type Problem struct {
	Status               int
	Code, Title, Details string
}
type Dependencies struct {
	Classify  func(error) Problem
	RequestID func(context.Context) string
}
type Responder struct {
	cfg  Config
	deps Dependencies
}

func DefaultConfig() Config { return Config{IncludeRequestID: true, MaxDetailsBytes: 4096} }
func (cfg Config) Validate() error {
	if cfg.MaxDetailsBytes <= 0 {
		return &ConfigError{Field: "MaxDetailsBytes", Reason: "must be positive"}
	}
	return nil
}
func NewResponder(cfg Config, deps Dependencies) (*Responder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Classify == nil {
		return nil, &ConfigError{Field: "Classify", Reason: "is required"}
	}
	return &Responder{cfg: cfg, deps: deps}, nil
}

package router

import "errors"

var (
	ErrInvalidConfig  = errors.New("invalid router configuration")
	ErrInvalidCall    = errors.New("invalid router call")
	ErrFrozen         = errors.New("router is frozen")
	ErrDuplicateRoute = errors.New("duplicate route")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "router: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

package concurrency

import (
	"errors"
	"fmt"
	"runtime"
)

var (
	ErrInvalidConfig    = errors.New("invalid concurrency configuration")
	ErrClosed           = errors.New("pool admission is closed")
	ErrSaturated        = errors.New("pool capacity unavailable")
	ErrAdmissionTimeout = errors.New("pool admission timed out")
	ErrDuplicateID      = errors.New("job ID is already active")
	ErrJobTooLarge      = errors.New("job weight exceeds capacity")
	ErrKeyAborted       = errors.New("pending job canceled after key failure")
	ErrPanic            = errors.New("task panicked")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ErrInvalidConfig, e.Field, e.Reason)
}
func (e *ConfigError) Unwrap() error     { return ErrInvalidConfig }
func invalid(field, reason string) error { return &ConfigError{Field: field, Reason: reason} }

type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string        { return ErrPanic.Error() }
func (e *PanicError) Is(target error) bool { return target == ErrPanic }
func panicError(value any) *PanicError {
	stack := make([]byte, 64<<10)
	n := runtime.Stack(stack, false)
	return &PanicError{Value: value, Stack: stack[:n:n]}
}

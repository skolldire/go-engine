package health

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

var (
	ErrInvalidConfig  = errors.New("invalid health configuration")
	ErrInvalidCall    = errors.New("invalid health call")
	ErrNilContext     = errors.New("health: nil context")
	ErrDuplicateCheck = errors.New("health: duplicate check")
	ErrTooManyChecks  = errors.New("health: too many checks")
	ErrRegistryFrozen = errors.New("health: registry frozen")
	ErrNotReady       = errors.New("health: not ready")
	ErrPanic          = errors.New("health: checker terminated abnormally")
)

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "health: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type PanicError struct {
	Value  any
	Stack  []byte
	Goexit bool
}

func (e *PanicError) Error() string        { return ErrPanic.Error() }
func (e *PanicError) Is(target error) bool { return target == ErrPanic }

type Config struct {
	Timeout     time.Duration `mapstructure:"timeout" json:"timeout"`
	MaxParallel int           `mapstructure:"max_parallel" json:"max_parallel"`
	MaxChecks   int           `mapstructure:"max_checks" json:"max_checks"`
}

func DefaultConfig() Config { return Config{Timeout: 5 * time.Second, MaxParallel: 4, MaxChecks: 128} }
func (cfg Config) Validate() error {
	switch {
	case cfg.Timeout <= 0:
		return &ConfigError{"Timeout", "must be positive"}
	case cfg.MaxParallel <= 0:
		return &ConfigError{"MaxParallel", "must be positive"}
	case cfg.MaxChecks <= 0:
		return &ConfigError{"MaxChecks", "must be positive"}
	}
	return nil
}

type Checker interface{ Check(context.Context) error }
type CheckerFunc func(context.Context) error

func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

type Status string
type Reason string
type CheckResult struct {
	Name     string
	Status   Status
	Reason   Reason
	Duration time.Duration
	Err      error `json:"-"`
}
type Report struct {
	Live   bool
	Ready  bool
	Checks []CheckResult
}
type entry struct {
	name    string
	checker Checker
	running bool
}
type Registry struct {
	cfg     Config
	mu      sync.Mutex
	checks  []*entry
	frozen  bool
	active  int
	changed chan struct{}
}

func New(cfg Config) (*Registry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Registry{cfg: cfg, changed: make(chan struct{})}, nil
}
func (r *Registry) Register(name string, c Checker) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrRegistryFrozen
	}
	if len(name) == 0 || len(name) > 128 {
		return fmt.Errorf("%w: name length must be 1..128 bytes", ErrInvalidCall)
	}
	for i := range len(name) {
		b := name[i]
		allowed := b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '.' || b == '-'
		if !allowed {
			return fmt.Errorf("%w: name contains an unsupported byte", ErrInvalidCall)
		}
	}
	if c == nil {
		return fmt.Errorf("%w: checker is nil", ErrInvalidCall)
	}
	value := reflect.ValueOf(c)
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Func) && value.IsNil() {
		return fmt.Errorf("%w: checker is nil", ErrInvalidCall)
	}
	for _, existing := range r.checks {
		if existing.name == name {
			return ErrDuplicateCheck
		}
	}
	if len(r.checks) >= r.cfg.MaxChecks {
		return ErrTooManyChecks
	}
	r.checks = append(r.checks, &entry{name: name, checker: c})
	return nil
}
func (r *Registry) Live() bool { return true }

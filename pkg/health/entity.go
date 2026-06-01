package health

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	DefaultTimeout = 5 * time.Second
)

const (
	StatusUp   Status = "up"
	StatusDown Status = "down"
)

const (
	HealthStatusHealthy   = "healthy"
	HealthStatusUnhealthy = "unhealthy"
	CheckStatusOK         = "ok"
	CheckStatusError      = "error"
)

// HealthResponse is the unified response shape for GET /health.
type HealthResponse struct {
	Status    string        `json:"status"`
	Checks    []CheckResult `json:"checks"`
	LatencyMs int64         `json:"latency_ms"`
	Timestamp time.Time     `json:"timestamp"`
}

// CheckResult is a single dependency's result within HealthResponse.
type CheckResult struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

// Status represents a dependency's health state.
type Status string

// Checker defines a single dependency health check.
type Checker interface {
	Check(ctx context.Context) error
}

// DependencyStatus is the result of one checker run.
type DependencyStatus struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// HealthStatus is the full snapshot returned by GetStatus.
type HealthStatus struct {
	Status       Status             `json:"status"`
	Timestamp    time.Time          `json:"timestamp"`
	Dependencies []DependencyStatus `json:"dependencies"`
}

// Config holds HealthService settings.
type Config struct {
	Timeout       time.Duration `mapstructure:"timeout" json:"timeout"`
	EnableLogging bool          `mapstructure:"enable_logging" json:"enable_logging"`
}

// Service is the read-only interface consumed by the HTTP handler and callers.
type Service interface {
	IsLive() bool
	IsReady() bool
	GetStatus() HealthStatus
}

type namedChecker struct {
	name    string
	checker Checker
}

// HealthService executes registered checkers and aggregates their results.
type HealthService struct {
	checkers []namedChecker
	cfg      Config
	log      logger.Service
}

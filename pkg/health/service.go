package health

import (
	"context"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

var _ Service = (*HealthService)(nil)

// NewService creates a HealthService ready to accept checkers.
// Call Register to add checkers, then mount via NewHTTPHandler.
func NewService(cfg Config, log logger.Service) *HealthService {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &HealthService{
		checkers: make([]namedChecker, 0),
		cfg:      cfg,
		log:      log,
	}
}

// Register adds a named checker and returns the service for chaining.
// It is safe to call concurrently with GetStatus.
func (hs *HealthService) Register(name string, checker Checker) *HealthService {
	hs.mu.Lock()
	hs.checkers = append(hs.checkers, namedChecker{name: name, checker: checker})
	hs.mu.Unlock()
	if hs.cfg.EnableLogging {
		hs.log.Debug(context.Background(), "health checker registered",
			map[string]interface{}{"checker": name})
	}
	return hs
}

// IsLive reports whether the process is running. Always true.
func (hs *HealthService) IsLive() bool {
	return true
}

// IsReady reports whether all dependencies are healthy.
func (hs *HealthService) IsReady(ctx context.Context) bool {
	return hs.GetStatus(ctx).Status == StatusUp
}

// checkResult carries one checker's outcome back to GetStatus over a channel,
// so checker goroutines never write shared state and cannot race the timeout.
type checkResult struct {
	idx int
	ds  DependencyStatus
}

// GetStatus runs all checkers concurrently and returns the aggregated result.
// It is bounded by ctx and the configured timeout, whichever fires first: any
// checker that has not returned by then is reported as down instead of blocking
// the caller. This makes the health endpoint's timeout a hard limit even when a
// checker ignores its context.
func (hs *HealthService) GetStatus(ctx context.Context) HealthStatus {
	ctx, cancel := context.WithTimeout(ctx, hs.cfg.Timeout)
	defer cancel()

	hs.mu.RLock()
	checkers := make([]namedChecker, len(hs.checkers))
	copy(checkers, hs.checkers)
	hs.mu.RUnlock()

	if len(checkers) == 0 {
		return HealthStatus{
			Status:       StatusUp,
			Timestamp:    time.Now(),
			Dependencies: []DependencyStatus{},
		}
	}

	// Buffered so every checker goroutine can send its result and exit even
	// after we returned on timeout; this bounds any leak to goroutines whose
	// checker ignores ctx (which Go cannot forcibly stop).
	resultCh := make(chan checkResult, len(checkers))

	for i, entry := range checkers {
		go func(idx int, name string, c Checker) {
			start := time.Now()
			err := c.Check(ctx)
			latency := time.Since(start).Milliseconds()

			ds := DependencyStatus{
				Name:      name,
				LatencyMs: latency,
				Status:    StatusUp,
			}
			if err != nil {
				ds.Status = StatusDown
				ds.Error = err.Error()
			}
			resultCh <- checkResult{idx: idx, ds: ds}
		}(i, entry.name, entry.checker)
	}

	deps := make([]DependencyStatus, len(checkers))
	filled := make([]bool, len(checkers))
	received := 0

collect:
	for received < len(checkers) {
		select {
		case r := <-resultCh:
			deps[r.idx] = r.ds
			filled[r.idx] = true
			received++
		case <-ctx.Done():
			break collect
		}
	}

	// Any checker that did not report before the deadline is marked down.
	for i, entry := range checkers {
		if !filled[i] {
			deps[i] = DependencyStatus{
				Name:   entry.name,
				Status: StatusDown,
				Error:  "health check timed out",
			}
		}
	}

	overall := StatusUp
	for _, d := range deps {
		if d.Status == StatusDown {
			overall = StatusDown
			break
		}
	}

	return HealthStatus{
		Status:       overall,
		Timestamp:    time.Now(),
		Dependencies: deps,
	}
}

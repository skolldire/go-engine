package health

import (
	"context"
	"sync"
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
func (hs *HealthService) Register(name string, checker Checker) *HealthService {
	hs.checkers = append(hs.checkers, namedChecker{name: name, checker: checker})
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
func (hs *HealthService) IsReady() bool {
	return hs.GetStatus().Status == StatusUp
}

// GetStatus runs all checkers concurrently and returns the aggregated result.
func (hs *HealthService) GetStatus() HealthStatus {
	ctx, cancel := context.WithTimeout(context.Background(), hs.cfg.Timeout)
	defer cancel()

	if len(hs.checkers) == 0 {
		return HealthStatus{
			Status:       StatusUp,
			Timestamp:    time.Now(),
			Dependencies: []DependencyStatus{},
		}
	}

	deps := make([]DependencyStatus, len(hs.checkers))
	var wg sync.WaitGroup

	for i, entry := range hs.checkers {
		wg.Add(1)
		go func(idx int, name string, c Checker) {
			defer wg.Done()
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
			deps[idx] = ds
		}(i, entry.name, entry.checker)
	}
	wg.Wait()

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

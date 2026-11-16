package health

import (
	"context"
	"time"
)

func (r *Registry) Ready(ctx context.Context) (Report, error) {
	report, err := r.evaluate(ctx)
	if err != nil {
		return report, err
	}
	if !report.Ready {
		return report, ErrNotReady
	}
	return report, nil
}
func (r *Registry) Check(ctx context.Context) (Report, error) { return r.evaluate(ctx) }
func (r *Registry) evaluate(parent context.Context) (Report, error) {
	r.mu.Lock()
	r.frozen = true
	checks := r.checks
	r.mu.Unlock()
	if parent == nil {
		return Report{}, ErrNilContext
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, r.cfg.Timeout)
	defer cancel()
	report := Report{Live: true, Ready: true, Checks: make([]CheckResult, len(checks))}
	pending := make([]<-chan CheckResult, len(checks))
	for i, e := range checks {
		pending[i], report.Checks[i] = r.admit(ctx, e, start)
	}
	for i, ch := range pending {
		if ch != nil {
			report.Checks[i] = collect(ctx, ch, checks[i].name, start)
		}
		if report.Checks[i].Status != "up" {
			report.Ready = false
		}
	}
	return report, nil
}
func (r *Registry) admit(ctx context.Context, e *entry, start time.Time) (<-chan CheckResult, CheckResult) {
	for {
		r.mu.Lock()
		if e.running {
			r.mu.Unlock()
			return nil, CheckResult{Name: e.name, Status: "down", Reason: "busy"}
		}
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return nil, timedOut(e.name, start, err)
		}
		if r.active < r.cfg.MaxParallel {
			r.active++
			e.running = true
			r.mu.Unlock()
			child, cancel := context.WithCancel(ctx)
			done := make(chan CheckResult, 1)
			go r.run(child, cancel, e, start, done)
			return done, CheckResult{}
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
	}
}
func collect(ctx context.Context, done <-chan CheckResult, name string, start time.Time) CheckResult {
	select {
	case result := <-done:
		return result
	default:
	}
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		select {
		case result := <-done:
			return result
		default:
			return timedOut(name, start, ctx.Err())
		}
	}
}
func timedOut(name string, start time.Time, err error) CheckResult {
	return CheckResult{Name: name, Status: "down", Reason: "timeout", Duration: time.Since(start), Err: err}
}
func (r *Registry) finish(e *entry, result CheckResult, done chan<- CheckResult) {
	r.mu.Lock()
	e.running = false
	r.active--
	close(r.changed)
	r.changed = make(chan struct{})
	r.mu.Unlock()
	done <- result
}

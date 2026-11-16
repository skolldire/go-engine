package health

import (
	"context"
	"runtime"
	"time"
)

func (r *Registry) run(ctx context.Context, cancel context.CancelFunc, e *entry, start time.Time, done chan<- CheckResult) {
	result := CheckResult{Name: e.name, Status: "up", Reason: "ok"}
	completed, returned := false, false
	var value any
	var stack []byte
	defer func() {
		if !completed {
			result.Status = "down"
			result.Reason = "panic"
			result.Err = &PanicError{Value: value, Stack: stack, Goexit: !returned}
		} else if err := ctx.Err(); err != nil {
			result.Status = "down"
			result.Reason = "timeout"
			if result.Err == nil {
				result.Err = err
			}
		}
		result.Duration = time.Since(start)
		cancel()
		r.finish(e, result, done)
	}()
	func() {
		defer func() {
			if !completed {
				value = recover()
				buffer := make([]byte, 64<<10)
				n := runtime.Stack(buffer, false)
				stack = buffer[:n:n]
			}
		}()
		if err := ctx.Err(); err != nil {
			result = timedOut(e.name, start, err)
			completed = true
			return
		}
		result.Err = e.checker.Check(ctx)
		completed = true
		if result.Err != nil {
			result.Status = "down"
			result.Reason = "failed"
		}
	}()
	returned = true
}

package task_executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

type mockLogger struct{}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]any)     {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)      {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]any)      {}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]any)      {}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (m *mockLogger) WrapError(err error, msg string) error                            { return err }
func (m *mockLogger) WithField(key string, value any) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]any) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                              { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                   { return nil }

type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordTaskExecution(ctx context.Context, taskID string, durationMs int, success bool, priority int) {
}

func TestApplyOptions(t *testing.T) {
	cfg := applyOptions()
	assert.NotNil(t, cfg)
}

func TestValidateWorkerCount(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"positive", 5, 5},
		{"zero", 0, 1},
		{"negative", -1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateWorkerCount(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorkerPool_EmptyTasks(t *testing.T) {
	tasks := map[string]Tasker{}
	results := WorkerPool(context.Background(), tasks, 1)
	assert.NotNil(t, results)
	assert.Equal(t, 0, len(results))
}

func TestWorkerPool_SingleTask_Success(t *testing.T) {
	tasks := map[string]Tasker{
		"task1": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				return "result", nil
			},
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	results := WorkerPool(context.Background(), tasks, 1,
		WithLogger(&mockLogger{}),
	)

	assert.NotNil(t, results)
	assert.Equal(t, 1, len(results))
	assert.NoError(t, results["task1"].Err)
	assert.Equal(t, "result", results["task1"].Res)
}

func TestWorkerPool_SingleTask_Error(t *testing.T) {
	testErr := errors.New("task error")
	tasks := map[string]Tasker{
		"task1": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				return "", testErr
			},
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	results := WorkerPool(context.Background(), tasks, 1)

	assert.NotNil(t, results)
	assert.Equal(t, 1, len(results))
	assert.Error(t, results["task1"].Err)
	assert.Equal(t, testErr, results["task1"].Err)
}

func TestWorkerPool_ContextCancelled(t *testing.T) {
	tasks := map[string]Tasker{
		"task1": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				time.Sleep(100 * time.Millisecond)
				return "result", nil
			},
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	results := WorkerPool(ctx, tasks, 1)
	assert.NotNil(t, results)
}

func TestWithTaskTimeout(t *testing.T) {
	opt := WithTaskTimeout(5 * time.Second)
	assert.NotNil(t, opt)
}

func TestWithResultTimeout(t *testing.T) {
	opt := WithResultTimeout(5 * time.Second)
	assert.NotNil(t, opt)
}

func TestWithLogger(t *testing.T) {
	opt := WithLogger(&mockLogger{})
	assert.NotNil(t, opt)
}

func TestWithMetricsCollector(t *testing.T) {
	opt := WithMetricsCollector(&mockMetricsCollector{})
	assert.NotNil(t, opt)
}

func TestWithResultCallback(t *testing.T) {
	callback := func(result Result) {}
	opt := WithResultCallback(callback)
	assert.NotNil(t, opt)
}

func TestWithPrioritySupport(t *testing.T) {
	opt := WithPrioritySupport(true)
	assert.NotNil(t, opt)
}

func TestNewTask(t *testing.T) {
	task := NewTask[string, string](
		func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
		"input",
		PriorityNormal,
	)
	assert.NotNil(t, task)
	assert.Equal(t, "input", task.Args)
	assert.Equal(t, PriorityNormal, task.priority)
}

// TestWorkerPool_PanicIsContained verifies that a task that panics does not
// crash the process: the panic is recovered in the task goroutine and surfaced
// as an ErrTaskPanic result.
func TestWorkerPool_PanicIsContained(t *testing.T) {
	tasks := map[string]Tasker{
		"boom": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				panic("kaboom")
			},
			Args:     "input",
			priority: PriorityNormal,
		},
		"ok": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				return "fine", nil
			},
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	results := WorkerPool(context.Background(), tasks, 2, WithLogger(&mockLogger{}))

	assert.Len(t, results, 2)
	assert.ErrorIs(t, results["boom"].Err, ErrTaskPanic)
	assert.NoError(t, results["ok"].Err)
	assert.Equal(t, "fine", results["ok"].Res)
}

// TestWorkerPool_TaskTimeout verifies a task that exceeds the task timeout
// yields an ErrTaskTimeout result instead of blocking the pool.
func TestWorkerPool_TaskTimeout(t *testing.T) {
	tasks := map[string]Tasker{
		"slow": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				// Cooperative task: respects ctx cancellation.
				<-ctx.Done()
				return "", ctx.Err()
			},
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	results := WorkerPool(context.Background(), tasks, 1,
		WithTaskTimeout(50*time.Millisecond),
		WithLogger(&mockLogger{}),
	)

	assert.Len(t, results, 1)
	assert.ErrorIs(t, results["slow"].Err, ErrTaskTimeout)
}

// TestWorkerPool_NonCooperativeTaskTimeoutDoesNotLeak verifies that when a task
// ignores ctx and outlives the timeout, the pool still returns promptly and the
// runaway goroutine does not block on the outcome channel (bounded leak).
func TestWorkerPool_NonCooperativeTaskTimeoutDoesNotLeak(t *testing.T) {
	done := make(chan struct{})
	tasks := map[string]Tasker{
		"stubborn": &Task[string, string]{
			Func: func(ctx context.Context, input string) (string, error) {
				// Ignores ctx; finishes shortly after the timeout.
				time.Sleep(150 * time.Millisecond)
				close(done)
				return "late", nil
			},
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	start := time.Now()
	results := WorkerPool(context.Background(), tasks, 1,
		WithTaskTimeout(30*time.Millisecond),
		WithLogger(&mockLogger{}),
	)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 120*time.Millisecond, "pool must return on timeout, not wait for the task")
	assert.ErrorIs(t, results["stubborn"].Err, ErrTaskTimeout)

	// The runaway goroutine must still be able to complete and send on the
	// buffered channel without blocking; wait for it so goleak stays clean.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runaway task goroutine did not finish")
	}
}

func TestWorkerPool_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	tasks := map[string]Tasker{
		"a": &Task[string, string]{
			Func:     func(ctx context.Context, input string) (string, error) { return "a", nil },
			Args:     "input",
			priority: PriorityNormal,
		},
		"b": &Task[string, string]{
			Func:     func(ctx context.Context, input string) (string, error) { return "b", nil },
			Args:     "input",
			priority: PriorityNormal,
		},
	}

	results := WorkerPool(context.Background(), tasks, 2, WithLogger(&mockLogger{}))
	assert.Len(t, results, 2)
}

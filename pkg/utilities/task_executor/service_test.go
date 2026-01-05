package task_executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
)

type mockLogger struct{}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{})  {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{})  {}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{})  {}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service { return m }
func (m *mockLogger) GetLogLevel() string { return "info" }
func (m *mockLogger) SetLogLevel(level string) error { return nil }

type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordTaskExecution(ctx context.Context, taskID string, durationMs int, success bool, priority int) {}

func TestApplyOptions(t *testing.T) {
	cfg := applyOptions()
	assert.NotNil(t, cfg)
}

func TestValidateWorkerCount(t *testing.T) {
	tests := []struct {
		name      string
		input     int
		expected  int
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
			Func:     func(ctx context.Context, input string) (string, error) {
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
			Func:     func(ctx context.Context, input string) (string, error) {
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
			Func:     func(ctx context.Context, input string) (string, error) {
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


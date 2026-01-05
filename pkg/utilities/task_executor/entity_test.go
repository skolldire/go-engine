package task_executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTask_SetPriority(t *testing.T) {
	task := &Task[string, string]{
		Func: func(ctx context.Context, input string) (string, error) {
			return input, nil
		},
		Args: "test",
	}

	task.SetPriority(PriorityHigh)
	assert.Equal(t, PriorityHigh, task.Priority())
}

func TestTask_Priority(t *testing.T) {
	task := &Task[string, string]{
		priority: PriorityNormal,
	}
	assert.Equal(t, PriorityNormal, task.Priority())
}

func TestTask_Execute_Success(t *testing.T) {
	task := &Task[string, string]{
		Func: func(ctx context.Context, input string) (string, error) {
			time.Sleep(1 * time.Millisecond) // Ensure duration > 0
			return "result", nil
		},
		Args: "input",
	}

	result, duration, err := task.Execute(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.GreaterOrEqual(t, duration, 0) // Duration can be 0 for very fast operations
}

func TestTask_Execute_Error(t *testing.T) {
	testErr := errors.New("test error")
	task := &Task[string, string]{
		Func: func(ctx context.Context, input string) (string, error) {
			time.Sleep(1 * time.Millisecond) // Ensure duration > 0
			return "", testErr
		},
		Args: "input",
	}

	result, duration, err := task.Execute(context.Background())
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.Equal(t, "", result)           // For string type, zero value is "", not nil
	assert.GreaterOrEqual(t, duration, 0) // Duration can be 0 for very fast operations
}

func TestTask_Execute_ContextCancelled(t *testing.T) {
	task := &Task[string, string]{
		Func: func(ctx context.Context, input string) (string, error) {
			return "result", nil
		},
		Args: "input",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, duration, err := task.Execute(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Empty(t, result) // result is string, not pointer
	assert.Equal(t, 0, duration)
}

func TestResult(t *testing.T) {
	result := Result{
		ID:        "test-id",
		Err:       nil,
		Res:       "result",
		Time:      100,
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Priority:  PriorityNormal,
	}

	assert.Equal(t, "test-id", result.ID)
	assert.NoError(t, result.Err)
	assert.Equal(t, "result", result.Res)
	assert.Equal(t, 100, result.Time)
	assert.Equal(t, PriorityNormal, result.Priority)
}

package task_executor

import (
	"context"
	"errors"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	PriorityLow      = 0
	PriorityNormal   = 1
	PriorityHigh     = 2
	PriorityCritical = 3
)

var (
	ErrTaskTimeout   = errors.New("tarea cancelada por timeout")
	ErrPoolCancelled = errors.New("pool de trabajadores cancelado")
	ErrTaskPanic     = errors.New("panic en ejecución de tarea")
)

type Option func(*config)

type Tasker interface {
	Execute(ctx context.Context) (result interface{}, duration int, err error)
	Priority() int
}

type MetricsCollector interface {
	RecordTaskExecution(ctx context.Context, taskID string, durationMs int, success bool, priority int)
}

type Task[I, O any] struct {
	Func     func(context.Context, I) (O, error)
	Args     I
	priority int
}

type Result struct {
	ID        string
	Err       error
	Res       interface{}
	Time      int
	StartTime time.Time
	EndTime   time.Time
	Priority  int
}

type taskItem struct {
	id   string
	task Tasker
}

type config struct {
	taskTimeout      time.Duration
	resultTimeout    time.Duration
	logger           logger.Service
	metricsCollector MetricsCollector
	onResultFunc     func(Result)
	usePriority      bool
}

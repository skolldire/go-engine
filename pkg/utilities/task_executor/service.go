package task_executor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

func (t *Task[I, O]) SetPriority(priority int) {
	t.priority = priority
}

func (t Task[I, O]) Priority() int {
	return t.priority
}

func (t Task[I, O]) Execute(ctx context.Context) (interface{}, int, error) {
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	start := time.Now()
	out, err := t.Func(ctx, t.Args)
	duration := time.Since(start)

	return out, int(duration.Milliseconds()), err
}

func WorkerPool(ctx context.Context, tasks map[string]Tasker, numWorkers int, options ...Option) map[string]Result {
	cfg := applyOptions(options...)
	numWorkers = validateWorkerCount(numWorkers)

	taskChan := make(chan taskItem, len(tasks))
	resultChan := make(chan Result, len(tasks))

	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	wg := startWorkers(workerCtx, numWorkers, taskChan, resultChan, cfg)

	go distributeTasksByPriority(workerCtx, tasks, taskChan, cfg)

	go waitForWorkersToFinish(wg, resultChan, cfg, ctx)

	return collectResults(ctx, resultChan, tasks, cfg)
}

func applyOptions(options ...Option) *config {
	cfg := defaultConfig()
	for _, opt := range options {
		opt(cfg)
	}
	return cfg
}

func validateWorkerCount(numWorkers int) int {
	if numWorkers <= 0 {
		return 1
	}
	return numWorkers
}

func startWorkers(ctx context.Context, numWorkers int, taskChan <-chan taskItem, resultChan chan<- Result, cfg *config) *sync.WaitGroup {
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker-%d", i+1)
		go worker(workerID, ctx, &wg, taskChan, resultChan, cfg)
	}

	return &wg
}

func distributeTasksByPriority(ctx context.Context, tasks map[string]Tasker, taskChan chan<- taskItem, cfg *config) {
	defer close(taskChan)

	taskItems := sortTasksByPriority(tasks, cfg.usePriority)

	for _, item := range taskItems {
		if cfg.logger != nil {
			cfg.logger.Debug(ctx, "Encolando tarea", map[string]interface{}{
				"taskID":   item.id,
				"priority": item.task.Priority(),
			})
		}

		select {
		case taskChan <- item:
			if cfg.logger != nil {
				cfg.logger.Debug(ctx, "Tarea enviada a worker", map[string]interface{}{
					"taskID":   item.id,
					"priority": item.task.Priority(),
				})
			}
		case <-ctx.Done():
			if cfg.logger != nil {
				cfg.logger.Debug(ctx, "task distribution cancelled", nil)
			}
			return
		}
	}
}

func sortTasksByPriority(tasks map[string]Tasker, usePriority bool) []taskItem {
	taskItems := make([]taskItem, 0, len(tasks))

	for id, task := range tasks {
		taskItems = append(taskItems, taskItem{id: id, task: task})
	}

	if usePriority {
		sort.Slice(taskItems, func(i, j int) bool {
			return taskItems[i].task.Priority() > taskItems[j].task.Priority()
		})
	}

	return taskItems
}

func waitForWorkersToFinish(wg *sync.WaitGroup, resultChan chan<- Result, cfg *config, ctx context.Context) {
	wg.Wait()
	close(resultChan)
	if cfg.logger != nil {
		cfg.logger.Debug(ctx, "all tasks have been processed", nil)
	}
}

func collectResults(ctx context.Context, resultChan <-chan Result, tasks map[string]Tasker, cfg *config) map[string]Result {
	results := make(map[string]Result)

	var resultTimeout *time.Timer
	var resultTimeoutCh <-chan time.Time

	if cfg.resultTimeout > 0 {
		resultTimeout = time.NewTimer(cfg.resultTimeout)
		resultTimeoutCh = resultTimeout.C
		defer resultTimeout.Stop()
	}

	for {
		select {
		case res, ok := <-resultChan:
			if !ok {
				return results
			}

			results[res.ID] = res
			if cfg.onResultFunc != nil {
				cfg.onResultFunc(res)
			}

		case <-resultTimeoutCh:
			logTimeoutWarning(ctx, cfg, tasks, results)
			return results

		case <-ctx.Done():
			logCancellationWarning(ctx, cfg, tasks, results)
			return results
		}
	}
}

func logTimeoutWarning(ctx context.Context, cfg *config, tasks map[string]Tasker, results map[string]Result) {
	if cfg.logger != nil {
		cfg.logger.Warn(ctx, "timeout exceeded for result collection",
			map[string]interface{}{
				"totalTasks":       len(tasks),
				"collectedResults": len(results),
			})
	}
}

func logCancellationWarning(ctx context.Context, cfg *config, tasks map[string]Tasker, results map[string]Result) {
	if cfg.logger != nil {
		cfg.logger.Warn(ctx, "result collection cancelled",
			map[string]interface{}{
				"totalTasks":       len(tasks),
				"collectedResults": len(results),
			})
	}
}

func BatchWorkPool(ctx context.Context, tasks map[string]Tasker, numWorkers int, batchSize int, options ...Option) map[string]Result {
	if batchSize <= 0 {
		batchSize = 100
	}

	allResults := make(map[string]Result)
	batchTasks := make(map[string]Tasker, batchSize)
	count := 0

	for id, task := range tasks {
		batchTasks[id] = task
		count++

		if count >= batchSize || count == len(tasks) {
			results := WorkerPool(ctx, batchTasks, numWorkers, options...)

			for id, result := range results {
				allResults[id] = result
			}

			batchTasks = make(map[string]Tasker, batchSize)
			count = 0

			if ctx.Err() != nil {
				break
			}
		}
	}

	return allResults
}

func worker(workerID string, ctx context.Context, wg *sync.WaitGroup, taskChan <-chan taskItem, resultChan chan<- Result, cfg *config) {
	defer wg.Done()

	for {
		select {
		case taskItem, ok := <-taskChan:
			if !ok {
				return
			}

			var taskCtx context.Context
			var cancel context.CancelFunc

			if cfg.taskTimeout > 0 {
				taskCtx, cancel = context.WithTimeout(ctx, cfg.taskTimeout)
			} else {
				taskCtx, cancel = context.WithCancel(ctx)
			}

			result := safeExecuteTask(taskCtx, taskItem.task, taskItem.id, cfg, workerID)

			select {
			case resultChan <- result:
			case <-ctx.Done():
				if cfg.logger != nil {
					cfg.logger.Debug(ctx, "discarding result due to cancellation",
						map[string]interface{}{
							"taskID":   taskItem.id,
							"workerID": workerID,
						})
				}
			}

			cancel()

		case <-ctx.Done():
			return
		}
	}
}

func safeExecuteTask(ctx context.Context, task Tasker, id string, cfg *config, workerID string) Result {
	startTime := time.Now()

	result := Result{
		ID:        id,
		Time:      0,
		Err:       nil,
		StartTime: startTime,
		Priority:  task.Priority(),
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("panic during task execution: %v", r)
				if cfg.logger != nil {
					cfg.logger.Error(ctx, ErrTaskPanic, map[string]interface{}{
						"taskID":   id,
						"workerID": workerID,
						"panic":    r,
						"priority": task.Priority(),
					})
				}
				result.Err = fmt.Errorf("%w: %s", ErrTaskPanic, errMsg)
			}
		}()

		doneCh := make(chan struct{})

		go func() {
			defer close(doneCh)
			res, _, err := task.Execute(ctx)
			result.Res = res
			result.Err = err
		}()

		select {
		case <-doneCh:
		case <-ctx.Done():
			result.Err = ctx.Err()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.Err = fmt.Errorf("%w: %v", ErrTaskTimeout, ctx.Err())
			} else {
				result.Err = fmt.Errorf("%w: %v", ErrPoolCancelled, ctx.Err())
			}
		}
	}()

	result.EndTime = time.Now()
	result.Time = int(result.EndTime.Sub(startTime).Milliseconds())

	if cfg.metricsCollector != nil {
		cfg.metricsCollector.RecordTaskExecution(ctx, id, result.Time, result.Err == nil, task.Priority())
	}

	return result
}

func defaultConfig() *config {
	return &config{
		taskTimeout:   0,
		resultTimeout: 0,
		usePriority:   true,
	}
}

func WithTaskTimeout(timeout time.Duration) Option {
	return func(c *config) {
		c.taskTimeout = timeout
	}
}

func WithResultTimeout(timeout time.Duration) Option {
	return func(c *config) {
		c.resultTimeout = timeout
	}
}

func WithLogger(logger logger.Service) Option {
	return func(c *config) {
		c.logger = logger
	}
}

func WithMetricsCollector(collector MetricsCollector) Option {
	return func(c *config) {
		c.metricsCollector = collector
	}
}

func WithResultCallback(fn func(Result)) Option {
	return func(c *config) {
		c.onResultFunc = fn
	}
}

func WithPrioritySupport(enabled bool) Option {
	return func(c *config) {
		c.usePriority = enabled
	}
}

func NewTask[I, O any](fn func(context.Context, I) (O, error), args I, priority int) Task[I, O] {
	return Task[I, O]{
		Func:     fn,
		Args:     args,
		priority: priority,
	}
}

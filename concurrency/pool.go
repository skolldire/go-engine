package concurrency

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"
	"unicode/utf8"
)

type Stats struct {
	Accepted, Succeeded, Failed, Canceled, Rejected uint64
	Queued, Running, ActiveKeys                     int
	InFlightWeight                                  int64
	Closed                                          bool
}

type poolEntry[I, O any] struct {
	job     Job[I]
	ctx     *jobContext
	cancel  context.CancelCauseFunc
	future  *Future[O]
	result  Result[O]
	running bool
}
type keyState[I, O any] struct {
	running bool
	pending []*poolEntry[I, O]
	ready   bool
}

type Pool[I, O any] struct {
	mu       sync.Mutex
	cfg      PoolConfig
	task     Task[I, O]
	live     map[string]*poolEntry[I, O]
	queue    []*poolEntry[I, O]
	keys     map[string]*keyState[I, O]
	ready    []string
	stats    Stats
	sequence uint64
	mode     ShutdownMode
	changed  chan struct{}
	wake     chan struct{}
	work     chan *poolEntry[I, O]
	done     chan struct{}
	workers  sync.WaitGroup
}

func NewPool[I, O any](cfg PoolConfig, task Task[I, O]) (*Pool[I, O], error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if task == nil {
		return nil, invalid("task", "must not be nil")
	}
	p := &Pool[I, O]{
		cfg:     cfg,
		task:    task,
		live:    make(map[string]*poolEntry[I, O]),
		keys:    make(map[string]*keyState[I, O]),
		changed: make(chan struct{}),
		wake:    make(chan struct{}, 1),
		work:    make(chan *poolEntry[I, O], cfg.Workers),
		done:    make(chan struct{}),
	}
	p.workers.Add(cfg.Workers)
	for range cfg.Workers {
		go p.worker()
	}
	go p.coordinate()
	return p, nil
}

func (p *Pool[I, O]) Submit(ctx context.Context, job Job[I]) (*Future[O], error) {
	if p == nil || p.done == nil {
		return nil, invalid("pool", "must be initialized")
	}
	if ctx == nil {
		return p.reject(invalid("context", "must not be nil"))
	}
	if job.ID == "" || len(job.ID) > 128 || !utf8.ValidString(job.ID) {
		return p.reject(invalid("id", "must be UTF-8 with 1..128 bytes"))
	}
	if len(job.Key) > 256 || !utf8.ValidString(job.Key) || (p.cfg.Ordering == OrderingPerKey && job.Key == "") || (p.cfg.Ordering == OrderingUnordered && job.Key != "") {
		return p.reject(invalid("key", "invalid for the configured ordering"))
	}
	if job.Weight <= 0 {
		return p.reject(invalid("weight", "must be positive"))
	}
	if job.Weight > p.cfg.MaxJobWeight {
		return p.reject(ErrJobTooLarge)
	}
	var deadline <-chan time.Time
	var admissionEnd time.Time
	if p.cfg.Admission == AdmissionBlock && p.cfg.AdmissionTimeout > 0 {
		admissionEnd = time.Now().Add(p.cfg.AdmissionTimeout)
		timer := time.NewTimer(p.cfg.AdmissionTimeout)
		defer timer.Stop()
		deadline = timer.C
	}
	for {
		p.mu.Lock()
		if p.stats.Closed {
			p.mu.Unlock()
			return p.reject(ErrClosed)
		}
		if err := ctx.Err(); err != nil {
			p.mu.Unlock()
			return p.reject(err)
		}
		if !admissionEnd.IsZero() && !time.Now().Before(admissionEnd) {
			p.mu.Unlock()
			return p.reject(ErrAdmissionTimeout)
		}
		if _, exists := p.live[job.ID]; exists {
			p.mu.Unlock()
			return p.reject(ErrDuplicateID)
		}
		p.reconcileCancellationLocked()
		p.scheduleLocked()
		keyQueue := p.keys[job.Key]
		canRunImmediately := p.stats.Running < p.cfg.Workers && (p.cfg.Ordering == OrderingUnordered || keyQueue == nil || !keyQueue.running && len(keyQueue.pending) == 0)
		hasQueueCapacity := canRunImmediately || p.stats.Queued < p.cfg.QueueCapacity
		hasKeyCapacity := p.cfg.Ordering == OrderingUnordered || (keyQueue != nil || len(p.keys) < p.cfg.MaxActiveKeys) && (canRunImmediately || keyQueue == nil || len(keyQueue.pending) < p.cfg.PerKeyCapacity)
		hasWeightCapacity := job.Weight <= p.cfg.MaxInFlightWeight-p.stats.InFlightWeight
		if hasQueueCapacity && hasKeyCapacity && hasWeightCapacity {
			ectx := newJobContext(ctx)
			cancel := context.CancelCauseFunc(ectx.cancelCause)
			future := &Future[O]{done: make(chan struct{})}
			p.sequence++
			entry := &poolEntry[I, O]{
				job:    job,
				ctx:    ectx,
				cancel: cancel,
				future: future,
				result: Result[O]{
					ID:         job.ID,
					Key:        job.Key,
					Sequence:   p.sequence,
					AcceptedAt: time.Now(),
				},
			}
			future.cancel = func(cause error) {
				p.mu.Lock()
				if p.live[job.ID] == entry {
					entry.ctx.syncParent()
					entry.cancel(cause)
					p.reconcileCancellationLocked()
					p.scheduleLocked()
					p.notifySchedulerLocked()
				}
				p.mu.Unlock()
			}
			p.live[job.ID] = entry
			p.stats.Accepted++
			p.stats.InFlightWeight += job.Weight
			if p.cfg.Ordering == OrderingPerKey && keyQueue == nil {
				keyQueue = &keyState[I, O]{}
				p.keys[job.Key] = keyQueue
				p.stats.ActiveKeys = len(p.keys)
			}
			if canRunImmediately {
				p.dispatchLocked(entry)
			} else {
				p.stats.Queued++
				if p.cfg.Ordering == OrderingPerKey {
					keyQueue.pending = append(keyQueue.pending, entry)
					p.enqueueReadyKeyLocked(job.Key)
				} else {
					p.queue = append(p.queue, entry)
				}
			}
			p.notifySchedulerLocked()
			p.mu.Unlock()
			return future, nil
		}
		changed := p.changed
		p.mu.Unlock()
		if p.cfg.Admission == AdmissionReject {
			return p.reject(ErrSaturated)
		}
		select {
		case <-ctx.Done():
			return p.reject(ctx.Err())
		case <-deadline:
			if err := ctx.Err(); err != nil {
				return p.reject(err)
			}
			return p.reject(ErrAdmissionTimeout)
		case <-changed:
		}
	}
}

func (p *Pool[I, O]) reject(err error) (*Future[O], error) {
	p.mu.Lock()
	p.stats.Rejected++
	p.mu.Unlock()
	return nil, err
}
func (p *Pool[I, O]) notifySchedulerLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
func (p *Pool[I, O]) enqueueReadyKeyLocked(key string) {
	keyQueue := p.keys[key]
	if keyQueue != nil && !keyQueue.running && len(keyQueue.pending) > 0 && !keyQueue.ready {
		keyQueue.ready = true
		p.ready = append(p.ready, key)
	}
}
func (p *Pool[I, O]) dispatchLocked(entry *poolEntry[I, O]) {
	entry.running = true
	p.stats.Running++
	if p.cfg.Ordering == OrderingPerKey {
		p.keys[entry.job.Key].running = true
	}
	p.work <- entry
}
func (p *Pool[I, O]) scheduleLocked() {
	for p.stats.Running < p.cfg.Workers {
		var entry *poolEntry[I, O]
		if p.cfg.Ordering == OrderingUnordered {
			if len(p.queue) == 0 {
				return
			}
			entry = p.queue[0]
			p.queue[0] = nil
			p.queue = p.queue[1:]
		} else {
			if len(p.ready) == 0 {
				return
			}
			key := p.ready[0]
			p.ready[0] = ""
			p.ready = p.ready[1:]
			keyQueue := p.keys[key]
			if keyQueue == nil {
				continue
			}
			keyQueue.ready = false
			if keyQueue.running || len(keyQueue.pending) == 0 {
				continue
			}
			entry = keyQueue.pending[0]
			keyQueue.pending[0] = nil
			keyQueue.pending = keyQueue.pending[1:]
		}
		p.stats.Queued--
		p.dispatchLocked(entry)
	}
}

func (p *Pool[I, O]) reconcileCancellationLocked() {
	for _, entry := range p.live {
		entry.ctx.syncParent()
	}
	if p.cfg.Ordering == OrderingUnordered {
		keep := p.queue[:0]
		for _, entry := range p.queue {
			if entry.ctx.Err() != nil {
				p.stats.Queued--
				p.finalizeLocked(entry, zero[O](), nil, nil)
			} else {
				keep = append(keep, entry)
			}
		}
		clear(p.queue[len(keep):])
		p.queue = keep
		return
	}
	for key, keyQueue := range p.keys {
		keep := keyQueue.pending[:0]
		for _, entry := range keyQueue.pending {
			if entry.ctx.Err() != nil {
				p.stats.Queued--
				p.finalizeLocked(entry, zero[O](), nil, nil)
			} else {
				keep = append(keep, entry)
			}
		}
		clear(keyQueue.pending[len(keep):])
		keyQueue.pending = keep
		if !keyQueue.running && len(keyQueue.pending) == 0 {
			delete(p.keys, key)
		}
	}
	ready := p.ready[:0]
	for _, key := range p.ready {
		if keyQueue := p.keys[key]; keyQueue != nil {
			if !keyQueue.running && len(keyQueue.pending) > 0 {
				ready = append(ready, key)
			} else {
				keyQueue.ready = false
			}
		}
	}
	clear(p.ready[len(ready):])
	p.ready = ready
	p.stats.ActiveKeys = len(p.keys)
}
func zero[T any]() T { var z T; return z }

func (p *Pool[I, O]) worker() {
	defer p.workers.Done()
	for entry := range p.work {
		p.mu.Lock()
		entry.ctx.syncParent()
		if entry.ctx.Err() != nil {
			p.finishRunningLocked(entry, zero[O](), nil, nil)
			p.mu.Unlock()
			continue
		}
		entry.result.StartedAt = time.Now()
		var taskCtx context.Context = entry.ctx
		cancel := func() {}
		if p.cfg.TaskTimeout > 0 {
			taskCtx, cancel = context.WithTimeout(taskCtx, p.cfg.TaskTimeout)
		}
		p.mu.Unlock()
		value, panicked, err := executeTask(taskCtx, entry.job.Value, p.task)
		p.mu.Lock()
		cause := context.Cause(taskCtx)
		if cause != nil {
			if context.Cause(entry.ctx) == nil {
				entry.cancel(cause)
			}
			err = errors.Join(err, cause)
		}
		p.finishRunningLocked(entry, value, err, panicked)
		p.mu.Unlock()
		cancel()
	}
}
func executeTask[I, O any](ctx context.Context, in I, task Task[I, O]) (out O, panicked *PanicError, err error) {
	defer func() {
		if v := recover(); v != nil {
			panicked = panicError(v)
		}
	}()
	out, err = task(ctx, in)
	return
}
func (p *Pool[I, O]) finishRunningLocked(entry *poolEntry[I, O], value O, err error, panicked *PanicError) {
	p.stats.Running--
	if p.cfg.Ordering == OrderingPerKey {
		p.keys[entry.job.Key].running = false
	}
	p.finalizeLocked(entry, value, err, panicked)
	if p.cfg.Ordering == OrderingPerKey {
		keyQueue := p.keys[entry.job.Key]
		if p.cfg.KeyFailure == KeyFailureCancelPending && entry.result.State == StateFailed {
			for _, pending := range keyQueue.pending {
				pending.cancel(ErrKeyAborted)
				p.stats.Queued--
				p.finalizeLocked(pending, zero[O](), nil, nil)
			}
			clear(keyQueue.pending)
			keyQueue.pending = nil
		}
		if len(keyQueue.pending) == 0 {
			delete(p.keys, entry.job.Key)
		} else {
			p.enqueueReadyKeyLocked(entry.job.Key)
		}
		p.stats.ActiveKeys = len(p.keys)
	}
	p.reconcileCancellationLocked()
	p.scheduleLocked()
	p.notifySchedulerLocked()
}
func (p *Pool[I, O]) finalizeLocked(entry *poolEntry[I, O], value O, err error, panicked *PanicError) {
	entry.ctx.syncParent()
	cause := context.Cause(entry.ctx)
	entry.result.Value = value
	entry.result.FinishedAt = time.Now()
	switch {
	case panicked != nil:
		entry.result.State = StateFailed
		entry.result.Err = errors.Join(panicked, err, cause)
		p.stats.Failed++
	case cause != nil:
		entry.result.State = StateCanceled
		entry.result.Err = errors.Join(err, cause)
		p.stats.Canceled++
	case err != nil:
		entry.result.State = StateFailed
		entry.result.Err = err
		p.stats.Failed++
	default:
		entry.result.State = StateSucceeded
		p.stats.Succeeded++
	}
	delete(p.live, entry.job.ID)
	p.stats.InFlightWeight -= entry.job.Weight
	entry.future.mu.Lock()
	entry.future.result = entry.result
	entry.future.cancel = nil
	close(entry.future.done)
	entry.future.mu.Unlock()
	entry.cancel(nil)
	entry.job.Value = zero[I]()
	entry.result.Value = zero[O]()
	entry.future = nil
}

func (p *Pool[I, O]) coordinate() {
	for {
		p.mu.Lock()
		p.reconcileCancellationLocked()
		p.scheduleLocked()
		if p.stats.Closed && len(p.live) == 0 {
			close(p.work)
			p.mu.Unlock()
			p.workers.Wait()
			close(p.done)
			return
		}
		cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p.wake)}}
		overflow := false
		add := func(done <-chan struct{}) {
			if done == nil {
				return
			}
			if len(cases) == 65535 {
				overflow = true
				return
			}
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)})
		}
		for _, entry := range p.live {
			if context.Cause(entry.ctx) == nil {
				add(entry.ctx.parent.Done())
			}
			if !entry.running {
				add(entry.ctx.Done())
			}
			if overflow {
				break
			}
		}

		var scan *time.Timer
		if overflow {
			scan = time.NewTimer(time.Millisecond)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(scan.C)})
		}
		p.mu.Unlock()
		reflect.Select(cases)
		if scan != nil {
			scan.Stop()
		}

		p.mu.Lock()
		p.notifyAdmissionLocked()
		p.mu.Unlock()
	}
}
func (p *Pool[I, O]) notifyAdmissionLocked() { close(p.changed); p.changed = make(chan struct{}) }

func (p *Pool[I, O]) Shutdown(ctx context.Context, mode ShutdownMode) error {
	if p == nil || p.done == nil || ctx == nil {
		return invalid("shutdown", "requires initialized pool and non-nil context")
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	if mode != ShutdownDrain && mode != ShutdownCancel {
		return invalid("shutdown_mode", "must be drain or cancel")
	}
	p.mu.Lock()
	p.stats.Closed = true
	if mode == ShutdownCancel || p.mode == ShutdownCancel {
		p.mode = ShutdownCancel
		for _, entry := range p.live {
			entry.cancel(context.Canceled)
		}
	} else {
		p.mode = ShutdownDrain
	}
	p.reconcileCancellationLocked()
	p.scheduleLocked()
	p.notifySchedulerLocked()
	p.mu.Unlock()
	select {
	case <-p.done:
		return nil
	default:
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool[I, O]) Done() <-chan struct{} {
	if p == nil {
		return nil
	}
	return p.done
}

func (p *Pool[I, O]) Stats() Stats { p.mu.Lock(); defer p.mu.Unlock(); return p.stats }

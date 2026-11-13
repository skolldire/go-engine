package concurrency

import (
	"context"
	"sync"
)

type coordinatedParent struct {
	context.Context
	mu       sync.Mutex
	callback func()
}

func (p *coordinatedParent) AfterFunc(callback func()) func() bool {
	p.mu.Lock()
	p.callback = callback
	p.mu.Unlock()
	return func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.callback == nil {
			return false
		}
		p.callback = nil
		return true
	}
}
func (p *coordinatedParent) syncParent() {
	p.mu.Lock()
	var callback func()
	if p.Err() != nil {
		callback = p.callback
		p.callback = nil
	}
	p.mu.Unlock()
	if callback != nil {
		callback()
	}
}

type jobContext struct {
	context.Context
	parent       *coordinatedParent
	nativeCancel context.CancelCauseFunc
}

func newJobContext(parent context.Context) *jobContext {
	bridge := &coordinatedParent{Context: parent}
	owned, cancel := context.WithCancelCause(bridge)
	return &jobContext{Context: owned, parent: bridge, nativeCancel: cancel}
}
func (c *jobContext) cancelCause(cause error) {
	if cause == nil {
		cause = context.Canceled
	}
	c.nativeCancel(cause)
}
func (c *jobContext) syncParent() { c.parent.syncParent() }

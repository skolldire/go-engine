package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"

	"github.com/skolldire/go-engine/router"
)

type ownedListener struct {
	net.Listener
	once sync.Once
	err  error
}

func (l *ownedListener) Close() error { l.once.Do(func() { l.err = l.Listener.Close() }); return l.err }
func sameListener(a, b net.Listener) bool {
	if nilPointer(a) || nilPointer(b) {
		return false
	}
	x, y := reflect.ValueOf(a), reflect.ValueOf(b)
	return x.Kind() == reflect.Pointer && y.Kind() == reflect.Pointer && x.Type() == y.Type() && x.Pointer() == y.Pointer()
}
func closeListeners(api, ops net.Listener) error {
	var e1, e2 error
	if !nilPointer(api) {
		e1 = api.Close()
	}
	if !nilPointer(ops) && !sameListener(api, ops) {
		e2 = ops.Close()
	}
	return errors.Join(e1, e2)
}

// Run binds both listeners before serving. It owns only those listeners/servers,
// drains both on cancellation/failure and joins their errors. Execution is single-use.
func (a *Application) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: ctx", ErrInvalidCall)
	}
	if !a.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	if ctx.Err() != nil {
		return nil
	}
	api, e := net.Listen("tcp", a.cfg.Server.Listen)
	if e != nil {
		return e
	}
	var ops net.Listener
	if a.cfg.EnableOperations {
		ops, e = net.Listen("tcp", a.cfg.OperationsServer.Listen)
		if e != nil {
			return errors.Join(e, api.Close())
		}
	}
	return a.serve(ctx, api, ops)
}

// RunListeners transfers listener ownership even on invalid/repeated calls.
// Invalid calls do not consume the execution slot. Never reuse transferred listeners.
func (a *Application) RunListeners(ctx context.Context, api, ops net.Listener) error {
	if ctx != nil && a.started.Load() {
		return errors.Join(ErrAlreadyStarted, closeListeners(api, ops))
	}
	if ctx == nil || nilPointer(api) || a.cfg.EnableOperations == nilPointer(ops) || sameListener(api, ops) {
		return errors.Join(fmt.Errorf("%w: ctx or listeners", ErrInvalidCall), closeListeners(api, ops))
	}
	if !a.started.CompareAndSwap(false, true) {
		return errors.Join(ErrAlreadyStarted, closeListeners(api, ops))
	}
	return a.serve(ctx, api, ops)
}
func (a *Application) serve(ctx context.Context, api, ops net.Listener) (result error) {
	apiOwned := &ownedListener{Listener: api}
	var opsOwned *ownedListener
	if a.cfg.EnableOperations {
		opsOwned = &ownedListener{Listener: ops}
	}
	defer func() {
		var e error
		if opsOwned != nil {
			e = opsOwned.Close()
		}
		result = errors.Join(result, apiOwned.Close(), e)
	}()
	if ctx.Err() != nil {
		return nil
	}
	shared, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 2)
	n := 1
	go func() { done <- router.ServeListener(shared, a.cfg.Server, apiOwned, a.api) }()
	if opsOwned != nil {
		n++
		go func() { done <- router.ServeListener(shared, a.cfg.OperationsServer, opsOwned, a.ops) }()
	}
	for range n {
		result = errors.Join(result, <-done)
		cancel()
	}
	return result
}

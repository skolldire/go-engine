package httpapi

import (
	"net/http"
	"reflect"
	"sync/atomic"

	"github.com/skolldire/go-engine/router"
	"github.com/skolldire/go-engine/router/middleware"
	"github.com/skolldire/go-engine/router/operations"
)

// Application is a frozen handler assembly with a single HTTP execution lifetime.
// Do not copy it after New. Handler use remains independent of Run.
type Application struct {
	cfg      Config
	api, ops http.Handler
	started  atomic.Bool
}

func nilPointer(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return r.Kind() == reflect.Pointer && r.IsNil()
}

// New builds/finalizes handlers without running them or owning external resources.
// Middleware factories execute here; their panics propagate without recovery.
func New(c Config, d Dependencies) (*Application, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	c.Router = copyPolicy(c.Router)
	c.OperationsRouter = copyPolicy(c.OperationsRouter)
	d.Routes = append([]Route(nil), d.Routes...)
	d.Middleware = append([]router.Middleware(nil), d.Middleware...)
	d.OperationsMiddleware = append([]router.Middleware(nil), d.OperationsMiddleware...)
	var chain []router.Middleware
	if c.RequestID.Enabled {
		mw, e := middleware.RequestID(middleware.RequestIDConfig{Header: c.RequestID.Header, AcceptIncoming: c.RequestID.AcceptIncoming, Generate: d.GenerateRequestID})
		if e != nil {
			return nil, delegated("RequestID", e)
		}
		chain = append(chain, mw)
	}
	if c.EnableRecovery {
		mw, e := middleware.Recover(d.OnPanic)
		if e != nil {
			return nil, delegated("OnPanic", e)
		}
		chain = append(chain, mw)
	}
	if c.RequestTimeout > 0 {
		mw, e := middleware.Timeout(c.RequestTimeout)
		if e != nil {
			return nil, delegated("RequestTimeout", e)
		}
		chain = append(chain, mw)
	}
	for _, mw := range d.Middleware {
		if mw == nil {
			return nil, &ConfigError{"Middleware", "nil factory"}
		}
	}
	var ops http.Handler
	var opPolicy *router.Policy
	if c.EnableOperations {
		var e error
		ops, e = operations.New(c.Operations, operations.Dependencies{Health: d.Health, Metrics: d.Metrics})
		if e != nil {
			return nil, delegated("Operations.Dependencies", e)
		}
		opPolicy, e = router.NewPolicy(c.OperationsRouter)
		if e != nil {
			return nil, delegated("OperationsRouter", e)
		}
		for _, mw := range d.OperationsMiddleware {
			if mw == nil {
				return nil, &ConfigError{"OperationsMiddleware", "nil factory"}
			}
		}
	}
	mux, e := router.New(c.Router)
	if e != nil {
		return nil, delegated("Router", e)
	}
	for _, r := range d.Routes {
		if e := mux.Handle(r.Method, r.Host, r.Pattern, r.Handler); e != nil {
			return nil, delegated("Routes", e)
		}
	}
	chain = append(chain, d.Middleware...)
	if e = mux.Use(chain...); e != nil {
		return nil, delegated("Middleware", e)
	}
	if e = mux.Freeze(); e != nil {
		return nil, delegated("Middleware", e)
	}
	if c.EnableOperations {
		for i := len(d.OperationsMiddleware) - 1; i >= 0; i-- {
			ops = d.OperationsMiddleware[i](ops)
			if nilPointer(ops) {
				return nil, &ConfigError{"OperationsMiddleware", "factory returned nil"}
			}
		}
		ops = opPolicy.Wrap(ops)
	}
	return &Application{cfg: c, api: http.HandlerFunc(mux.ServeHTTP), ops: ops}, nil
}

// Handler returns the frozen API handler, safe when borrowed callbacks are safe.
func (a *Application) Handler() http.Handler { return a.api }

// OperationsHandler returns the isolated handler, or nil when disabled.
func (a *Application) OperationsHandler() http.Handler { return a.ops }

// ServerConfig returns API serving policy by value.
func (a *Application) ServerConfig() router.ServerConfig { return a.cfg.Server }

// OperationsServerConfig returns operations serving policy and its enabled flag.
func (a *Application) OperationsServerConfig() (router.ServerConfig, bool) {
	return a.cfg.OperationsServer, a.cfg.EnableOperations
}

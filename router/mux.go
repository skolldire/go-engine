package router

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type route struct {
	method, host, pattern string
	handler               http.Handler
}
type Mux struct {
	mu         sync.RWMutex
	policy     *Policy
	routes     []route
	middleware []Middleware
	frozen     bool
	handler    http.Handler
	freezeDone chan struct{}
}

func New(cfg Config) (*Mux, error) {
	p, e := NewPolicy(cfg)
	if e != nil {
		return nil, e
	}
	return &Mux{policy: p}, nil
}

// validateRoutes contains the sole recovery boundary for ServeMux registration:
// it is a throwaway mux, never the live request dispatcher.
func validateRoutes(routes []route) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: conflicting or invalid pattern", ErrInvalidCall)
		}
	}()
	buckets := map[string]*http.ServeMux{}
	for _, r := range routes {
		mux := buckets[r.host]
		if mux == nil {
			mux = http.NewServeMux()
			buckets[r.host] = mux
		}
		mux.Handle(routePattern(r), r.handler)
	}
	return nil
}
func routePattern(r route) string {
	if r.method == "" {
		return r.pattern
	}
	return r.method + " " + r.pattern
}
func (m *Mux) Handle(method, host, pattern string, h http.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frozen {
		return ErrFrozen
	}
	if method != "" && (!token(method) || method != strings.ToUpper(method)) {
		return fmt.Errorf("%w: method", ErrInvalidCall)
	}
	if nilPointer(h) || !strings.HasPrefix(pattern, "/") || strings.ContainsAny(pattern, " \t\r\n") {
		return fmt.Errorf("%w: pattern or handler", ErrInvalidCall)
	}
	if host != "" {
		normalized, e := configHost(host)
		if e != nil {
			return fmt.Errorf("%w: host", ErrInvalidCall)
		}
		host = normalized
		allowed := false
		for _, rule := range m.policy.hosts {
			if rule == host || !strings.HasPrefix(host, "*.") && hostMatches(rule, host) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: host not allowed", ErrInvalidCall)
		}
	}
	for _, r := range m.routes {
		if r.method == method && r.host == host && r.pattern == pattern {
			return ErrDuplicateRoute
		}
	}
	routes := append(append([]route(nil), m.routes...), route{method, host, pattern, h})
	if err := validateRoutes(routes); err != nil {
		return err
	}
	m.routes = routes
	return nil
}
func (m *Mux) Use(mw ...Middleware) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frozen {
		return ErrFrozen
	}
	for _, v := range mw {
		if v == nil {
			return fmt.Errorf("%w: middleware", ErrInvalidCall)
		}
	}
	m.middleware = append(m.middleware, mw...)
	return nil
}
func (m *Mux) Freeze() error {
	m.mu.Lock()
	if m.frozen {
		done := m.freezeDone
		m.mu.Unlock()
		<-done
		return nil
	}
	m.frozen = true
	m.freezeDone = make(chan struct{})
	routes := m.routes
	middleware := m.middleware
	m.mu.Unlock()
	defer close(m.freezeDone)
	buckets := map[string]*http.ServeMux{}
	for _, r := range routes {
		b := buckets[r.host]
		if b == nil {
			b = http.NewServeMux()
			buckets[r.host] = b
		}
		b.Handle(routePattern(r), r.handler)
	}
	var dispatch http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _ := hostName(r.Host)
		keys := []string{host}
		for _, rule := range m.policy.hosts {
			if strings.HasPrefix(rule, "*.") && hostMatches(rule, host) {
				keys = append(keys, rule)
			}
		}
		keys = append(keys, "")
		for _, key := range keys {
			if b := buckets[key]; b != nil {
				b.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
	for i := len(middleware) - 1; i >= 0; i-- {
		dispatch = middleware[i](dispatch)
		if nilPointer(dispatch) {
			return fmt.Errorf("%w: middleware returned nil handler", ErrInvalidCall)
		}
	}
	handler := m.policy.Wrap(dispatch)
	m.mu.Lock()
	m.handler = handler
	m.routes = nil
	m.middleware = nil
	m.mu.Unlock()
	return nil
}
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	h := m.handler
	m.mu.RUnlock()
	if h == nil {
		w.WriteHeader(503)
		return
	}
	h.ServeHTTP(w, r)
}

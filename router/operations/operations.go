package operations

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/pprof"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/skolldire/go-engine/health"
)

var ErrInvalidConfig = errors.New("invalid operations configuration")

type ConfigError struct{ Field, Reason string }

func (e *ConfigError) Error() string { return "operations: " + e.Field + ": " + e.Reason }
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

type Config struct {
	Ping               bool          `mapstructure:"ping" json:"ping"`
	Health             bool          `mapstructure:"health" json:"health"`
	Pprof              bool          `mapstructure:"pprof" json:"pprof"`
	MaxProfileDuration time.Duration `mapstructure:"max_profile_duration" json:"max_profile_duration"`
}
type Health interface {
	Live() bool
	Ready(context.Context) (health.Report, error)
	Check(context.Context) (health.Report, error)
}
type Dependencies struct {
	Health  Health
	Metrics http.Handler
}

func DefaultConfig() Config {
	return Config{Ping: true, Health: true, MaxProfileDuration: 30 * time.Second}
}
func (c Config) Validate() error {
	if c.MaxProfileDuration <= 0 {
		return &ConfigError{"MaxProfileDuration", "must be positive"}
	}
	return nil
}
func nilPointer(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return r.Kind() == reflect.Pointer && r.IsNil()
}

type checkDTO struct {
	Name       string        `json:"name"`
	Status     health.Status `json:"status"`
	Reason     health.Reason `json:"reason"`
	DurationMS int64         `json:"duration_ms"`
}
type reportDTO struct {
	Ready  bool       `json:"ready"`
	Checks []checkDTO `json:"checks"`
}

func publicReport(report health.Report) reportDTO {
	out := reportDTO{Ready: report.Ready, Checks: make([]checkDTO, len(report.Checks))}
	for i, c := range report.Checks {
		out.Checks[i] = checkDTO{c.Name, c.Status, c.Reason, c.Duration.Milliseconds()}
	}
	return out
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, e := json.Marshal(v)
	if e != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
func New(cfg Config, deps Dependencies) (http.Handler, error) {
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	if cfg.Health && nilPointer(deps.Health) {
		return nil, &ConfigError{"Health", "is required"}
	}
	mux := http.NewServeMux()
	if cfg.Ping {
		mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			}{"ok", "pong"})
		})
	}
	if cfg.Health {
		mux.HandleFunc("GET /live", func(w http.ResponseWriter, r *http.Request) {
			live := deps.Health.Live()
			status := 200
			if !live {
				status = 503
			}
			writeJSON(w, status, struct {
				Live bool `json:"live"`
			}{live})
		})
		mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
			report, e := deps.Health.Ready(r.Context())
			status := 200
			if errors.Is(e, health.ErrNotReady) {
				status = 503
			} else if e != nil {
				status = 500
			}
			if status == 500 {
				writeJSON(w, 500, struct {
					Error string `json:"error"`
				}{"internal_error"})
				return
			}
			writeJSON(w, status, publicReport(report))
		})
		mux.HandleFunc("GET /deps", func(w http.ResponseWriter, r *http.Request) {
			report, e := deps.Health.Check(r.Context())
			status := 200
			if e != nil {
				status = 500
			}
			if status == 500 {
				writeJSON(w, 500, struct {
					Error string `json:"error"`
				}{"internal_error"})
				return
			}
			writeJSON(w, status, struct {
				reportDTO
				Live bool `json:"live"`
			}{publicReport(report), deps.Health.Live()})
		})
	}
	if !nilPointer(deps.Metrics) {
		mux.Handle("GET /metrics", deps.Metrics)
	}
	if cfg.Pprof {
		guard := func(next http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if !profileSeconds(r, cfg.MaxProfileDuration) {
					writeJSON(w, 400, struct {
						Error string `json:"error"`
					}{"invalid_seconds"})
					return
				}
				next(w, r)
			}
		}
		mux.HandleFunc("GET /debug/pprof/", guard(pprof.Index))
		mux.HandleFunc("GET /debug/pprof/cmdline", guard(pprof.Cmdline))
		mux.HandleFunc("GET /debug/pprof/profile", guard(pprof.Profile))
		mux.HandleFunc("GET /debug/pprof/symbol", guard(pprof.Symbol))
		mux.HandleFunc("GET /debug/pprof/trace", guard(pprof.Trace))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			mux.ServeHTTP(headWriter{w}, r)
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}
func profileSeconds(r *http.Request, max time.Duration) bool {
	q := r.URL.Query()
	values, exists := q["seconds"]
	limit := float64(max) / float64(time.Second)
	if exists {
		if len(values) != 1 {
			return false
		}
		n, e := strconv.ParseFloat(values[0], 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > limit {
			return false
		}
		if !strings.HasSuffix(r.URL.Path, "/trace") && n != math.Trunc(n) {
			return false
		}
		return true
	}
	if strings.HasSuffix(r.URL.Path, "/profile") {
		n := math.Min(30, math.Floor(limit))
		if n < 1 {
			return false
		}
		q.Set("seconds", strconv.FormatFloat(n, 'f', 0, 64))
		r.URL.RawQuery = q.Encode()
	}
	if strings.HasSuffix(r.URL.Path, "/trace") {
		q.Set("seconds", strconv.FormatFloat(math.Min(1, limit), 'g', -1, 64))
		r.URL.RawQuery = q.Encode()
	}
	return true
}

type headWriter struct{ http.ResponseWriter }

func (w headWriter) Write(b []byte) (int, error) { return len(b), nil }

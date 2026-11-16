package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/skolldire/go-engine/router"
)

type RequestIDConfig struct {
	Header         string
	AcceptIncoming bool
	Generate       func() string
}
type idKey struct{}

func RequestIDFrom(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	s, ok := ctx.Value(idKey{}).(string)
	return s, ok
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
func RequestID(cfg RequestIDConfig) (router.Middleware, error) {
	if cfg.Header == "" {
		cfg.Header = "X-Request-ID"
	}
	for _, c := range cfg.Header {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)
		if !valid {
			return nil, &router.ConfigError{Field: "Header", Reason: "invalid HTTP token"}
		}
	}
	if cfg.Generate == nil {
		cfg.Generate = func() string {
			var b [16]byte
			if _, e := rand.Read(b[:]); e != nil {
				panic("request ID generation failed")
			}
			return hex.EncodeToString(b[:])
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if cfg.AcceptIncoming {
				id = r.Header.Get(cfg.Header)
			}
			if !validID(id) {
				id = cfg.Generate()
				if !validID(id) {
					panic("invalid generated request ID")
				}
			}
			w.Header().Set(cfg.Header, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), idKey{}, id)))
		})
	}, nil
}
func Recover(onPanic func(http.ResponseWriter, *http.Request, any)) (router.Middleware, error) {
	if onPanic == nil {
		return nil, &router.ConfigError{Field: "onPanic", Reason: "is required"}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { //nolint:errorlint // Exact net/http panic sentinel.
						panic(v)
					}
					onPanic(w, r, v)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}, nil
}
func Timeout(d time.Duration) (router.Middleware, error) {
	if d <= 0 {
		return nil, &router.ConfigError{Field: "d", Reason: "must be positive"}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

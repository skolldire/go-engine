package http

import (
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"

	"github.com/skolldire/go-engine/auth/jwt"
)

func skip(r *nethttp.Request, paths []string) bool {
	p := r.URL.Path
	// Never canonicalize an ambiguous input into a public route. A downstream
	// router's redirects/rewrites must not turn an unauthenticated path public.
	if !canonicalPath(p) || r.URL.RawPath != "" || strings.Contains(r.URL.EscapedPath(), "%") {
		return false
	}
	for _, entry := range paths {
		if strings.HasSuffix(entry, "/") {
			if strings.HasPrefix(p, entry) {
				return true
			}
		} else if p == entry {
			return true
		}
	}
	return false
}
func bearer(r *nethttp.Request, header string) string {
	values := r.Header.Values(header)
	if len(values) != 1 {
		return ""
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	token = strings.Trim(token, " ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return ""
	}
	return token
}
func errorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, jwt.ErrKeySetUnavailable):
		return 503, "auth_unavailable"
	case errors.Is(err, jwt.ErrExpiredToken):
		return 401, "expired_token"
	case errors.Is(err, jwt.ErrInvalidToken):
		return 401, "invalid_token"
	case errors.Is(err, jwt.ErrForbidden):
		return 403, "forbidden"
	default:
		return 500, "internal_error"
	}
}
func writeError(w nethttp.ResponseWriter, r *nethttp.Request, status int, code string, err error, onError ErrorWriter) {
	if onError != nil {
		onError(w, r, status, err)
		return
	}
	body, _ := json.Marshal(struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
		Title  string `json:"title"`
		Code   string `json:"code"`
	}{"about:blank", status, nethttp.StatusText(status), code})
	w.Header().Set("Content-Type", "application/problem+json")
	if status == 401 {
		challenge := "Bearer"
		if code != "missing_token" {
			challenge += ` error="invalid_token"`
		}
		w.Header().Set("WWW-Authenticate", challenge)
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
func Middleware(cfg Config, deps Dependencies) (func(nethttp.Handler) nethttp.Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if nilPointer(deps.Verifier) {
		return nil, &jwt.ConfigError{Field: "Verifier", Reason: "is required"}
	}
	cfg.SkipPaths = append([]string(nil), cfg.SkipPaths...)
	cfg.RequiredGroups = append([]string(nil), cfg.RequiredGroups...)
	return func(next nethttp.Handler) nethttp.Handler {
		if nilPointer(next) {
			return nil
		}
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			if skip(r, cfg.SkipPaths) {
				next.ServeHTTP(w, r)
				return
			}
			token := bearer(r, cfg.Header)
			if token == "" {
				writeError(w, r, 401, "missing_token", fmt.Errorf("%w: missing token", jwt.ErrInvalidToken), deps.OnError)
				return
			}
			claims, err := deps.Verifier.Verify(r.Context(), token)
			if err != nil {
				status, code := errorStatus(err)
				writeError(w, r, status, code, err, deps.OnError)
				return
			}
			if len(cfg.RequiredGroups) != 0 {
				allowed := false
				for _, g := range cfg.RequiredGroups {
					if jwt.RequireGroup(claims, g) == nil {
						allowed = true
						break
					}
				}
				if !allowed {
					writeError(w, r, 403, "forbidden", jwt.ErrForbidden, deps.OnError)
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(ContextWithClaims(r.Context(), claims)))
		})
	}, nil
}

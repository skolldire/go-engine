package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	tokenjwt "github.com/golang-jwt/jwt/v5"
	"github.com/skolldire/go-engine/auth/jwt"
	jwthttp "github.com/skolldire/go-engine/auth/jwt/http"
	"github.com/skolldire/go-engine/health"
	"github.com/skolldire/go-engine/router"
	routererrors "github.com/skolldire/go-engine/router/errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"
)

var _ jwthttp.Verifier = (*jwt.Verifier)(nil)

type memoryJWKS struct {
	body        string
	unavailable atomic.Bool
	calls       atomic.Int64
}

func (m *memoryJWKS) RoundTrip(r *http.Request) (*http.Response, error) {
	m.calls.Add(1)
	status := 200
	body := m.body
	if m.unavailable.Load() {
		status = 503
		body = "private-JWKS-failure"
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func run() error {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	point, e := key.PublicKey.Bytes()
	if e != nil {
		return e
	}
	enc := base64.RawURLEncoding.EncodeToString
	body, e := json.Marshal(map[string]any{"keys": []any{map[string]any{"kid": "current", "kty": "EC", "crv": "P-256", "x": enc(point[1:33]), "y": enc(point[33:])}}})
	if e != nil {
		return e
	}
	transport := &memoryJWKS{body: string(body)}
	cfg := jwt.DefaultConfig()
	cfg.Issuer = "https://issuer.test"
	cfg.Audience = "application"
	cfg.JWKSURL = "https://issuer.test/keys"
	cfg.Algorithms = []string{"ES256"}
	v, e := jwt.NewVerifier(cfg, jwt.Dependencies{HTTPClient: &http.Client{Transport: transport}})
	if e != nil {
		return e
	}
	makeToken := func(kid string, groups []string) (string, error) {
		t := tokenjwt.NewWithClaims(tokenjwt.SigningMethodES256, tokenjwt.MapClaims{"iss": cfg.Issuer, "aud": cfg.Audience, "sub": "local-user", "groups": groups, "exp": time.Now().Add(time.Hour).Unix()})
		t.Header["kid"] = kid
		return t.SignedString(key)
	}
	valid, e := makeToken("current", []string{"staff"})
	if e != nil {
		return e
	}
	forbidden, e := makeToken("current", []string{"other"})
	if e != nil {
		return e
	}
	unknown, e := makeToken("new", []string{"staff"})
	if e != nil {
		return e
	}
	reg, e := health.New(health.DefaultConfig())
	if e != nil {
		return e
	}
	if e = reg.Register("jwks", health.CheckerFunc(v.Refresh)); e != nil {
		return e
	}
	if _, e = reg.Ready(context.Background()); e != nil {
		return e
	}
	hc := jwthttp.DefaultConfig()
	hc.SkipPaths = []string{"/public"}
	hc.RequiredGroups = []string{"staff"}
	var writeFailure error
	mw, e := jwthttp.Middleware(hc, jwthttp.Dependencies{Verifier: v, OnError: func(w http.ResponseWriter, r *http.Request, status int, original error) {
		code := "internal_error"
		switch status {
		case 401:
			code = "invalid_token"
			fields := strings.Fields(r.Header.Get("Authorization"))
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				code = "missing_token"
				w.Header().Set("WWW-Authenticate", "Bearer")
			} else {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			}
			if errors.Is(original, jwt.ErrExpiredToken) {
				code = "expired_token"
			}
		case 403:
			code = "forbidden"
		case 503:
			code = "auth_unavailable"
		}
		res, err := routererrors.NewResponder(routererrors.DefaultConfig(), routererrors.Dependencies{Classify: func(error) routererrors.Problem {
			return routererrors.Problem{Status: status, Code: code, Title: http.StatusText(status)}
		}})
		if err != nil {
			writeFailure = err
			return
		}
		writeFailure = res.Write(r.Context(), w, original)
	}})
	if e != nil {
		return e
	}
	mux, e := router.New(router.Config{AllowedHosts: []string{"example.test"}})
	if e != nil {
		return e
	}
	if e = mux.Use(router.Middleware(mw)); e != nil {
		return e
	}
	if e = mux.Handle("GET", "", "/public", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })); e != nil {
		return e
	}
	if e = mux.Handle("GET", "", "/private", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := jwthttp.ClaimsFromContext(r.Context())
		if !ok || c.Subject != "local-user" {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	})); e != nil {
		return e
	}
	if e = mux.Freeze(); e != nil {
		return e
	}
	check := func(path, raw string, status int) error {
		r := httptest.NewRequest("GET", "http://example.test"+path, nil)
		if raw != "" {
			r.Header.Set("Authorization", "Bearer "+raw)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if writeFailure != nil {
			return writeFailure
		}
		if w.Code != status || strings.Contains(w.Body.String(), "private-JWKS") || raw != "" && strings.Contains(w.Body.String(), raw) {
			return fmt.Errorf("unexpected or unsafe response")
		}
		return nil
	}
	for _, tt := range []struct {
		path, token string
		status      int
	}{{"/public", "", 204}, {"/publicity", "", 401}, {"/private", "", 401}, {"/private", valid, 204}, {"/private", forbidden, 403}} {
		if e = check(tt.path, tt.token, tt.status); e != nil {
			return e
		}
	}
	transport.unavailable.Store(true)
	if _, e = reg.Ready(context.Background()); !errors.Is(e, health.ErrNotReady) {
		return fmt.Errorf("JWKS failure did not affect readiness")
	}
	if e = check("/private", valid, 204); e != nil {
		return e
	}
	if e = check("/private", unknown, 503); e != nil {
		return e
	}
	before := transport.calls.Load()
	if e = check("/private", unknown, 503); e != nil {
		return e
	}
	if transport.calls.Load() != before {
		return fmt.Errorf("unknown-key failure window fetched twice")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("auth/jwt/http composed passed")
}

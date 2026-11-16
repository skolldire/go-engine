package main

import (
	"context"
	"fmt"
	"github.com/skolldire/go-engine/auth/jwt"
	jwthttp "github.com/skolldire/go-engine/auth/jwt/http"
	"net/http"
	"net/http/httptest"
)

type localVerifier struct{}

func (localVerifier) Verify(ctx context.Context, token string) (jwt.Claims, error) {
	if token != "local" {
		return jwt.Claims{}, jwt.ErrInvalidToken
	}
	return jwt.Claims{Subject: "user", Groups: []string{"staff"}}, nil
}
func run() error {
	cfg := jwthttp.DefaultConfig()
	cfg.RequiredGroups = []string{"staff"}
	cfg.SkipPaths = []string{"/public"}
	mw, e := jwthttp.Middleware(cfg, jwthttp.Dependencies{Verifier: localVerifier{}})
	if e != nil {
		return e
	}
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public" {
			c, ok := jwthttp.ClaimsFromContext(r.Context())
			if !ok || c.Subject != "user" {
				w.WriteHeader(500)
				return
			}
		}
		w.WriteHeader(204)
	}))
	for _, tt := range []struct {
		path, header string
		status       int
	}{{"/public", "", 204}, {"/publicity", "", 401}, {"/protected", "Bearer local", 204}, {"/protected", "Bearer bad", 401}} {
		r := httptest.NewRequest("GET", "http://local"+tt.path, nil)
		r.Header.Set("Authorization", tt.header)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tt.status {
			return fmt.Errorf("unexpected HTTP result")
		}
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("auth/jwt/http minimal passed")
}

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	tokenjwt "github.com/golang-jwt/jwt/v5"
	jwt "github.com/skolldire/go-engine/auth/jwt"
	"github.com/skolldire/go-engine/health"
	"io"
	"net/http"
	"strings"
	"time"
)

type memoryJWKS struct{ body string }

func (m memoryJWKS) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(m.body)), Request: r}, nil
}
func setup() (*jwt.Verifier, string, error) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, "", e
	}
	point, e := key.PublicKey.Bytes()
	if e != nil {
		return nil, "", e
	}
	enc := base64.RawURLEncoding.EncodeToString
	body, e := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "kid": "local", "crv": "P-256", "x": enc(point[1:33]), "y": enc(point[33:])}}})
	if e != nil {
		return nil, "", e
	}
	cfg := jwt.DefaultConfig()
	cfg.Issuer = "https://issuer.test"
	cfg.Audience = "application"
	cfg.JWKSURL = "https://issuer.test/keys"
	cfg.Algorithms = []string{"ES256"}
	if e = cfg.Validate(); e != nil {
		return nil, "", e
	}
	v, e := jwt.NewVerifier(cfg, jwt.Dependencies{HTTPClient: &http.Client{Transport: memoryJWKS{string(body)}}})
	if e != nil {
		return nil, "", e
	}
	token := tokenjwt.NewWithClaims(tokenjwt.SigningMethodES256, tokenjwt.MapClaims{"iss": cfg.Issuer, "aud": cfg.Audience, "sub": "local-user", "groups": []string{"staff"}, "exp": time.Now().Add(time.Hour).Unix()})
	token.Header["kid"] = "local"
	raw, e := token.SignedString(key)
	return v, raw, e
}
func run() error {
	v, raw, e := setup()
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
	claims, e := v.Verify(context.Background(), raw)
	if e != nil {
		return e
	}
	return jwt.RequireGroup(claims, "staff")
}
func main() {
	if e := run(); e != nil {
		panic(e)
	}
	fmt.Println("auth/jwt composed passed")
}

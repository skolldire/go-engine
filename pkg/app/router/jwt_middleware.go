package router

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTAuthConfig configures the JWT validation middleware.
type JWTAuthConfig struct {
	// JWKSURL is the URL of the JSON Web Key Set endpoint.
	// For Cognito: https://cognito-idp.{region}.amazonaws.com/{pool_id}/.well-known/jwks.json
	JWKSURL string

	// JWKSCache controls how long JWKS public keys are cached.
	// Defaults to 1 hour. Refreshes automatically 10 minutes before expiry.
	// Falls back to stale keys on network failure.
	JWKSCache time.Duration

	// Issuer is the expected "iss" claim value.
	// Leave empty to skip issuer validation.
	Issuer string

	// Audience is the expected "aud" or "client_id" claim value.
	// For Cognito access tokens the field is "client_id" — both are checked.
	// Leave empty to skip audience validation.
	Audience string

	// GroupsClaim is the JWT claim name that holds the user's groups.
	// Defaults to "cognito:groups".
	GroupsClaim string

	// SkipPaths lists request paths that bypass JWT validation entirely.
	// Matching is by prefix: "/health" also skips "/health/live", "/health/ready".
	SkipPaths []string
}

// JWTAuth returns a chi-compatible HTTP middleware that validates Bearer tokens
// on every non-skipped request.
//
// On success it injects *Claims into the request context; use ClaimsFromContext
// or MustClaimsFromContext to retrieve them in handlers.
// On failure it writes HTTP 401 with body {"error":"<code>"} and short-circuits
// the handler chain.
func JWTAuth(cfg JWTAuthConfig) func(http.Handler) http.Handler {
	if cfg.JWKSCache == 0 {
		cfg.JWKSCache = time.Hour
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "cognito:groups"
	}
	cache := &jwksCache{
		endpoint: cfg.JWKSURL,
		ttl:      cfg.JWKSCache,
		keys:     make(map[string]*rsa.PublicKey),
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldSkip(r.URL.Path, cfg.SkipPaths) {
				next.ServeHTTP(w, r)
				return
			}

			tokenStr := extractBearer(r)
			if tokenStr == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing_token")
				return
			}

			claims, err := parseAndValidate(r.Context(), tokenStr, cfg, cache)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, errorCode(err))
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// errorCode maps a validation error to the error code string defined in KAN-7.
func errorCode(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "expired") {
		return "token_expired"
	}
	return "invalid_token"
}

// ── internal ──────────────────────────────────────────────────────────────────

func shouldSkip(path string, skipPaths []string) bool {
	for _, p := range skipPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		return ""
	}
	return token
}

func parseAndValidate(ctx context.Context, tokenStr string, cfg JWTAuthConfig, cache *jwksCache) (*Claims, error) {
	parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v (expected RS256)", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("missing kid in token header")
		}
		return cache.getKey(ctx, kid)
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unexpected claims format")
	}

	if cfg.Issuer != "" {
		iss, _ := mapClaims["iss"].(string)
		if iss != cfg.Issuer {
			return nil, fmt.Errorf("issuer mismatch: expected %s", cfg.Issuer)
		}
	}

	if cfg.Audience != "" && !audienceMatches(mapClaims, cfg.Audience) {
		return nil, fmt.Errorf("audience mismatch")
	}

	return buildClaims(mapClaims, cfg.GroupsClaim), nil
}

func audienceMatches(claims jwt.MapClaims, expected string) bool {
	if aud, ok := claims["aud"].(string); ok && aud == expected {
		return true
	}
	if auds, ok := claims["aud"].([]interface{}); ok {
		for _, a := range auds {
			if s, ok := a.(string); ok && s == expected {
				return true
			}
		}
	}
	// Cognito access tokens use "client_id" instead of "aud"
	if cid, ok := claims["client_id"].(string); ok && cid == expected {
		return true
	}
	return false
}

func buildClaims(m jwt.MapClaims, groupsClaim string) *Claims {
	c := &Claims{Raw: make(map[string]interface{})}
	for k, v := range m {
		c.Raw[k] = v
	}
	if v, ok := m["sub"].(string); ok {
		c.Sub = v
	}
	if v, ok := m["email"].(string); ok {
		c.Email = v
	}
	if v, ok := m["cognito:username"].(string); ok {
		c.Username = v
	}
	if v, ok := m["token_use"].(string); ok {
		c.TokenUse = v
	}
	if raw, ok := m[groupsClaim].([]interface{}); ok {
		for _, g := range raw {
			if s, ok := g.(string); ok {
				c.Groups = append(c.Groups, s)
			}
		}
	}
	return c
}

// ── JWKS cache ────────────────────────────────────────────────────────────────

type jwksCache struct {
	mu        sync.RWMutex
	endpoint  string
	ttl       time.Duration
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

const jwksRefreshThreshold = 10 * time.Minute

func (c *jwksCache) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, exists := c.keys[kid]
	stale := time.Since(c.fetchedAt) >= (c.ttl - jwksRefreshThreshold)
	c.mu.RUnlock()

	if exists && !stale {
		return key, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// double-check: another goroutine may have refreshed already
	if key, ok := c.keys[kid]; ok && time.Since(c.fetchedAt) < (c.ttl-jwksRefreshThreshold) {
		return key, nil
	}

	newKeys, err := fetchJWKS(ctx, c.endpoint)
	if err != nil {
		if key != nil {
			return key, nil // stale fallback on network failure
		}
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	c.keys = newKeys
	c.fetchedAt = time.Now()

	k, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("key with kid %q not found in JWKS endpoint %s", kid, c.endpoint)
	}
	return k, nil
}

// ── JWKS fetching ─────────────────────────────────────────────────────────────

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func fetchJWKS(ctx context.Context, endpoint string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode JWKS response: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := rsaKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	return keys, nil
}

func rsaKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeAuthError(w http.ResponseWriter, status int, errCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, errCode)
}

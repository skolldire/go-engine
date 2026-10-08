package router

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/skolldire/go-engine/pkg/utilities/error_handler"
	"golang.org/x/sync/singleflight"
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

	// Issuer is the expected "iss" claim value. It is required by default: if it
	// is empty and AllowEmptyIssuer is false, the middleware fails closed and
	// rejects every request. Set AllowEmptyIssuer to intentionally skip issuer
	// validation.
	Issuer string

	// Audience is the expected "aud" or "client_id" claim value.
	// For Cognito access tokens the field is "client_id" — both are checked.
	// It is required by default: if empty and AllowEmptyAudience is false, the
	// middleware fails closed. Set AllowEmptyAudience to skip audience validation.
	Audience string

	// AllowEmptyIssuer explicitly disables issuer validation when Issuer is empty.
	AllowEmptyIssuer bool

	// AllowEmptyAudience explicitly disables audience validation when Audience is empty.
	AllowEmptyAudience bool

	// GroupsClaim is the JWT claim name that holds the user's groups.
	// Defaults to "cognito:groups".
	GroupsClaim string

	// SkipPaths lists request paths that bypass JWT validation entirely.
	// Matching is by prefix: "/health" also skips "/health/live", "/health/ready".
	SkipPaths []string

	// HTTPClient fetches the JWKS. Defaults to a client with a 10s timeout.
	// Inject a custom client to control transport, TLS or timeouts (and in tests).
	HTTPClient *http.Client
}

// JWTAuth returns a chi-compatible HTTP middleware that validates Bearer tokens
// on every non-skipped request.
//
// On success it injects *Claims into the request context; use ClaimsFromContext
// or MustClaimsFromContext to retrieve them in handlers.
// On failure it writes HTTP 401 with an error_handler.CommonApiError body
// (code "ER-401", and "details.reason" set to a stable value: "missing_token",
// "invalid_token" or "expired_token") and short-circuits the handler chain.
func JWTAuth(cfg JWTAuthConfig) func(http.Handler) http.Handler {
	if cfg.JWKSCache == 0 {
		cfg.JWKSCache = time.Hour
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "cognito:groups"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	// Fail closed on insecure configuration: issuer and audience are required
	// unless explicitly opted out. A middleware that always rejects is safer
	// than one that silently accepts tokens for any issuer/audience.
	misconfig := configError(cfg)

	cache := &jwksCache{
		endpoint:   cfg.JWKSURL,
		ttl:        cfg.JWKSCache,
		keys:       make(map[string]*rsa.PublicKey),
		httpClient: cfg.HTTPClient,
		now:        time.Now,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldSkip(r.URL.Path, cfg.SkipPaths) {
				next.ServeHTTP(w, r)
				return
			}

			if misconfig != "" {
				writeAuthError(w, http.StatusInternalServerError, misconfig)
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

// configError returns a non-empty reason when the middleware is configured
// insecurely (missing issuer/audience without an explicit opt-out), so it can
// fail closed. An empty string means the configuration is acceptable.
func configError(cfg JWTAuthConfig) string {
	if cfg.JWKSURL == "" {
		return "auth_misconfigured_jwks_url"
	}
	if cfg.Issuer == "" && !cfg.AllowEmptyIssuer {
		return "auth_misconfigured_issuer"
	}
	if cfg.Audience == "" && !cfg.AllowEmptyAudience {
		return "auth_misconfigured_audience"
	}
	return ""
}

// errorCode maps a validation error to a stable reason string surfaced in
// the CommonApiError "details.reason" field. It matches on the library's
// sentinel errors rather than on message text, which COMPATIBILITY.md requires
// and which survives upstream wording changes.
func errorCode(err error) string {
	if errors.Is(err, jwt.ErrTokenExpired) {
		return "expired_token"
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
	parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != "RS256" {
			return nil, fmt.Errorf("unexpected signing method: %v (expected RS256)", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("missing kid in token header")
		}
		return cache.getKey(ctx, kid)
	}, jwt.WithValidMethods([]string{"RS256"}))
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

	// Issuer/audience are validated whenever configured. Callers that leave them
	// empty must opt out explicitly (enforced at construction via configError),
	// so an empty expected value here means validation was intentionally skipped.
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
	if auds, ok := claims["aud"].([]any); ok {
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
	c := &Claims{Raw: make(map[string]any)}
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
	if raw, ok := m[groupsClaim].([]any); ok {
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
	mu         sync.RWMutex
	endpoint   string
	ttl        time.Duration
	keys       map[string]*rsa.PublicKey
	fetchedAt  time.Time
	httpClient *http.Client
	now        func() time.Time
	// refresh collapses concurrent refreshes into a single JWKS fetch. Without
	// it the write lock was held for the whole HTTP round trip, so every
	// in-flight request serialised behind a key rotation.
	refresh singleflight.Group
}

const jwksRefreshThreshold = 10 * time.Minute

// maxStaleWindow bounds how long stale keys may be served after a failed
// refresh: up to this long since the last successful fetch. It is an absolute
// cap (independent of the TTL) so a short TTL still gets useful resilience
// during an outage, while arbitrarily old keys are never trusted.
const maxStaleWindow = 6 * time.Hour

// refreshThreshold is how long before expiry a proactive refresh is attempted.
// It never exceeds half the TTL, so a small TTL does not make the cache
// consider itself perpetually stale (which would refetch on every request).
func (c *jwksCache) refreshThreshold() time.Duration {
	if jwksRefreshThreshold > c.ttl/2 {
		return c.ttl / 2
	}
	return jwksRefreshThreshold
}

func (c *jwksCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *jwksCache) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	threshold := c.ttl - c.refreshThreshold()

	c.mu.RLock()
	key, exists := c.keys[kid]
	age := c.clock().Sub(c.fetchedAt)
	c.mu.RUnlock()

	if exists && age < threshold {
		return key, nil
	}

	// Exactly one goroutine performs the fetch; the others wait for its result.
	// The mutex is never held across the network call.
	_, err, _ := c.refresh.Do("jwks", func() (any, error) {
		// A concurrent refresh may have completed while this call queued.
		c.mu.RLock()
		fresh := c.clock().Sub(c.fetchedAt) < threshold
		c.mu.RUnlock()
		if fresh {
			return nil, nil
		}

		newKeys, fetchErr := fetchJWKS(ctx, c.httpClient, c.endpoint)
		if fetchErr != nil {
			return nil, fetchErr
		}

		c.mu.Lock()
		c.keys = newKeys
		c.fetchedAt = c.clock()
		c.mu.Unlock()

		return nil, nil
	})

	if err != nil {
		// Stale fallback on network failure, bounded to maxStaleWindow so
		// arbitrarily old keys are never trusted.
		c.mu.RLock()
		staleAge := c.clock().Sub(c.fetchedAt)
		c.mu.RUnlock()

		if key != nil && staleAge < maxStaleWindow {
			return key, nil
		}
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}

	c.mu.RLock()
	k, ok := c.keys[kid]
	c.mu.RUnlock()

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
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// maxJWKSBody caps the JWKS response size to defend against a hostile or
// misbehaving endpoint returning an unbounded body.
const maxJWKSBody = 1 << 20 // 1 MiB

func fetchJWKS(ctx context.Context, client *http.Client, endpoint string) (map[string]*rsa.PublicKey, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJWKSBody)).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode JWKS response: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		// Only accept signing keys and, when advertised, the RS256 algorithm.
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if k.Alg != "" && k.Alg != "RS256" {
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

// writeAuthError writes an error_handler.CommonApiError body so that auth
// failures share the same error taxonomy as the rest of the API
// (same shape produced by error_handler.HandleApiErrorResponse).
// reason is a stable machine-readable value exposed under "details.reason"
// (e.g. "missing_token", "invalid_token", "expired_token", "forbidden").
func writeAuthError(w http.ResponseWriter, status int, reason string) {
	var apiErr *error_handler.CommonApiError
	switch status {
	case http.StatusForbidden:
		apiErr = error_handler.NewForbiddenError(authErrorMsg(reason), nil)
	case http.StatusInternalServerError:
		apiErr = error_handler.NewInternalError(authErrorMsg(reason), nil)
	default:
		apiErr = error_handler.NewUnauthorizedError(authErrorMsg(reason), nil)
	}
	apiErr = apiErr.WithDetail("reason", reason)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.HttpCode)
	b, _ := json.Marshal(apiErr)
	_, _ = w.Write(b)
}

// authErrorMsg returns a human-readable message for a given auth failure reason.
func authErrorMsg(reason string) string {
	switch reason {
	case "missing_token":
		return "authentication token is missing"
	case "invalid_token":
		return "authentication token is invalid"
	case "expired_token":
		return "authentication token has expired"
	case "forbidden":
		return "access forbidden: insufficient permissions"
	case "auth_misconfigured_jwks_url", "auth_misconfigured_issuer", "auth_misconfigured_audience":
		return "authentication is misconfigured on the server"
	default:
		return "authentication failed"
	}
}

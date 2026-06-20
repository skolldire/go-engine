package router

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testKID      = "test-key-1"
	testIssuer   = "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test"
	testAudience = "test-client-id"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func jwksServer(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	pub := &key.PublicKey
	nBytes := pub.N.Bytes()
	eBytes := big.NewInt(int64(pub.E)).Bytes()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"keys": []map[string]interface{}{{
				"kid": testKID,
				"kty": "RSA",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(nBytes),
				"e":   base64.RawURLEncoding.EncodeToString(eBytes),
			}},
		})
	}))
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":              "user-123",
		"email":            "john@example.com",
		"cognito:username": "john",
		"cognito:groups":   []interface{}{"students", "admins"},
		"iss":              testIssuer,
		"aud":              testAudience,
		"exp":              time.Now().Add(time.Hour).Unix(),
		"iat":              time.Now().Unix(),
		"token_use":        "id",
	}
}

func echoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims != nil {
			json.NewEncoder(w).Encode(claims) //nolint:errcheck
		}
		w.WriteHeader(http.StatusOK)
	}
}

// ── JWTMiddleware tests ───────────────────────────────────────────────────────

func TestJWTMiddleware_ValidToken(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Issuer: testIssuer, Audience: testAudience}
	mw := JWTAuth(cfg)(echoHandler())

	token := signToken(t, key, validClaims())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "user-123")
	assert.Contains(t, rec.Body.String(), "john@example.com")
}

func TestJWTMiddleware_ClaimsInContext(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	var captured *Claims
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	cfg := JWTAuthConfig{JWKSURL: srv.URL, Issuer: testIssuer, Audience: testAudience}
	mw := JWTAuth(cfg)(handler)

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, validClaims()))
	mw.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, captured)
	assert.Equal(t, "user-123", captured.Sub)
	assert.Equal(t, "john@example.com", captured.Email)
	assert.Equal(t, "john", captured.Username)
	assert.Equal(t, "id", captured.TokenUse)
	assert.Contains(t, captured.Groups, "students")
	assert.Contains(t, captured.Groups, "admins")
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	cfg := JWTAuthConfig{JWKSURL: srv.URL}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertAuthErrorBody(t, rec.Body.Bytes(), "ER-401", "missing_token")
}

// assertAuthErrorBody verifies the response body matches the CommonApiError
// shape used across the API: {"code","msg","details":{"reason":...}}.
func assertAuthErrorBody(t *testing.T, body []byte, wantCode, wantReason string) {
	t.Helper()
	var parsed struct {
		Code    string            `json:"code"`
		Msg     string            `json:"msg"`
		Details map[string]string `json:"details"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Equal(t, wantCode, parsed.Code)
	assert.NotEmpty(t, parsed.Msg)
	assert.Equal(t, wantReason, parsed.Details["reason"])
}

func TestJWTMiddleware_MalformedHeader(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	cfg := JWTAuthConfig{JWKSURL: srv.URL}
	mw := JWTAuth(cfg)(echoHandler())

	for _, bad := range []string{"Basic abc", "Bearer", "token-without-prefix"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Authorization", bad)
		mw.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "header: %q", bad)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	claims := validClaims()
	claims["exp"] = time.Now().Add(-time.Hour).Unix() // already expired

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Issuer: testIssuer, Audience: testAudience}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, claims))
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertAuthErrorBody(t, rec.Body.Bytes(), "ER-401", "expired_token")
}

func TestJWTMiddleware_WrongSigningKey(t *testing.T) {
	key := generateTestKey(t)
	wrongKey := generateTestKey(t) // different key pair
	srv := jwksServer(t, key)      // server serves key, but token signed with wrongKey
	defer srv.Close()

	claims := validClaims()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	tok.Header["kid"] = testKID
	token, _ := tok.SignedString(wrongKey)

	cfg := JWTAuthConfig{JWKSURL: srv.URL}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_IssuerMismatch(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	claims := validClaims()
	claims["iss"] = "https://wrong-issuer.example.com"

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Issuer: testIssuer}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, claims))
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertAuthErrorBody(t, rec.Body.Bytes(), "ER-401", "invalid_token")
}

func TestJWTMiddleware_AudienceMismatch(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	claims := validClaims()
	claims["aud"] = "wrong-client-id"

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Audience: testAudience}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, claims))
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_AudienceViaClientID(t *testing.T) {
	// Cognito access tokens use "client_id" instead of "aud"
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	claims := validClaims()
	delete(claims, "aud")
	claims["client_id"] = testAudience

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Audience: testAudience}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, claims))
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJWTMiddleware_SkipPaths(t *testing.T) {
	cfg := JWTAuthConfig{
		JWKSURL:   "http://unreachable-jwks.invalid",
		SkipPaths: []string{"/health", "/ping"},
	}
	mw := JWTAuth(cfg)(echoHandler())

	for _, path := range []string{"/health", "/health/live", "/health/ready", "/ping"} {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, rec.Code, "path %s should be skipped", path)
	}
}

func TestJWTMiddleware_SkipPath_NonSkippedRequiresAuth(t *testing.T) {
	cfg := JWTAuthConfig{
		JWKSURL:   "http://unreachable-jwks.invalid",
		SkipPaths: []string{"/health"},
	}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_NoIssuerValidation(t *testing.T) {
	// Issuer left empty → skip issuer check
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	claims := validClaims()
	claims["iss"] = "https://anything.example.com"

	cfg := JWTAuthConfig{JWKSURL: srv.URL} // no Issuer set
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, claims))
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── ClaimsFromContext tests ───────────────────────────────────────────────────

func TestClaimsFromContext_NilWhenAbsent(t *testing.T) {
	assert.Nil(t, ClaimsFromContext(context.Background()))
}

func TestClaimsFromContext_ReturnsInjected(t *testing.T) {
	c := &Claims{Sub: "abc", Email: "x@y.com"}
	ctx := context.WithValue(context.Background(), claimsKey{}, c)
	got := ClaimsFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "abc", got.Sub)
}

// ── RequireGroup tests ────────────────────────────────────────────────────────

func TestRequireGroup_AllowsMatchingGroup(t *testing.T) {
	c := &Claims{Groups: []string{"students", "admins"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil).WithContext(ctx)
	RequireGroup("admins")(echoHandler()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireGroup_BlocksNonMatchingGroup(t *testing.T) {
	c := &Claims{Groups: []string{"students"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil).WithContext(ctx)
	RequireGroup("admins")(echoHandler()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assertAuthErrorBody(t, rec.Body.Bytes(), "ER-403", "forbidden")
}

func TestRequireGroup_NoClaims_Returns401(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	RequireGroup("admins")(echoHandler()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireGroup_MultipleGroupsAllowed(t *testing.T) {
	c := &Claims{Groups: []string{"teachers"}}
	ctx := context.WithValue(context.Background(), claimsKey{}, c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/content", nil).WithContext(ctx)
	RequireGroup("admins", "teachers", "staff")(echoHandler()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── MustClaimsFromContext tests ───────────────────────────────────────────────

func TestMustClaimsFromContext_PanicsWhenAbsent(t *testing.T) {
	assert.Panics(t, func() {
		MustClaimsFromContext(context.Background())
	})
}

func TestMustClaimsFromContext_ReturnsClaimsWhenPresent(t *testing.T) {
	c := &Claims{Sub: "u1"}
	ctx := InjectClaimsForTest(context.Background(), c)
	assert.Equal(t, "u1", MustClaimsFromContext(ctx).Sub)
}

// ── Concurrent requests (race detector) ──────────────────────────────────────

func TestJWTMiddleware_ConcurrentRequests(t *testing.T) {
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Issuer: testIssuer, Audience: testAudience}
	mw := JWTAuth(cfg)(echoHandler())

	token := signToken(t, key, validClaims())

	const n = 100
	results := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/users", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			mw.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}
	for i := 0; i < n; i++ {
		assert.Equal(t, http.StatusOK, <-results)
	}
}

// ── shouldSkip tests ──────────────────────────────────────────────────────────

func TestShouldSkip(t *testing.T) {
	skip := []string{"/health", "/ping"}

	assert.True(t, shouldSkip("/health", skip))
	assert.True(t, shouldSkip("/health/live", skip))
	assert.True(t, shouldSkip("/health/ready", skip))
	assert.True(t, shouldSkip("/ping", skip))
	assert.False(t, shouldSkip("/users", skip))
	assert.False(t, shouldSkip("/healthcheck", skip)) // prefix match requires /
}

// ── rsaKeyFromJWK tests ───────────────────────────────────────────────────────

func TestRSAKeyFromJWK_RoundTrip(t *testing.T) {
	key := generateTestKey(t)
	pub := &key.PublicKey

	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	recovered, err := rsaKeyFromJWK(nB64, eB64)
	require.NoError(t, err)
	assert.Equal(t, pub.N, recovered.N)
	assert.Equal(t, pub.E, recovered.E)
}

func TestRSAKeyFromJWK_InvalidBase64(t *testing.T) {
	_, err := rsaKeyFromJWK("!!!invalid", "AQAB")
	assert.Error(t, err)
}

func TestRSAKeyFromJWK_InvalidExponent(t *testing.T) {
	key := generateTestKey(t)
	nB64 := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	_, err := rsaKeyFromJWK(nB64, "!!!invalid")
	assert.Error(t, err)
}

// ── audienceMatches — slice audience ─────────────────────────────────────────

func TestJWTMiddleware_AudienceAsSlice(t *testing.T) {
	// JWT spec allows "aud" to be a string or array of strings
	key := generateTestKey(t)
	srv := jwksServer(t, key)
	defer srv.Close()

	claims := validClaims()
	claims["aud"] = []interface{}{testAudience, "other-client"}

	cfg := JWTAuthConfig{JWKSURL: srv.URL, Audience: testAudience}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, key, claims))
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── fetchJWKS error paths ─────────────────────────────────────────────────────

func TestJWTMiddleware_JWKSServerUnreachable(t *testing.T) {
	cfg := JWTAuthConfig{JWKSURL: "http://127.0.0.1:1"}
	mw := JWTAuth(cfg)(echoHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6InRlc3Qta2V5LTEifQ.e30.signature")
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_JWKSReturnsInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not-json")) //nolint:errcheck
	}))
	defer srv.Close()

	key := generateTestKey(t)
	cfg := JWTAuthConfig{JWKSURL: srv.URL}
	mw := JWTAuth(cfg)(echoHandler())

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(validClaims()))
	tok.Header["kid"] = testKID
	token, _ := tok.SignedString(key)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── stale cache fallback ──────────────────────────────────────────────────────

func TestJWTMiddleware_StaleKeyUsedWhenJWKSDown(t *testing.T) {
	key := generateTestKey(t)
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount > 1 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		pub := &key.PublicKey
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"keys": []map[string]interface{}{{
				"kid": testKID, "kty": "RSA", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	}))
	defer srv.Close()

	cfg := JWTAuthConfig{
		JWKSURL:   srv.URL,
		JWKSCache: 1 * time.Millisecond, // expire immediately
	}
	mw := JWTAuth(cfg)(echoHandler())

	doRequest := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Authorization", "Bearer "+signToken(t, key, validClaims()))
		mw.ServeHTTP(rec, req)
		return rec.Code
	}

	// First request: JWKS is fetched and key is cached.
	assert.Equal(t, http.StatusOK, doRequest())
	// Second request: cache expired, JWKS fetch fails, but stale key is used.
	assert.Equal(t, http.StatusOK, doRequest())
}

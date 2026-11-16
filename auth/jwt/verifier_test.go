package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	tokenjwt "github.com/golang-jwt/jwt/v5"
)

type endpoint struct {
	mu      sync.Mutex
	body    string
	status  int
	failure error
	calls   atomic.Int64
	gate    <-chan struct{}
	entered chan struct{}
	once    sync.Once
	seen    context.Context
}

func (e *endpoint) RoundTrip(r *http.Request) (*http.Response, error) {
	e.calls.Add(1)
	if e.entered != nil {
		e.once.Do(func() { e.seen = r.Context(); close(e.entered) })
	}
	if e.gate != nil {
		select {
		case <-e.gate:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failure != nil {
		return nil, e.failure
	}
	return &http.Response{StatusCode: e.status, Body: io.NopCloser(strings.NewReader(e.body)), Header: make(http.Header), Request: r}, nil
}
func (e *endpoint) set(body string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.body = body
	e.failure = err
}
func rsaKey(t testing.TB) *rsa.PrivateKey {
	t.Helper()
	k, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func ecKey(t testing.TB, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	k, e := ecdsa.GenerateKey(curve, rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func keyJSON(kid string, key any) map[string]any {
	b64 := base64.RawURLEncoding.EncodeToString
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return map[string]any{"kty": "RSA", "kid": kid, "n": b64(k.N.Bytes()), "e": b64(big.NewInt(int64(k.E)).Bytes())}
	case *ecdsa.PrivateKey:
		curve := "P-256"
		size := 32
		if k.Curve == elliptic.P384() {
			curve = "P-384"
			size = 48
		}
		return map[string]any{"kty": "EC", "kid": kid, "crv": curve, "x": b64(k.X.FillBytes(make([]byte, size))), "y": b64(k.Y.FillBytes(make([]byte, size)))}
	}
	panic("fixture key type")
}
func keySet(keys ...map[string]any) string {
	body, e := json.Marshal(map[string]any{"keys": keys})
	if e != nil {
		panic(e)
	}
	return string(body)
}
func config() Config {
	c := DefaultConfig()
	c.Issuer = "https://issuer.test"
	c.Audience = "app"
	c.JWKSURL = "https://issuer.test/keys"
	return c
}
func verifier(t testing.TB, c Config, e *endpoint) *Verifier {
	t.Helper()
	v, err := NewVerifier(c, Dependencies{HTTPClient: &http.Client{Transport: e}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func claims() tokenjwt.MapClaims {
	return tokenjwt.MapClaims{"iss": "https://issuer.test", "sub": "subject", "aud": "app", "jti": "id", "exp": time.Now().Add(24 * time.Hour).Unix(), "iat": time.Now().Unix(), "groups": []string{"staff"}, "nested": map[string]any{"list": []any{"original"}}}
}
func sign(t testing.TB, method tokenjwt.SigningMethod, kid string, key any, c tokenjwt.MapClaims) string {
	t.Helper()
	token := tokenjwt.NewWithClaims(method, c)
	if kid != "" {
		token.Header["kid"] = kid
	}
	raw, e := token.SignedString(key)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestConfig(t *testing.T) {
	base := config()
	for _, tt := range []struct {
		field  string
		change func(*Config)
	}{
		{"Issuer", func(c *Config) { c.Issuer = "" }}, {"Audience", func(c *Config) { c.Audience = "" }}, {"AudienceClaim", func(c *Config) { c.AudienceClaim = "" }}, {"GroupsClaim", func(c *Config) { c.GroupsClaim = "" }}, {"JWKSURL", func(c *Config) { c.JWKSURL = "http://remote.test/keys" }}, {"Algorithms", func(c *Config) { c.Algorithms = nil }}, {"Algorithms", func(c *Config) { c.Algorithms = []string{"HS256"} }}, {"Algorithms", func(c *Config) { c.Algorithms = []string{"none"} }}, {"CacheTTL", func(c *Config) { c.CacheTTL = 0 }}, {"RefreshTimeout", func(c *Config) { c.RefreshTimeout = 0 }}, {"ClockSkew", func(c *Config) { c.ClockSkew = -1 }}, {"StaleGrace", func(c *Config) { c.StaleGrace = -1 }}, {"MaxTokenBytes", func(c *Config) { c.MaxTokenBytes = 0 }}, {"MaxJWKSBytes", func(c *Config) { c.MaxJWKSBytes = 0 }},
	} {
		c := base
		tt.change(&c)
		var ce *ConfigError
		e := c.Validate()
		if !errors.Is(e, ErrInvalidConfig) || !errors.As(e, &ce) || ce.Field != tt.field {
			t.Fatalf("%s: %v", tt.field, e)
		}
	}
	for _, u := range []string{"http://127.0.0.1/keys", "http://[::1]/keys", "http://localhost/keys", "https://remote.test/keys"} {
		c := base
		c.JWKSURL = u
		if e := c.Validate(); e != nil {
			t.Fatal(u, e)
		}
	}
	c := base
	c.ClockSkew = 0
	c.StaleGrace = 0
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	if _, e := NewVerifier(base, Dependencies{}); !errors.Is(e, ErrInvalidConfig) {
		t.Fatal(e)
	}
	first, second := DefaultConfig(), DefaultConfig()
	first.Algorithms[0] = "bad"
	if second.Algorithms[0] != "RS256" {
		t.Fatal("shared defaults")
	}
	if _, e := NewVerifier(Config{}, Dependencies{}); e == nil {
		t.Fatal("literal completed")
	}
	e := &endpoint{status: 200}
	v := verifier(t, base, e)
	if e.calls.Load() != 0 {
		t.Fatal("New fetched")
	}
	base.Algorithms[0] = "none"
	if v.cfg.Algorithms[0] != "RS256" {
		t.Fatal("config not copied")
	}
}
func TestAlgorithmsAndClaims(t *testing.T) {
	rsa := rsaKey(t)
	p256 := ecKey(t, elliptic.P256())
	p384 := ecKey(t, elliptic.P384())
	e := &endpoint{status: 200, body: keySet(keyJSON("rsa", rsa), keyJSON("p256", p256), keyJSON("p384", p384))}
	c := config()
	c.Algorithms = []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384"}
	v := verifier(t, c, e)
	for _, tt := range []struct {
		method tokenjwt.SigningMethod
		kid    string
		key    any
	}{{tokenjwt.SigningMethodRS256, "rsa", rsa}, {tokenjwt.SigningMethodRS384, "rsa", rsa}, {tokenjwt.SigningMethodRS512, "rsa", rsa}, {tokenjwt.SigningMethodPS256, "rsa", rsa}, {tokenjwt.SigningMethodPS384, "rsa", rsa}, {tokenjwt.SigningMethodPS512, "rsa", rsa}, {tokenjwt.SigningMethodES256, "p256", p256}, {tokenjwt.SigningMethodES384, "p384", p384}} {
		raw := sign(t, tt.method, tt.kid, tt.key, claims())
		got, err := v.Verify(context.Background(), raw)
		if err != nil || got.Subject != "subject" || got.ID != "id" || got.Issuer != c.Issuer || got.Groups[0] != "staff" {
			t.Fatal(tt.method.Alg(), got, err)
		}
		got.Custom["nested"].(map[string]any)["list"].([]any)[0] = "changed"
		again, err := v.Verify(context.Background(), raw)
		if err != nil || again.Custom["nested"].(map[string]any)["list"].([]any)[0] != "original" {
			t.Fatal("shared claims")
		}
	}
	if e.calls.Load() != 1 {
		t.Fatal(e.calls.Load())
	}
}
func TestTokenRejections(t *testing.T) {
	key := rsaKey(t)
	ep := &endpoint{status: 200, body: keySet(keyJSON("known", key))}
	v := verifier(t, config(), ep)
	tests := []struct {
		name   string
		change func(tokenjwt.MapClaims)
		kid    string
		want   error
	}{
		{"expired", func(c tokenjwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, "known", ErrExpiredToken},
		{"missing exp", func(c tokenjwt.MapClaims) { delete(c, "exp") }, "known", ErrInvalidToken},
		{"future nbf", func(c tokenjwt.MapClaims) { c["nbf"] = time.Now().Add(time.Minute).Unix() }, "known", ErrInvalidToken},
		{"future iat", func(c tokenjwt.MapClaims) { c["iat"] = time.Now().Add(time.Minute).Unix() }, "known", ErrInvalidToken},
		{"issuer", func(c tokenjwt.MapClaims) { c["iss"] = "private-issuer" }, "known", ErrInvalidToken},
		{"audience", func(c tokenjwt.MapClaims) { c["aud"] = "private-audience" }, "known", ErrInvalidToken},
		{"groups string", func(c tokenjwt.MapClaims) { c["groups"] = "private-groups" }, "known", ErrInvalidToken},
		{"groups mixed", func(c tokenjwt.MapClaims) { c["groups"] = []any{"staff", 1} }, "known", ErrInvalidToken},
		{"kid", func(tokenjwt.MapClaims) {}, "", ErrInvalidToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := claims()
			tt.change(c)
			raw := sign(t, tokenjwt.SigningMethodRS256, tt.kid, key, c)
			_, e := v.Verify(context.Background(), raw)
			if !errors.Is(e, tt.want) || !errors.Is(e, ErrInvalidToken) {
				t.Fatal(e)
			}
			if strings.Contains(e.Error(), raw) || strings.Contains(e.Error(), "private-") {
				t.Fatal("leaked input")
			}
		})
	}
	before := ep.calls.Load()
	raw := sign(t, tokenjwt.SigningMethodHS256, "known", []byte("secret"), claims())
	if _, e := v.Verify(context.Background(), raw); !errors.Is(e, ErrInvalidToken) || ep.calls.Load() != before {
		t.Fatal("algorithm looked up a key", e)
	}
	if _, e := v.Verify(context.Background(), strings.Repeat("x", v.cfg.MaxTokenBytes+1)); !errors.Is(e, ErrInvalidToken) {
		t.Fatal(e)
	}
	if _, e := v.Verify(nil, "x"); !errors.Is(e, ErrNilContext) { //nolint:staticcheck // Exercise invalid context.
		t.Fatal(e)
	} //nolint:staticcheck // Invalid-context contract.
	if e := v.Refresh(nil); !errors.Is(e, ErrNilContext) { //nolint:staticcheck // Exercise invalid context.
		t.Fatal(e)
	} //nolint:staticcheck // Invalid-context contract.
	if e := RequireGroup(Claims{}, ""); !errors.Is(e, ErrInvalidCall) {
		t.Fatal(e)
	}
	if e := RequireGroup(Claims{Groups: []string{"staff"}}, "staff"); e != nil {
		t.Fatal(e)
	}
	if e := RequireGroup(Claims{}, "staff"); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
}
func TestCustomAudienceAndGroups(t *testing.T) {
	key := rsaKey(t)
	c := config()
	c.AudienceClaim = "client_id"
	c.GroupsClaim = "cognito:groups"
	v := verifier(t, c, &endpoint{status: 200, body: keySet(keyJSON("k", key))})
	m := claims()
	delete(m, "aud")
	delete(m, "groups")
	m["client_id"] = "app"
	m["cognito:groups"] = []string{"staff"}
	m["token_use"] = "access"
	raw := sign(t, tokenjwt.SigningMethodRS256, "k", key, m)
	got, e := v.Verify(context.Background(), raw)
	if e != nil || got.Groups[0] != "staff" || got.Custom["token_use"] != "access" {
		t.Fatal(got, e)
	}
	c = config()
	v = verifier(t, c, &endpoint{status: 200, body: keySet(keyJSON("k", key))})
	m = claims()
	m["aud"] = []string{"other", "app"}
	delete(m, "groups")
	if got, e := v.Verify(context.Background(), sign(t, tokenjwt.SigningMethodRS256, "k", key, m)); e != nil || len(got.Groups) != 0 {
		t.Fatal(got, e)
	}
}
func TestJWKConstraints(t *testing.T) {
	key := rsaKey(t)
	ec := ecKey(t, elliptic.P384())
	for _, change := range []func(map[string]any){func(j map[string]any) { j["alg"] = "RS384" }, func(j map[string]any) { j["use"] = "enc" }, func(j map[string]any) { j["e"] = "AA" }} {
		j := keyJSON("bad", key)
		change(j)
		v := verifier(t, config(), &endpoint{status: 200, body: keySet(j, keyJSON("other", key))})
		if _, e := v.Verify(context.Background(), sign(t, tokenjwt.SigningMethodRS256, "bad", key, claims())); !errors.Is(e, ErrInvalidToken) {
			t.Fatal(e)
		}
	}
	v := verifier(t, config(), &endpoint{status: 200, body: keySet(keyJSON("ec", ec))})
	if _, e := v.Verify(context.Background(), sign(t, tokenjwt.SigningMethodRS256, "ec", key, claims())); !errors.Is(e, ErrInvalidToken) {
		t.Fatal(e)
	}
	if _, e := parseJWKS([]byte(keySet(keyJSON("same", key), keyJSON("same", key)))); e == nil {
		t.Fatal("ambiguous kid")
	}
}
func TestRefreshFailures(t *testing.T) {
	key := rsaKey(t)
	for _, tt := range []struct {
		body          string
		status, limit int
	}{{"{}", 200, 1024}, {"bad", 200, 1024}, {keySet(keyJSON("k", key)), 503, 4096}, {keySet(keyJSON("k", key)), 200, 10}, {`{"keys":[{"kid":"x","kty":"oct"}]}`, 200, 4096}} {
		c := config()
		c.MaxJWKSBytes = tt.limit
		v := verifier(t, c, &endpoint{body: tt.body, status: tt.status})
		if e := v.Refresh(context.Background()); !errors.Is(e, ErrKeySetUnavailable) {
			t.Fatal(e)
		}
	}
}
func TestUnknownKidWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := rsaKey(t)
		ep := &endpoint{status: 200, body: keySet(keyJSON("known", key))}
		v := verifier(t, config(), ep)
		if e := v.Refresh(context.Background()); e != nil {
			t.Fatal(e)
		}
		unknown := sign(t, tokenjwt.SigningMethodRS256, "unknown", key, claims())
		known := sign(t, tokenjwt.SigningMethodRS256, "known", key, claims())
		cause := errors.New("network down")
		ep.set("", cause)
		for range 2 {
			if _, e := v.Verify(context.Background(), unknown); !errors.Is(e, ErrKeySetUnavailable) || !errors.Is(e, cause) {
				t.Fatal(e)
			}
		}
		if ep.calls.Load() != 2 {
			t.Fatal(ep.calls.Load())
		}
		if _, e := v.Verify(context.Background(), known); e != nil {
			t.Fatal("known fresh key failed", e)
		}
		time.Sleep(30 * time.Second)
		ep.set(keySet(keyJSON("known", key)), nil)
		for range 2 {
			if _, e := v.Verify(context.Background(), unknown); !errors.Is(e, ErrUnknownKey) {
				t.Fatal(e)
			}
		}
		if ep.calls.Load() != 3 {
			t.Fatal(ep.calls.Load())
		}
		ep.set(keySet(keyJSON("known", key), keyJSON("unknown", key)), nil)
		if e := v.Refresh(context.Background()); e != nil {
			t.Fatal(e)
		}
		if _, e := v.Verify(context.Background(), unknown); e != nil {
			t.Fatal(e)
		}
	})
}
func TestTTLAndStale(t *testing.T) {
	for _, grace := range []time.Duration{0, 2 * time.Second} {
		synctest.Test(t, func(t *testing.T) {
			key := rsaKey(t)
			cfg := config()
			cfg.CacheTTL = time.Second
			cfg.StaleGrace = grace
			ep := &endpoint{status: 200, body: keySet(keyJSON("k", key))}
			v := verifier(t, cfg, ep)
			raw := sign(t, tokenjwt.SigningMethodRS256, "k", key, claims())
			if _, e := v.Verify(context.Background(), raw); e != nil {
				t.Fatal(e)
			}
			ep.set("", errors.New("offline"))
			time.Sleep(time.Second)
			_, e := v.Verify(context.Background(), raw)
			if grace == 0 && !errors.Is(e, ErrKeySetUnavailable) || grace > 0 && e != nil {
				t.Fatal(grace, e)
			}
			if grace > 0 {
				time.Sleep(grace)
				if _, e := v.Verify(context.Background(), raw); !errors.Is(e, ErrKeySetUnavailable) {
					t.Fatal(e)
				}
			}
		})
	}
}

type contextKey struct{}

func TestCoalescingAndInitiatorCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := rsaKey(t)
		gate := make(chan struct{})
		ep := &endpoint{status: 200, body: keySet(keyJSON("k", key)), gate: gate, entered: make(chan struct{})}
		v := verifier(t, config(), ep)
		raw := sign(t, tokenjwt.SigningMethodRS256, "k", key, claims())
		ctx, cancel := context.WithCancelCause(context.WithValue(context.Background(), contextKey{}, "value"))
		first := make(chan error, 1)
		go func() { _, e := v.Verify(ctx, raw); first <- e }()
		<-ep.entered
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				if _, e := v.Verify(context.Background(), raw); e != nil {
					t.Error(e)
				}
			})
		}
		synctest.Wait()
		cause := errors.New("caller canceled")
		cancel(cause)
		if e := <-first; !errors.Is(e, context.Canceled) || !errors.Is(e, cause) {
			t.Fatal(e)
		}
		if ep.seen.Err() != nil || ep.seen.Value(contextKey{}) != "value" {
			t.Fatal("fetch lost independence/context")
		}
		close(gate)
		wg.Wait()
		if ep.calls.Load() != 1 {
			t.Fatal(ep.calls.Load())
		}
	})
}
func TestRefreshTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := config()
		cfg.RefreshTimeout = time.Second
		gate := make(chan struct{})
		ep := &endpoint{gate: gate}
		v := verifier(t, cfg, ep)
		if e := v.Refresh(context.Background()); !errors.Is(e, ErrKeySetUnavailable) || !errors.Is(e, context.DeadlineExceeded) {
			t.Fatal(e)
		}
	})
}
func TestClockSkew(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := rsaKey(t)
		cfg := config()
		cfg.ClockSkew = 2 * time.Second
		v := verifier(t, cfg, &endpoint{status: 200, body: keySet(keyJSON("k", key))})
		m := claims()
		m["exp"] = time.Now().Unix()
		raw := sign(t, tokenjwt.SigningMethodRS256, "k", key, m)
		if _, e := v.Verify(context.Background(), raw); e != nil {
			t.Fatal(e)
		}
		time.Sleep(2 * time.Second)
		if _, e := v.Verify(context.Background(), raw); !errors.Is(e, ErrExpiredToken) {
			t.Fatal(e)
		}
	})
}

func TestRotationAndSkewBoundaries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := ecKey(t, elliptic.P256())
		cfg := config()
		cfg.Algorithms = []string{"ES256"}
		cfg.CacheTTL = time.Second
		cfg.ClockSkew = 2 * time.Second
		ep := &endpoint{status: 200, body: keySet(keyJSON("old", key))}
		v := verifier(t, cfg, ep)
		old := sign(t, tokenjwt.SigningMethodES256, "old", key, claims())
		if _, e := v.Verify(context.Background(), old); e != nil {
			t.Fatal(e)
		}
		ep.set(keySet(keyJSON("new", key)), nil)
		time.Sleep(time.Second)
		m := claims()
		m["nbf"] = time.Now().Add(2 * time.Second).Unix()
		m["iat"] = time.Now().Add(2 * time.Second).Unix()
		if _, e := v.Verify(context.Background(), sign(t, tokenjwt.SigningMethodES256, "new", key, m)); e != nil {
			t.Fatal(e)
		}
		m["iat"] = time.Now().Add(3 * time.Second).Unix()
		if _, e := v.Verify(context.Background(), sign(t, tokenjwt.SigningMethodES256, "new", key, m)); !errors.Is(e, ErrInvalidToken) {
			t.Fatal(e)
		}
		if ep.calls.Load() != 2 {
			t.Fatal(ep.calls.Load())
		}
	})
}
func TestCanceledRefreshBeforeAdmission(t *testing.T) {
	v := verifier(t, config(), &endpoint{status: 503})
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("stop")
	cancel(cause)
	if e := v.Refresh(ctx); !errors.Is(e, cause) || !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := v.Verify(ctx, "bad"); !errors.Is(e, cause) {
		t.Fatal(e)
	}
}

func TestStrictPayloadAndJOSE(t *testing.T) {
	key := rsaKey(t)
	ep := &endpoint{status: 200, body: keySet(keyJSON("k", key))}
	v := verifier(t, config(), ep)
	m := claims()
	body, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"k"}`))
	payload := base64.RawURLEncoding.EncodeToString(append(body, []byte(`{}`)...))
	unsigned := header + "." + payload
	signature, e := tokenjwt.SigningMethodRS256.Sign(unsigned, key)
	if e != nil {
		t.Fatal(e)
	}
	raw := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	if _, e := v.Verify(context.Background(), raw); !errors.Is(e, ErrInvalidToken) || ep.calls.Load() != 0 {
		t.Fatal(e, ep.calls.Load())
	}
	for _, h := range []map[string]any{{"crit": []string{"custom"}}, {"b64": false}} {
		token := tokenjwt.NewWithClaims(tokenjwt.SigningMethodRS256, claims())
		token.Header["kid"] = "k"
		for k, value := range h {
			token.Header[k] = value
		}
		raw, e := token.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := v.Verify(context.Background(), raw); !errors.Is(e, ErrInvalidToken) || ep.calls.Load() != 0 {
			t.Fatal(e)
		}
	}
	token := tokenjwt.NewWithClaims(tokenjwt.SigningMethodRS256, claims())
	token.Header["kid"] = "k"
	token.Header["jku"] = "http://attacker.test/keys"
	raw, e = token.SignedString(key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := v.Verify(context.Background(), raw); e != nil {
		t.Fatal(e)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type countedBody struct {
	io.Reader
	closed *atomic.Int64
}

func (b countedBody) Close() error { b.closed.Add(1); return nil }
func TestBodyBoundaryAndCallbacksOutsideLock(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	body := keySet(keyJSON("k", key))
	cfg := config()
	cfg.Algorithms = []string{"ES256"}
	cfg.MaxJWKSBytes = len(body)
	ep := &endpoint{status: 200, body: body}
	v := verifier(t, cfg, ep)
	raw := sign(t, tokenjwt.SigningMethodES256, "k", key, claims())
	if _, e := v.Verify(context.Background(), raw); e != nil {
		t.Fatal(e)
	}
	var closed atomic.Int64
	v.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if _, e := v.Verify(r.Context(), raw); e != nil {
			t.Error(e)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: countedBody{strings.NewReader(body), &closed}, Request: r}, nil
	})
	if e := v.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	if closed.Load() != 1 {
		t.Fatal("body not closed", closed.Load())
	}
	v.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: countedBody{strings.NewReader(body + " "), &closed}, Request: r}, nil
	})
	if e := v.Refresh(context.Background()); !errors.Is(e, ErrKeySetUnavailable) {
		t.Fatal(e)
	}
	if closed.Load() != 2 {
		t.Fatal("failed body not closed")
	}
}

func TestSignatureConfusionAndWrongCurve(t *testing.T) {
	rsaPrivate := rsaKey(t)
	p256 := ecKey(t, elliptic.P256())
	p384 := ecKey(t, elliptic.P384())
	cfg := config()
	cfg.Algorithms = []string{"RS256", "ES256"}
	ep := &endpoint{status: 200, body: keySet(keyJSON("rsa", rsaPrivate), keyJSON("ec", p384))}
	v := verifier(t, cfg, ep)
	forged := tokenjwt.NewWithClaims(tokenjwt.SigningMethodHS256, claims())
	forged.Header["alg"] = "RS256"
	forged.Header["kid"] = "rsa"
	raw, e := forged.SignedString(rsaPrivate.N.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	if _, e := v.Verify(context.Background(), raw); !errors.Is(e, ErrInvalidToken) {
		t.Fatal("algorithm confusion accepted", e)
	}
	valid := sign(t, tokenjwt.SigningMethodRS256, "rsa", rsaPrivate, claims())
	parts := strings.Split(valid, ".")
	sig, e := base64.RawURLEncoding.DecodeString(parts[2])
	if e != nil {
		t.Fatal(e)
	}
	sig[0] ^= 1
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	if _, e := v.Verify(context.Background(), strings.Join(parts, ".")); !errors.Is(e, ErrInvalidToken) {
		t.Fatal("bad signature accepted", e)
	}
	if _, e := v.Verify(context.Background(), sign(t, tokenjwt.SigningMethodES256, "ec", p256, claims())); !errors.Is(e, ErrInvalidToken) {
		t.Fatal("wrong curve accepted", e)
	}
}

func TestJWKSInvalidUTF8(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	body := []byte(keySet(keyJSON("k", key)))
	body = []byte(strings.Replace(string(body), `"kid":"k"`, "\"kid\":\"\xff\"", 1))
	cfg := config()
	cfg.Algorithms = []string{"ES256"}
	v := verifier(t, cfg, &endpoint{status: 200, body: string(body)})
	if e := v.Refresh(context.Background()); !errors.Is(e, ErrKeySetUnavailable) {
		t.Fatal("invalid UTF-8 JWKS accepted", e)
	}
}

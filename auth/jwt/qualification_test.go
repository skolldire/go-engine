package jwt

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tokenjwt "github.com/golang-jwt/jwt/v5"
)

func TestStressRotation(t *testing.T) {
	if os.Getenv("AUTHJWT_STRESS") != "1" {
		t.Skip("separate stress campaign")
	}
	synctest.Test(t, func(t *testing.T) {
		first, second := rsaKey(t), rsaKey(t)
		ep := &endpoint{status: 200, body: keySet(keyJSON("old", first))}
		cfg := config()
		cfg.CacheTTL = time.Second
		v := verifier(t, cfg, ep)
		old := sign(t, tokenjwt.SigningMethodRS256, "old", first, claims())
		newToken := sign(t, tokenjwt.SigningMethodRS256, "new", second, claims())
		if _, e := v.Verify(context.Background(), old); e != nil {
			t.Fatal(e)
		}
		for round := range 8 {
			time.Sleep(time.Second)
			var wg sync.WaitGroup
			if round%2 == 0 {
				ep.set(keySet(keyJSON("new", second)), nil)
			} else {
				ep.set("", errors.New("offline"))
			}
			for range 64 {
				wg.Go(func() {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if round%3 == 0 {
						cancel()
					}
					_, err := v.Verify(ctx, newToken)
					if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrKeySetUnavailable) {
						t.Error(err)
					}
				})
			}
			wg.Wait()
		}
	})
}
func TestRetainedStateAndNoLeaks(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	ep := &endpoint{status: 200, body: keySet(keyJSON("k", key))}
	v := verifier(t, config(), ep)
	if e := v.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	for i := range 1000 {
		_, _ = v.key(context.Background(), strings.Repeat("x", i%32+1))
	}
	v.mu.Lock()
	if len(v.keys) != 1 || v.active != nil {
		t.Fatal("unexpected retained state")
	}
	v.mu.Unlock()
	var b strings.Builder
	if e := pprof.Lookup("goroutineleak").WriteTo(&b, 1); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(b.String(), "total 0") {
		t.Fatal(b.String())
	}
}
func FuzzToken(f *testing.F) {
	f.Add("bad")
	f.Add("eyJhbGciOiJIUzI1NiJ9.e30.AA")
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 32<<10 {
			return
		}
		cfg := config()
		ep := &endpoint{status: 503}
		v := verifier(t, cfg, ep)
		got, e := v.Verify(context.Background(), raw)
		if e == nil {
			t.Fatal("untrusted fuzz token accepted", got.Subject)
		}
		if strings.Contains(e.Error(), raw) && len(raw) > 128 {
			t.Fatal("reflected token")
		}
	})
}
func FuzzJWKS(f *testing.F) {
	f.Add([]byte(`{"keys":[]}`))
	f.Add([]byte(`{"keys":[{"kty":"RSA","kid":"x","n":"AQ","e":"Aw"}]}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 16<<10 {
			return
		}
		keys, e := parseJWKS(body)
		if e == nil {
			if len(keys) == 0 {
				t.Fatal("empty success")
			}
			for kid, k := range keys {
				if kid == "" || k.public == nil {
					t.Fatal("invalid key")
				}
			}
		}
	})
}
func FuzzJOSE(f *testing.F) {
	f.Add(`{"alg":"none","kid":"x"}`)
	f.Add(`{"alg":"RS256","kid":"x"}`)
	f.Fuzz(func(t *testing.T, header string) {
		if len(header) > 2048 {
			return
		}
		raw := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":9999999999}`)) + ".AA"
		v := verifier(t, config(), &endpoint{status: 503})
		if _, e := v.Verify(context.Background(), raw); e == nil {
			t.Fatal("unsigned token accepted")
		}
	})
}
func BenchmarkVerify(b *testing.B) {
	for _, name := range []string{"RS256", "ES256"} {
		b.Run(name, func(b *testing.B) {
			var key any
			var signing tokenjwt.SigningMethod
			if name == "RS256" {
				key = rsaKey(b)
				signing = tokenjwt.SigningMethodRS256
			} else {
				key = ecKey(b, elliptic.P256())
				signing = tokenjwt.SigningMethodES256
			}
			cfg := config()
			cfg.Algorithms = []string{name}
			v := verifier(b, cfg, &endpoint{status: 200, body: keySet(keyJSON("k", key))})
			raw := sign(b, signing, "k", key, claims())
			if _, e := v.Verify(context.Background(), raw); e != nil {
				b.Fatal(e)
			}
			pub := v.keys["k"].public
			for _, wrapped := range []bool{false, true} {
				b.Run(map[bool]string{false: "library", true: "verifier"}[wrapped], func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if wrapped {
							if _, e := v.Verify(context.Background(), raw); e != nil {
								b.Fatal(e)
							}
						} else {
							if _, e := tokenjwt.Parse(raw, func(*tokenjwt.Token) (any, error) { return pub, nil }, tokenjwt.WithValidMethods(cfg.Algorithms), tokenjwt.WithJSONNumber(), tokenjwt.WithExpirationRequired(), tokenjwt.WithIssuedAt(), tokenjwt.WithLeeway(cfg.ClockSkew), tokenjwt.WithIssuer(cfg.Issuer)); e != nil {
								b.Fatal(e)
							}
						}
					}
				})
			}
		})
	}
}

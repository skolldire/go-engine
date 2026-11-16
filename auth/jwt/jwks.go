package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

type verificationKey struct {
	public any
	alg    string
}
type rawJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func decodeInteger(s string) (*big.Int, error) {
	b, e := base64.RawURLEncoding.Strict().DecodeString(s)
	if e != nil || len(b) == 0 {
		return nil, errors.New("invalid JWK integer")
	}
	return new(big.Int).SetBytes(b), nil
}
func parseKey(raw json.RawMessage) (string, verificationKey, bool) {
	var k rawJWK
	if json.Unmarshal(raw, &k) != nil || k.Kid == "" || k.Use != "" && k.Use != "sig" || k.Alg != "" && !allowedAlgorithm(k.Alg) {
		return "", verificationKey{}, false
	}
	var public any
	switch k.Kty {
	case "RSA":
		n, e := decodeInteger(k.N)
		if e != nil || n.Sign() <= 0 || n.Bit(0) == 0 {
			return "", verificationKey{}, false
		}
		ex, e := decodeInteger(k.E)
		if e != nil || ex.BitLen() > strconv.IntSize-1 || ex.Int64() < 3 || ex.Bit(0) == 0 {
			return "", verificationKey{}, false
		}
		public = &rsa.PublicKey{N: n, E: int(ex.Int64())}
	case "EC":
		var curve elliptic.Curve
		size := 0
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
			size = 32
		case "P-384":
			curve = elliptic.P384()
			size = 48
		default:
			return "", verificationKey{}, false
		}
		xb, e := base64.RawURLEncoding.Strict().DecodeString(k.X)
		if e != nil || len(xb) != size {
			return "", verificationKey{}, false
		}
		yb, e := base64.RawURLEncoding.Strict().DecodeString(k.Y)
		if e != nil || len(yb) != size {
			return "", verificationKey{}, false
		}
		encoded := append([]byte{4}, xb...)
		encoded = append(encoded, yb...)
		parsed, err := ecdsa.ParseUncompressedPublicKey(curve, encoded)
		if err != nil {
			return "", verificationKey{}, false
		}
		public = parsed
	default:
		return "", verificationKey{}, false
	}
	key := verificationKey{public: public, alg: k.Alg}
	if k.Alg != "" && !key.supports(k.Alg) {
		return "", verificationKey{}, false
	}
	return k.Kid, key, true
}
func (k verificationKey) supports(alg string) bool {
	if k.alg != "" && k.alg != alg {
		return false
	}
	switch key := k.public.(type) {
	case *rsa.PublicKey:
		return strings.HasPrefix(alg, "RS") || strings.HasPrefix(alg, "PS")
	case *ecdsa.PublicKey:
		return alg == "ES256" && key.Curve == elliptic.P256() || alg == "ES384" && key.Curve == elliptic.P384()
	}
	return false
}
func parseJWKS(body []byte) (map[string]verificationKey, error) {
	if !utf8.Valid(body) {
		return nil, errors.New("invalid JWKS JSON")
	}
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if e := json.Unmarshal(body, &set); e != nil {
		return nil, errors.New("invalid JWKS JSON")
	}
	out := make(map[string]verificationKey)
	duplicates := make(map[string]bool)
	for _, raw := range set.Keys {
		kid, key, ok := parseKey(raw)
		if !ok {
			continue
		}
		if _, exists := out[kid]; exists || duplicates[kid] {
			delete(out, kid)
			duplicates[kid] = true
			continue
		}
		out[kid] = key
	}
	if len(out) == 0 {
		return nil, errors.New("no usable JWKS keys")
	}
	return out, nil
}
func (v *Verifier) fetch(ctx context.Context) (map[string]verificationKey, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if e != nil {
		return nil, errors.New("invalid JWKS request")
	}
	// Preserve caller transport/jar/timeout and redirect policy without mutating it.
	client := *v.client
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !validEndpoint(req.URL.String()) {
			return errors.New("invalid JWKS redirect")
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("too many JWKS redirects")
		}
		return nil
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unexpected JWKS status")
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, int64(v.cfg.MaxJWKSBytes)))
	if e != nil {
		return nil, e
	}
	var extra [1]byte
	n, e := io.ReadFull(resp.Body, extra[:])
	if n != 0 {
		return nil, errors.New("JWKS size exceeded")
	}
	if e != nil && !errors.Is(e, io.EOF) {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, errors.Join(e, context.Cause(ctx))
	}
	return parseJWKS(body)
}
func unavailable(e error) error { return fmt.Errorf("%w: refresh failed: %w", ErrKeySetUnavailable, e) }

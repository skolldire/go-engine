package jwt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	tokenjwt "github.com/golang-jwt/jwt/v5"
)

func stringList(value any) ([]string, bool) {
	switch v := value.(type) {
	case string:
		return []string{v}, true
	case []any:
		out := make([]string, len(v))
		for i, x := range v {
			s, ok := x.(string)
			if !ok {
				return nil, false
			}
			out[i] = s
		}
		return out, true
	case []string:
		return append([]string(nil), v...), true
	}
	return nil, false
}
func cloneValue(value any) any {
	switch x := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = cloneValue(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = cloneValue(v)
		}
		return out
	case []string:
		return append([]string(nil), x...)
	default:
		return value
	}
}
func invalid(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidToken, reason) }
func validateDateTypes(m tokenjwt.MapClaims) bool {
	for _, name := range []string{"exp", "nbf", "iat"} {
		raw, exists := m[name]
		if !exists {
			continue
		}
		number, ok := raw.(json.Number)
		if !ok {
			return false
		}
		n, e := number.Float64()
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n >= float64(math.MaxInt64) || n < float64(math.MinInt64) {
			return false
		}
	}
	return true
}
func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	if ctx == nil {
		return Claims{}, ErrNilContext
	}
	if raw == "" || len(raw) > v.cfg.MaxTokenBytes {
		return Claims{}, invalid("empty or oversized token")
	}
	if ctx.Err() != nil {
		return Claims{}, canceled(ctx)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Claims{}, invalid("malformed token")
	}
	for _, part := range parts[:2] {
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(part)
		if err != nil || !utf8.Valid(decoded) || !json.Valid(decoded) {
			return Claims{}, invalid("malformed JSON segment")
		}
	}
	var keyErr error
	parsed, e := tokenjwt.Parse(raw, func(token *tokenjwt.Token) (any, error) {
		if _, exists := token.Header["crit"]; exists {
			keyErr = invalid("unsupported critical header")
			return nil, keyErr
		}
		if value, exists := token.Header["b64"]; exists && value != true {
			keyErr = invalid("unsupported payload encoding")
			return nil, keyErr
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			keyErr = invalid("missing key identifier")
			return nil, keyErr
		}
		m, ok := token.Claims.(tokenjwt.MapClaims)
		if !ok || !validateDateTypes(m) {
			keyErr = invalid("invalid date claim")
			return nil, keyErr
		}
		key, err := v.key(ctx, kid)
		if err != nil {
			keyErr = err
			return nil, err
		}
		if !key.supports(token.Method.Alg()) {
			keyErr = invalid("incompatible key")
			return nil, keyErr
		}
		return key.public, nil
	}, tokenjwt.WithValidMethods(v.cfg.Algorithms), tokenjwt.WithJSONNumber(), tokenjwt.WithStrictDecoding(), tokenjwt.WithExpirationRequired(), tokenjwt.WithIssuedAt(), tokenjwt.WithLeeway(v.cfg.ClockSkew), tokenjwt.WithIssuer(v.cfg.Issuer))
	if e != nil {
		if keyErr != nil {
			return Claims{}, keyErr
		}
		if errors.Is(e, tokenjwt.ErrTokenExpired) {
			return Claims{}, ErrExpiredToken
		}
		return Claims{}, invalid("signature or claims rejected")
	}
	if ctx.Err() != nil {
		return Claims{}, canceled(ctx)
	}
	m := parsed.Claims.(tokenjwt.MapClaims)
	aud, ok := stringList(m[v.cfg.AudienceClaim])
	if !ok {
		return Claims{}, invalid("invalid audience claim")
	}
	matched := false
	for _, a := range aud {
		if a == v.cfg.Audience {
			matched = true
		}
	}
	if !matched {
		return Claims{}, invalid("audience mismatch")
	}
	var groups []string
	if value, exists := m[v.cfg.GroupsClaim]; exists {
		list, ok := value.([]any)
		if !ok {
			return Claims{}, invalid("invalid groups claim")
		}
		groups, ok = stringList(list)
		if !ok {
			return Claims{}, invalid("invalid groups claim")
		}
	}
	subject, id := "", ""
	if value, exists := m["sub"]; exists {
		var ok bool
		subject, ok = value.(string)
		if !ok {
			return Claims{}, invalid("invalid subject claim")
		}
	}
	if value, exists := m["jti"]; exists {
		var ok bool
		id, ok = value.(string)
		if !ok {
			return Claims{}, invalid("invalid identifier claim")
		}
	}
	exp, _ := m.GetExpirationTime()
	nbf, _ := m.GetNotBefore()
	iat, _ := m.GetIssuedAt()
	custom := make(map[string]any)
	for name, value := range m {
		switch name {
		case "iss", "sub", "jti", "exp", "nbf", "iat", v.cfg.AudienceClaim, v.cfg.GroupsClaim:
			continue
		}
		custom[name] = cloneValue(value)
	}
	return Claims{Subject: subject, Issuer: v.cfg.Issuer, Audience: aud, ID: id, Expiry: exp.Time, NotBefore: dateTime(nbf), IssuedAt: dateTime(iat), Groups: groups, Custom: custom}, nil
}
func dateTime(d *tokenjwt.NumericDate) time.Time {
	if d == nil {
		return time.Time{}
	}
	return d.Time
}

package http

import (
	"context"

	"github.com/skolldire/go-engine/auth/jwt"
)

type claimsKey struct{}

func cloneJSON(value any) any {
	switch x := value.(type) {
	case map[string]any:
		if x == nil {
			return map[string]any(nil)
		}
		m := make(map[string]any, len(x))
		for k, v := range x {
			m[k] = cloneJSON(v)
		}
		return m
	case []any:
		if x == nil {
			return []any(nil)
		}
		s := make([]any, len(x))
		for i, v := range x {
			s[i] = cloneJSON(v)
		}
		return s
	case []string:
		return append([]string(nil), x...)
	default:
		return value
	}
}
func copyClaims(c jwt.Claims) jwt.Claims {
	c.Audience = append([]string(nil), c.Audience...)
	c.Groups = append([]string(nil), c.Groups...)
	if c.Custom != nil {
		c.Custom = cloneJSON(c.Custom).(map[string]any)
	}
	return c
}
func ContextWithClaims(ctx context.Context, c jwt.Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, copyClaims(c))
}
func ClaimsFromContext(ctx context.Context) (jwt.Claims, bool) {
	if ctx == nil {
		return jwt.Claims{}, false
	}
	c, ok := ctx.Value(claimsKey{}).(jwt.Claims)
	if !ok {
		return jwt.Claims{}, false
	}
	return copyClaims(c), true
}

package router

import (
	"context"
	"net/http"
)

// claimsKey is the unexported context key for JWT claims, avoiding collisions
// with other packages that use context.WithValue.
type claimsKey struct{}

// Claims holds the validated JWT payload injected into the request context by
// JWTAuth. Retrieve it in handlers with ClaimsFromContext or MustClaimsFromContext.
type Claims struct {
	// Sub is the subject — the unique user ID (e.g. Cognito User Sub).
	Sub string

	// Email is the user's email address.
	Email string

	// Username is the value of the "cognito:username" claim.
	Username string

	// Groups lists the groups the user belongs to, extracted from GroupsClaim.
	Groups []string

	// TokenUse is "id" or "access".
	TokenUse string

	// Raw holds all claims from the JWT payload for access to custom attributes
	// (e.g. "custom:school_id", "custom:role").
	Raw map[string]interface{}
}

// ClaimsFromContext returns the validated Claims stored in ctx by JWTAuth.
// Returns nil if the request did not pass through JWTAuth or was on a SkipPath.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey{}).(*Claims)
	return c
}

// MustClaimsFromContext returns the validated Claims stored in ctx by JWTAuth.
// Panics if no claims are present — use only on routes that are guaranteed
// to be behind JWTAuth and are not in SkipPaths.
func MustClaimsFromContext(ctx context.Context) *Claims {
	c := ClaimsFromContext(ctx)
	if c == nil {
		panic("jwt: claims not found in context — is JWTAuth middleware applied?")
	}
	return c
}

// InjectClaimsForTest injects claims into ctx using the same mechanism as JWTAuth.
// Use in tests that need to simulate an authenticated request without going
// through the full JWT validation flow.
// Must not be called from production code.
func InjectClaimsForTest(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, c)
}

// RequireGroup returns a chi middleware that allows only requests where the
// authenticated user belongs to at least one of the specified groups.
// Must be applied after JWTAuth — returns 401 if claims are absent, 403 if
// the user does not belong to any of the required groups.
//
// Example:
//
//	r.With(RequireGroup("school_admin")).Get("/admin/schools", handler)
func RequireGroup(groups ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		allowed[g] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, http.StatusUnauthorized, "missing_token")
				return
			}
			for _, g := range claims.Groups {
				if _, ok := allowed[g]; ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeAuthError(w, http.StatusForbidden, "forbidden")
		})
	}
}

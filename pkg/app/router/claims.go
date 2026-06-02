package router

import (
	"context"
	"net/http"
)

// claimsKey is the unexported context key for JWT claims, avoiding collisions
// with other packages that use context.WithValue.
type claimsKey struct{}

// Claims holds the validated JWT payload injected into the request context by
// JWTMiddleware. Retrieve it in handlers with ClaimsFromContext.
type Claims struct {
	// Sub is the subject — the unique user ID (Cognito User Sub).
	Sub string

	// Email is the user's email address.
	Email string

	// Username is the value of the "cognito:username" claim.
	Username string

	// Groups lists the Cognito groups the user belongs to ("cognito:groups").
	Groups []string

	// TokenUse is "id" or "access".
	TokenUse string

	// Raw holds all claims from the JWT payload for access to custom attributes
	// (e.g. "custom:school_id", "custom:role").
	Raw map[string]interface{}
}

// ClaimsFromContext returns the validated Claims stored in ctx by JWTMiddleware.
// Returns nil if the request did not pass through JWTMiddleware or the token
// was not validated (e.g. a SkipPaths route).
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey{}).(*Claims)
	return c
}

// RequireGroup returns a chi middleware that allows only requests where the
// authenticated user belongs to at least one of the specified Cognito groups.
// It must be applied after JWTMiddleware — if claims are absent it returns 401.
//
// Example:
//
//	router.With(RequireGroup("admins", "staff")).Get("/admin/users", handler)
func RequireGroup(groups ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		allowed[g] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeJSONError(w, http.StatusUnauthorized, "ER-401", "unauthenticated")
				return
			}
			for _, g := range claims.Groups {
				if _, ok := allowed[g]; ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeJSONError(w, http.StatusForbidden, "ER-403", "insufficient group permissions")
		})
	}
}

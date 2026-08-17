package testutil_test

import (
	"testing"

	"github.com/skolldire/go-engine/pkg/router"
	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── context helpers ───────────────────────────────────────────────────────────

func TestNewTestContext_ClaimsReadable(t *testing.T) {
	ctx := testutil.NewTestContext(&router.Claims{
		Sub:    "user-abc",
		Email:  "dev@example.com",
		Groups: []string{"admins"},
	})

	claims := router.ClaimsFromContext(ctx)
	require.NotNil(t, claims)
	assert.Equal(t, "user-abc", claims.Sub)
	assert.Equal(t, "dev@example.com", claims.Email)
	assert.Contains(t, claims.Groups, "admins")
}

func TestNewEmptyTestContext_NoClaims(t *testing.T) {
	ctx := testutil.NewEmptyTestContext()
	assert.Nil(t, router.ClaimsFromContext(ctx))
}

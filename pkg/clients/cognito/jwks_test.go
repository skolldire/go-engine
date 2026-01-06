package cognito

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewJWKSClient(t *testing.T) {
	url := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_TestPool/.well-known/jwks.json"
	client := NewJWKSClient(url)

	assert.NotNil(t, client)
	assert.Equal(t, url, client.url)
	assert.Equal(t, DefaultJWKSCacheTTL, client.cacheTTL)
	assert.Equal(t, DefaultJWKSRefreshThreshold, client.refreshThreshold)
	assert.NotNil(t, client.cache)
}

func TestJWKSClient_ClearCache(t *testing.T) {
	client := NewJWKSClient("https://test.com/jwks.json")

	// Agregar algo al cache manualmente (para testing)
	client.mu.Lock()
	client.cache["test-kid"] = nil
	client.lastFetch = time.Now()
	client.mu.Unlock()

	client.ClearCache()

	client.mu.RLock()
	assert.Empty(t, client.cache)
	assert.True(t, client.lastFetch.IsZero())
	client.mu.RUnlock()
}

func TestJWKSClient_shouldRefresh(t *testing.T) {
	client := NewJWKSClient("https://test.com/jwks.json")

	// Sin fetch previo, debe necesitar refresh
	assert.True(t, client.shouldRefresh())

	// Con fetch reciente, no debe necesitar refresh
	client.mu.Lock()
	client.lastFetch = time.Now()
	client.mu.Unlock()
	assert.False(t, client.shouldRefresh())

	// Con fetch antiguo (más allá del threshold), debe necesitar refresh
	client.mu.Lock()
	client.lastFetch = time.Now().Add(-(DefaultJWKSCacheTTL - DefaultJWKSRefreshThreshold + time.Minute))
	client.mu.Unlock()
	assert.True(t, client.shouldRefresh())
}

func TestJWKSClient_GetKey_WithoutRealEndpoint(t *testing.T) {
	// Este test verifica que GetKey maneja errores cuando no hay endpoint real
	// No podemos hacer un test completo sin un endpoint JWKS real o un mock
	client := NewJWKSClient("https://invalid-url-that-does-not-exist.com/jwks.json")
	ctx := context.Background()

	_, err := client.GetKey(ctx, "test-kid")
	// Esperamos un error porque la URL no existe
	assert.Error(t, err)
}

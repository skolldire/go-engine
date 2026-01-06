package cognito

import (
	"context"
	"crypto/rsa"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

const (
	// DefaultJWKSCacheTTL es el TTL por defecto para el cache de claves JWKS (1 hora)
	DefaultJWKSCacheTTL = 1 * time.Hour
	// DefaultJWKSRefreshThreshold es el umbral antes de expirar para refrescar (10 minutos)
	DefaultJWKSRefreshThreshold = 10 * time.Minute
)

// JWKSClient maneja la obtención y cache de claves públicas JWKS de Cognito
type JWKSClient struct {
	url              string
	cache            map[string]*rsa.PublicKey
	cacheTTL         time.Duration
	lastFetch        time.Time
	mu               sync.RWMutex
	refreshThreshold time.Duration
}

// NewJWKSClient crea un nuevo cliente JWKS
func NewJWKSClient(url string) *JWKSClient {
	return &JWKSClient{
		url:              url,
		cache:            make(map[string]*rsa.PublicKey),
		cacheTTL:         DefaultJWKSCacheTTL,
		refreshThreshold: DefaultJWKSRefreshThreshold,
	}
}

// GetKey obtiene una clave pública por su Key ID (kid)
// Implementa cache con refresh automático antes de expirar
func (c *JWKSClient) GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, exists := c.cache[kid]
	needsRefresh := c.shouldRefresh()
	c.mu.RUnlock()

	// Si la clave existe y no necesita refresh, retornarla
	if exists && !needsRefresh {
		return key, nil
	}

	// Si necesita refresh o no existe, obtener desde el endpoint
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check: otro goroutine pudo haber actualizado el cache
	if key, exists := c.cache[kid]; exists && !c.shouldRefresh() {
		return key, nil
	}

	// Obtener claves desde el endpoint JWKS
	keys, err := c.fetchKeys(ctx)
	if err != nil {
		// Si falla pero tenemos una clave en cache, usar la cacheada
		if cachedKey, exists := c.cache[kid]; exists {
			return cachedKey, nil
		}
		return nil, fmt.Errorf("failed to fetch JWKS keys: %w", err)
	}

	// Actualizar cache
	c.cache = keys
	c.lastFetch = time.Now()

	// Retornar la clave solicitada
	foundKey, exists := c.cache[kid]
	if !exists {
		return nil, fmt.Errorf("key with kid '%s' not found in JWKS", kid)
	}

	return foundKey, nil
}

// fetchKeys obtiene las claves desde el endpoint JWKS de Cognito
func (c *JWKSClient) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	set, err := jwk.Fetch(ctx, c.url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS set: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)

	// Iterar sobre las claves en el set
	iter := set.Keys(ctx)
	for iter.Next(ctx) {
		pair := iter.Pair()
		key := pair.Value.(jwk.Key)

		// Obtener kid
		kid := key.KeyID()
		if kid == "" {
			continue
		}

		// Convertir a RSA public key
		var pubkey rsa.PublicKey
		if err := key.Raw(&pubkey); err != nil {
			continue
		}

		keys[kid] = &pubkey
	}

	return keys, nil
}

// shouldRefresh determina si el cache necesita ser refrescado
func (c *JWKSClient) shouldRefresh() bool {
	if c.lastFetch.IsZero() {
		return true
	}
	elapsed := time.Since(c.lastFetch)
	return elapsed >= (c.cacheTTL - c.refreshThreshold)
}

// ClearCache limpia el cache de claves (útil para testing)
func (c *JWKSClient) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*rsa.PublicKey)
	c.lastFetch = time.Time{}
}

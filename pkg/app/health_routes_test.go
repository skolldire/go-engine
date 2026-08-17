package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/config/viper"
	"github.com/skolldire/go-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type okChecker struct{}

func (okChecker) Check(context.Context) error { return nil }

// TestBuilder_MountsKubernetesProbes is the regression test for
// health.HTTPHandler.Routes defining /live, /ready and /deps while the builder
// mounted only /health, so every Kubernetes probe returned 404.
func TestBuilder_MountsKubernetesProbes(t *testing.T) {
	b := NewAppBuilder().
		WithContext(context.Background()).
		SetLogger(&mockLogger{})
	b.engine.Conf = &viper.Config{}

	b = b.WithRouter().
		WithHealth(health.Config{Timeout: time.Second}).
		RegisterHealthChecker("dep", okChecker{})

	require.Empty(t, b.GetErrors())
	require.NotNil(t, b.engine.Router)

	for _, path := range []string{"/health", "/live", "/ready", "/deps"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			b.engine.Router.Router().ServeHTTP(rec, req)

			assert.NotEqual(t, http.StatusNotFound, rec.Code,
				"%s must be mounted by the builder", path)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestBuilder_MountHealthIsIdempotent guards the mount guard itself: calling
// the health builders in either order must not register duplicate routes.
func TestBuilder_MountHealthIsIdempotent(t *testing.T) {
	b := NewAppBuilder().
		WithContext(context.Background()).
		SetLogger(&mockLogger{})
	b.engine.Conf = &viper.Config{}

	b = b.WithHealth(health.Config{Timeout: time.Second}).
		WithRouter().
		RegisterHealthChecker("dep", okChecker{})

	require.Empty(t, b.GetErrors())

	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rec := httptest.NewRecorder()
	assert.NotPanics(t, func() {
		b.engine.Router.Router().ServeHTTP(rec, req)
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

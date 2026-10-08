package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/skolldire/go-engine/example/internal/handler"
	"github.com/skolldire/go-engine/example/internal/repository"
	"github.com/skolldire/go-engine/example/internal/usecase"
	"github.com/skolldire/go-engine/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCache stands in for Redis. The repository depends on a narrow interface
// rather than on the provider, which is what makes this possible without a
// container — and is the pattern the example is demonstrating.
type fakeCache struct {
	mu     sync.Mutex
	values map[string]string
	failOn string
}

func newFakeCache() *fakeCache { return &fakeCache{values: map[string]string{}} }

func (f *fakeCache) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if key == f.failOn {
		return "", errors.New("cache unavailable")
	}
	v, ok := f.values[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeCache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if key == f.failOn {
		return errors.New("cache unavailable")
	}
	f.values[key] = value.(string)
	return nil
}

// newTestService builds the real service over a fake cache and returns an HTTP
// server for it, so the tests exercise the same wiring main.go uses.
func newTestService(t *testing.T, cache *fakeCache) *httptest.Server {
	t.Helper()

	eng, err := engine.New(context.Background(),
		engine.WithConfigDir(repoConfigDir(t)),
		engine.WithConfigFiles("application", "application-test"),
		engine.WithRouter(),
		engine.WithHealth(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close(context.Background()) })

	orders := repository.NewOrderRepository(cache, time.Hour)
	handler.NewOrder(
		usecase.NewCreateOrder(orders),
		usecase.NewGetOrder(orders),
	).Register(eng.Router())

	srv := httptest.NewServer(eng.Router().Router())
	t.Cleanup(srv.Close)

	return srv
}

func TestService_CreateAndFetchOrder(t *testing.T) {
	srv := newTestService(t, newFakeCache())

	body, err := json.Marshal(repository.Order{ID: "o-1", Item: "book", Total: 12.5})
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/orders", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	got, err := http.Get(srv.URL + "/orders/o-1")
	require.NoError(t, err)
	defer func() { _ = got.Body.Close() }()
	require.Equal(t, http.StatusOK, got.StatusCode)

	var order repository.Order
	require.NoError(t, json.NewDecoder(got.Body).Decode(&order))
	assert.Equal(t, "book", order.Item)
	assert.InDelta(t, 12.5, order.Total, 0.001)
}

func TestService_RejectsInvalidOrder(t *testing.T) {
	srv := newTestService(t, newFakeCache())

	cases := map[string]repository.Order{
		"missing id":    {Item: "book", Total: 1},
		"missing item":  {ID: "o-2", Total: 1},
		"total is zero": {ID: "o-3", Item: "book"},
	}

	for name, order := range cases {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(order)
			require.NoError(t, err)

			resp, err := http.Post(srv.URL+"/orders", "application/json", bytes.NewReader(body))
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
				"a validation failure must not read as a server error")
		})
	}
}

func TestService_MalformedBodyIsRejected(t *testing.T) {
	srv := newTestService(t, newFakeCache())

	resp, err := http.Post(srv.URL+"/orders", "application/json", bytes.NewReader([]byte("{")))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestService_MissingOrderIsNotFound(t *testing.T) {
	srv := newTestService(t, newFakeCache())

	resp, err := http.Get(srv.URL + "/orders/never-created")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestService_StorageFailureIsAServerError separates a caller mistake from a
// dependency failure, which is the distinction that decides whether a page is
// raised.
func TestService_StorageFailureIsAServerError(t *testing.T) {
	cache := newFakeCache()
	cache.failOn = "order:o-9"

	srv := newTestService(t, cache)

	body, err := json.Marshal(repository.Order{ID: "o-9", Item: "book", Total: 1})
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/orders", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// TestService_ProbesAreServed proves the engine mounts the Kubernetes probes on
// the same router the application uses.
func TestService_ProbesAreServed(t *testing.T) {
	srv := newTestService(t, newFakeCache())

	for _, path := range []string{"/health", "/live", "/ready", "/deps"} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err, path)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

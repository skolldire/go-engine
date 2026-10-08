package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statusServer(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(calls.Add(1)) - 1
		if n >= len(statuses) {
			n = len(statuses) - 1
		}
		w.WriteHeader(statuses[n])
		_, _ = w.Write([]byte(`{"code":"boom"}`))
	}))
	t.Cleanup(srv.Close)

	return srv, &calls
}

func fastRetry() *RetryConfig {
	return &RetryConfig{MaxRetries: 2, WaitTime: time.Millisecond, MaxWaitTime: 5 * time.Millisecond}
}

// TestResponseTravelsWithTheError is the fix for the wrapper that folded the
// status into a string and discarded the response.
func TestResponseTravelsWithTheError(t *testing.T) {
	srv, _ := statusServer(t, http.StatusConflict)

	c := NewClient(context.Background(), Config{BaseURL: srv.URL}, &testutil.MockLogger{})
	resp, err := c.Get(context.Background(), "/x", nil)

	require.Error(t, err)
	require.NotNil(t, resp, "the response must survive the error")
	assert.Equal(t, http.StatusConflict, resp.StatusCode())

	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(resp.Body(), &body))
	assert.Equal(t, "boom", body.Code)
}

// TestBuilderIsExposed: the point of building on resty is getting its builder,
// not reimplementing a smaller one.
func TestBuilderIsExposed(t *testing.T) {
	var gotPath, gotQuery, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("page")
		gotHeader = r.Header.Get("X-Trace")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(context.Background(), Config{BaseURL: srv.URL}, &testutil.MockLogger{})

	resp, err := c.R(context.Background()).
		SetPathParam("id", "42").
		SetQueryParam("page", "2").
		SetHeader("X-Trace", "abc").
		Get("/users/{id}/orders")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, "/users/42/orders", gotPath)
	assert.Equal(t, "2", gotQuery)
	assert.Equal(t, "abc", gotHeader)
}

// TestNoRetryBlockMeansNoRetries: resilience is composed in, not always present
// behind a flag.
func TestNoRetryBlockMeansNoRetries(t *testing.T) {
	srv, calls := statusServer(t, http.StatusServiceUnavailable)

	c := NewClient(context.Background(), Config{BaseURL: srv.URL}, &testutil.MockLogger{})
	_, err := c.Get(context.Background(), "/x", nil)

	require.Error(t, err)
	assert.Equal(t, int64(1), calls.Load())
}

func TestRetryReplaysIdempotentOnTransientStatus(t *testing.T) {
	srv, calls := statusServer(t, http.StatusServiceUnavailable)

	c := NewClient(context.Background(), Config{BaseURL: srv.URL, Retry: fastRetry()}, &testutil.MockLogger{})
	_, err := c.Get(context.Background(), "/x", nil)

	require.Error(t, err)
	assert.Equal(t, int64(3), calls.Load(), "one attempt plus two retries")
}

// TestRetryRefusesNonIdempotent is the safety rule: replaying a POST can
// duplicate a side effect the server already applied.
func TestRetryRefusesNonIdempotent(t *testing.T) {
	srv, calls := statusServer(t, http.StatusServiceUnavailable)

	c := NewClient(context.Background(), Config{BaseURL: srv.URL, Retry: fastRetry()}, &testutil.MockLogger{})
	_, err := c.Post(context.Background(), "/x", map[string]int{"a": 1}, nil)

	require.Error(t, err)
	assert.Equal(t, int64(1), calls.Load(), "a POST must be attempted exactly once")
}

func TestRetryNonIdempotentOptIn(t *testing.T) {
	srv, calls := statusServer(t, http.StatusServiceUnavailable)

	cfg := fastRetry()
	cfg.RetryNonIdempotent = true
	c := NewClient(context.Background(), Config{BaseURL: srv.URL, Retry: cfg}, &testutil.MockLogger{})

	_, err := c.Post(context.Background(), "/x", map[string]int{"a": 1}, nil)
	require.Error(t, err)
	assert.Equal(t, int64(3), calls.Load())
}

// TestRetryIgnoresPermanentStatuses: retrying a 400 only repeats the same
// failure later while adding load.
func TestRetryIgnoresPermanentStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusUnprocessableEntity,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, calls := statusServer(t, status)

			c := NewClient(context.Background(), Config{BaseURL: srv.URL, Retry: fastRetry()}, &testutil.MockLogger{})
			_, err := c.Get(context.Background(), "/x", nil)

			require.Error(t, err)
			assert.Equal(t, int64(1), calls.Load())
		})
	}
}

func TestRetryStopsAtFirstSuccess(t *testing.T) {
	srv, calls := statusServer(t, http.StatusServiceUnavailable, http.StatusOK)

	c := NewClient(context.Background(), Config{BaseURL: srv.URL, Retry: fastRetry()}, &testutil.MockLogger{})
	resp, err := c.Get(context.Background(), "/x", nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, int64(2), calls.Load())
}

// TestBreakerOpensAndShortCircuitsRetries is the composition guarantee: the
// breaker sees each attempt, so it opens on the real failure count and ends the
// retry loop instead of letting it keep hammering a downed service.
func TestBreakerOpensAndShortCircuitsRetries(t *testing.T) {
	srv, calls := statusServer(t, http.StatusServiceUnavailable)

	c := NewClient(context.Background(), Config{
		BaseURL: srv.URL,
		Retry:   &RetryConfig{MaxRetries: 10, WaitTime: time.Millisecond, MaxWaitTime: 2 * time.Millisecond},
		CircuitBreaker: &circuit_breaker.Config{
			Name:             "test",
			MaxRequests:      1,
			Interval:         time.Minute,
			Timeout:          time.Minute,
			RequestThreshold: 3,
		},
	}, &testutil.MockLogger{})

	_, err := c.Get(context.Background(), "/x", nil)
	require.Error(t, err)

	got := calls.Load()
	t.Logf("upstream received %d of the 11 configured attempts", got)

	assert.LessOrEqual(t, got, int64(4),
		"with a threshold of 3 the breaker must open and end the retry loop, "+
			"not let all 11 attempts through")
	assert.Greater(t, got, int64(0), "the first attempts must reach the upstream")
}

// TestBreakerIgnoresClientErrors: a 404 means the dependency is up and
// answering; counting it would open the circuit over our own bad request.
func TestBreakerIgnoresClientErrors(t *testing.T) {
	srv, calls := statusServer(t, http.StatusNotFound)

	c := NewClient(context.Background(), Config{
		BaseURL: srv.URL,
		CircuitBreaker: &circuit_breaker.Config{
			Name:             "test-404",
			RequestThreshold: 2,
			Interval:         time.Minute,
			Timeout:          time.Minute,
		},
	}, &testutil.MockLogger{})

	for i := 0; i < 6; i++ {
		_, err := c.Get(context.Background(), "/x", nil)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrCircuitOpen)
	}
	assert.Equal(t, int64(6), calls.Load(), "every request must have reached the upstream")
}

func TestRetryAfterHeaderIsHonoured(t *testing.T) {
	cases := map[string]struct {
		header  string
		want    time.Duration
		atLeast bool
	}{
		"seconds":      {header: "2", want: 2 * time.Second},
		"absent":       {header: "", want: 0},
		"malformed":    {header: "soon", want: 0},
		"non positive": {header: "0", want: 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			t.Cleanup(srv.Close)

			c := NewClient(context.Background(), Config{BaseURL: srv.URL}, &testutil.MockLogger{})
			r, _ := c.Get(context.Background(), "/x", nil)
			require.NotNil(t, r)

			got, err := RetryAfter(nil, r)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsIdempotentAndRetryableStatus(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		assert.True(t, IsIdempotent(m), m)
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch} {
		assert.False(t, IsIdempotent(m), m)
	}

	assert.True(t, RetryableStatus(http.StatusTooManyRequests))
	assert.True(t, RetryableStatus(http.StatusServiceUnavailable))
	assert.False(t, RetryableStatus(http.StatusBadRequest))
	assert.False(t, RetryableStatus(http.StatusNotImplemented))
}

// TestBreakerTransportHonoursTheRoundTripperContract is the regression test for
// the transport returning a nil response with a nil error.
//
// It happened because circuit_breaker.Execute discards the result whenever the
// operation reports an error, and this transport reports one on purpose for
// statuses that must count against the breaker. net/http rejects that pair, and
// the failure was invisible while nothing exercised the logging path.
func TestBreakerTransportHonoursTheRoundTripperContract(t *testing.T) {
	for _, status := range []int{
		http.StatusOK,
		http.StatusNotFound,           // not counted by the breaker
		http.StatusServiceUnavailable, // counted by the breaker
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, _ := statusServer(t, status)

			transport := newBreakerTransport(nil, &circuit_breaker.Config{
				Name:             "contract",
				RequestThreshold: 100,
				Interval:         time.Minute,
				Timeout:          time.Minute,
			}, &testutil.MockLogger{})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/x", nil)
			require.NoError(t, err)

			resp, err := transport.RoundTrip(req)

			// The contract: never a nil response together with a nil error.
			if err == nil {
				require.NotNil(t, resp, "a nil response with a nil error violates http.RoundTripper")
				assert.Equal(t, status, resp.StatusCode)
				_ = resp.Body.Close()
			}
		})
	}
}

// TestBreakerTransportReportsAnOpenCircuit covers the other branch: once open,
// the transport must return an error rather than a nil response.
func TestBreakerTransportReportsAnOpenCircuit(t *testing.T) {
	srv, _ := statusServer(t, http.StatusServiceUnavailable)

	transport := newBreakerTransport(nil, &circuit_breaker.Config{
		Name:             "open",
		RequestThreshold: 2,
		Interval:         time.Minute,
		Timeout:          time.Minute,
	}, &testutil.MockLogger{})

	var lastErr error
	for i := 0; i < 6; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/x", nil)
		require.NoError(t, err)

		resp, rtErr := transport.RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if rtErr != nil {
			lastErr = rtErr
			require.Nil(t, resp, "an error must come with no response")
		}
	}

	assert.ErrorIs(t, lastErr, ErrCircuitOpen)
}

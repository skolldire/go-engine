package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ── mocks ─────────────────────────────────────────────────────────────────────

type mockLogger struct{ mock.Mock }

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]interface{}) {
	m.Called(ctx, err, fields)
}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]interface{}) {}
func (m *mockLogger) WrapError(err error, msg string) error                                    { return err }
func (m *mockLogger) WithField(key string, value interface{}) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                                      { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                           { return nil }

// mockChecker implements Checker.
type mockChecker struct{ mock.Mock }

func (m *mockChecker) Check(ctx context.Context) error {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return nil
}

// mockService implements Service.
type mockService struct{ mock.Mock }

func (m *mockService) IsLive() bool {
	return m.Called().Bool(0)
}
func (m *mockService) IsReady() bool {
	return m.Called().Bool(0)
}
func (m *mockService) GetStatus() HealthStatus {
	return m.Called().Get(0).(HealthStatus)
}

// mockRedisPinger implements redisPinger.
type mockRedisPinger struct{ mock.Mock }

func (m *mockRedisPinger) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return nil
}

// mockSQLPinger implements sqlPinger for testing.
type mockSQLDBer struct{ err error }

func (m *mockSQLDBer) Ping(_ context.Context) error { return m.err }

// ── helpers ───────────────────────────────────────────────────────────────────

func newSvc(log logger.Service, checkers ...namedChecker) *HealthService {
	svc := NewService(Config{Timeout: 2 * time.Second}, log)
	for _, nc := range checkers {
		svc.Register(nc.name, nc.checker)
	}
	return svc
}

func upChecker() *mockChecker {
	m := &mockChecker{}
	m.On("Check", mock.Anything).Return(nil)
	return m
}

func downChecker(msg string) *mockChecker {
	m := &mockChecker{}
	m.On("Check", mock.Anything).Return(errors.New(msg))
	return m
}

// ── NewService ────────────────────────────────────────────────────────────────

func TestNewService_DefaultTimeout(t *testing.T) {
	svc := NewService(Config{}, &mockLogger{})
	assert.Equal(t, DefaultTimeout, svc.cfg.Timeout)
}

func TestNewService_CustomTimeout(t *testing.T) {
	svc := NewService(Config{Timeout: 10 * time.Second}, &mockLogger{})
	assert.Equal(t, 10*time.Second, svc.cfg.Timeout)
}

func TestNewService_EmptyCheckers(t *testing.T) {
	svc := NewService(Config{}, &mockLogger{})
	assert.Empty(t, svc.checkers)
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestHealthService_Register_Chaining(t *testing.T) {
	svc := NewService(Config{}, &mockLogger{})
	result := svc.Register("a", upChecker()).Register("b", upChecker())
	assert.Same(t, svc, result)
	assert.Len(t, svc.checkers, 2)
	assert.Equal(t, "a", svc.checkers[0].name)
	assert.Equal(t, "b", svc.checkers[1].name)
}

func TestHealthService_Register_LogsWhenEnabled(t *testing.T) {
	log := &mockLogger{}
	log.On("Debug", mock.Anything, "health checker registered",
		map[string]interface{}{"checker": "redis"}).Return()

	svc := NewService(Config{EnableLogging: true}, log)
	svc.Register("redis", upChecker())

	log.AssertExpectations(t)
}

// ── IsLive ────────────────────────────────────────────────────────────────────

func TestHealthService_IsLive(t *testing.T) {
	svc := newSvc(&mockLogger{})
	assert.True(t, svc.IsLive())
}

// ── IsReady ───────────────────────────────────────────────────────────────────

func TestHealthService_IsReady_NoCheckers(t *testing.T) {
	svc := newSvc(&mockLogger{})
	assert.True(t, svc.IsReady())
}

func TestHealthService_IsReady_AllUp(t *testing.T) {
	svc := newSvc(&mockLogger{},
		namedChecker{"db", upChecker()},
		namedChecker{"cache", upChecker()},
	)
	assert.True(t, svc.IsReady())
}

func TestHealthService_IsReady_SomeDown(t *testing.T) {
	svc := newSvc(&mockLogger{},
		namedChecker{"db", upChecker()},
		namedChecker{"cache", downChecker("connection refused")},
	)
	assert.False(t, svc.IsReady())
}

func TestHealthService_IsReady_AllDown(t *testing.T) {
	svc := newSvc(&mockLogger{},
		namedChecker{"db", downChecker("timeout")},
	)
	assert.False(t, svc.IsReady())
}

// ── GetStatus ─────────────────────────────────────────────────────────────────

func TestHealthService_GetStatus_NoCheckers(t *testing.T) {
	svc := newSvc(&mockLogger{})
	status := svc.GetStatus()

	assert.Equal(t, StatusUp, status.Status)
	assert.Empty(t, status.Dependencies)
	assert.WithinDuration(t, time.Now(), status.Timestamp, 2*time.Second)
}

func TestHealthService_GetStatus_AllUp(t *testing.T) {
	svc := newSvc(&mockLogger{},
		namedChecker{"postgres", upChecker()},
		namedChecker{"redis", upChecker()},
	)
	status := svc.GetStatus()

	assert.Equal(t, StatusUp, status.Status)
	assert.Len(t, status.Dependencies, 2)
	for _, d := range status.Dependencies {
		assert.Equal(t, StatusUp, d.Status)
		assert.Empty(t, d.Error)
	}
}

func TestHealthService_GetStatus_SomeDown(t *testing.T) {
	svc := newSvc(&mockLogger{},
		namedChecker{"postgres", upChecker()},
		namedChecker{"redis", downChecker("connection refused")},
	)
	status := svc.GetStatus()

	assert.Equal(t, StatusDown, status.Status)
	assert.Len(t, status.Dependencies, 2)

	byName := map[string]DependencyStatus{}
	for _, d := range status.Dependencies {
		byName[d.Name] = d
	}
	assert.Equal(t, StatusUp, byName["postgres"].Status)
	assert.Equal(t, StatusDown, byName["redis"].Status)
	assert.Contains(t, byName["redis"].Error, "connection refused")
}

func TestHealthService_GetStatus_IncludesLatency(t *testing.T) {
	slow := &mockChecker{}
	slow.On("Check", mock.Anything).Run(func(args mock.Arguments) {
		time.Sleep(10 * time.Millisecond)
	}).Return(nil)

	svc := newSvc(&mockLogger{}, namedChecker{"slow-dep", slow})
	status := svc.GetStatus()

	assert.Equal(t, StatusUp, status.Status)
	assert.GreaterOrEqual(t, status.Dependencies[0].LatencyMs, int64(10))
}

func TestHealthService_GetStatus_OrderPreserved(t *testing.T) {
	names := []string{"a", "b", "c", "d"}
	checkers := make([]namedChecker, len(names))
	for i, n := range names {
		checkers[i] = namedChecker{n, upChecker()}
	}
	svc := newSvc(&mockLogger{}, checkers...)
	status := svc.GetStatus()

	for i, d := range status.Dependencies {
		assert.Equal(t, names[i], d.Name)
	}
}

func TestHealthService_GetStatus_ConcurrentCheckers(t *testing.T) {
	// Two checkers each sleeping 50ms: total should finish well under 200ms
	slow := func() *mockChecker {
		m := &mockChecker{}
		m.On("Check", mock.Anything).Run(func(_ mock.Arguments) {
			time.Sleep(50 * time.Millisecond)
		}).Return(nil)
		return m
	}

	svc := newSvc(&mockLogger{},
		namedChecker{"a", slow()},
		namedChecker{"b", slow()},
	)

	start := time.Now()
	svc.GetStatus()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 150*time.Millisecond,
		"checkers should run concurrently, not sequentially")
}

// ── HTTPHandler ───────────────────────────────────────────────────────────────

func TestHTTPHandler_Live_200(t *testing.T) {
	svc := &mockService{}
	svc.On("IsLive").Return(true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHTTPHandler_Live_503(t *testing.T) {
	svc := &mockService{}
	svc.On("IsLive").Return(false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHTTPHandler_Ready_200(t *testing.T) {
	svc := &mockService{}
	svc.On("IsReady").Return(true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHTTPHandler_Ready_503(t *testing.T) {
	svc := &mockService{}
	svc.On("IsReady").Return(false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHTTPHandler_Deps_200_AllUp(t *testing.T) {
	hs := HealthStatus{
		Status:    StatusUp,
		Timestamp: time.Now(),
		Dependencies: []DependencyStatus{
			{Name: "db", Status: StatusUp, LatencyMs: 3},
		},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/deps", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"status":"up"`)
	assert.Contains(t, rec.Body.String(), `"name":"db"`)
}

func TestHTTPHandler_Deps_503_SomeDown(t *testing.T) {
	hs := HealthStatus{
		Status:    StatusDown,
		Timestamp: time.Now(),
		Dependencies: []DependencyStatus{
			{Name: "cache", Status: StatusDown, Error: "connection refused"},
		},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/deps", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"down"`)
	assert.Contains(t, rec.Body.String(), "connection refused")
}

func TestHTTPHandler_Deps_JSON_Structure(t *testing.T) {
	hs := HealthStatus{
		Status:    StatusUp,
		Timestamp: time.Now(),
		Dependencies: []DependencyStatus{
			{Name: "db", Status: StatusUp, LatencyMs: 5},
		},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/deps", nil)
	NewHTTPHandler(svc).Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	assert.True(t, strings.HasPrefix(strings.TrimSpace(body), "{"),
		"response should be a JSON object")
	assert.Contains(t, body, `"dependencies"`)
	assert.Contains(t, body, `"timestamp"`)
	assert.Contains(t, body, `"latency_ms"`)
}

// ── RedisChecker ──────────────────────────────────────────────────────────────

func TestRedisChecker_Success(t *testing.T) {
	p := &mockRedisPinger{}
	p.On("Ping", mock.Anything).Return(nil)

	err := NewRedisChecker(p).Check(context.Background())
	assert.NoError(t, err)
	p.AssertExpectations(t)
}

func TestRedisChecker_Error(t *testing.T) {
	p := &mockRedisPinger{}
	p.On("Ping", mock.Anything).Return(errors.New("dial tcp: connection refused"))

	err := NewRedisChecker(p).Check(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis ping")
}

// ── SQLChecker ────────────────────────────────────────────────────────────────

func TestSQLChecker_Success(t *testing.T) {
	checker := NewSQLChecker(&mockSQLDBer{err: nil})
	assert.NoError(t, checker.Check(context.Background()))
}

func TestSQLChecker_Error_ClosedConnection(t *testing.T) {
	checker := NewSQLChecker(&mockSQLDBer{err: errors.New("connection closed")})
	err := checker.Check(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sql ping")
}

// ── HTTPChecker ───────────────────────────────────────────────────────────────

func TestHTTPChecker_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := NewHTTPChecker(srv.URL, 2*time.Second).Check(context.Background())
	assert.NoError(t, err)
}

func TestHTTPChecker_ServerError_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewHTTPChecker(srv.URL, 2*time.Second).Check(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPChecker_ClientError_4xx_IsOK(t *testing.T) {
	// 4xx is not treated as a health failure (the server is up, just unauthorized)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := NewHTTPChecker(srv.URL, 2*time.Second).Check(context.Background())
	assert.NoError(t, err)
}

func TestHTTPChecker_NetworkError(t *testing.T) {
	err := NewHTTPChecker("http://localhost:1", 500*time.Millisecond).Check(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "http get")
}

func TestHTTPChecker_DefaultTimeout(t *testing.T) {
	c := NewHTTPChecker("http://example.com", 0)
	assert.Equal(t, DefaultTimeout, c.client.Timeout)
}

func TestHTTPChecker_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := NewHTTPChecker(srv.URL, 2*time.Second).Check(ctx)
	assert.Error(t, err)
}

// ── HealthHandler ─────────────────────────────────────────────────────────────

func TestHealthHandler_200_AllHealthy(t *testing.T) {
	hs := HealthStatus{
		Status:    StatusUp,
		Timestamp: time.Now(),
		Dependencies: []DependencyStatus{
			{Name: "db", Status: StatusUp, LatencyMs: 3},
			{Name: "cache", Status: StatusUp, LatencyMs: 1},
		},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	NewHTTPHandler(svc).HealthHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	assert.Contains(t, body, `"status":"healthy"`)
	assert.Contains(t, body, `"checks"`)
	assert.Contains(t, body, `"latency_ms"`)
	assert.Contains(t, body, `"name":"db"`)
	assert.Contains(t, body, `"status":"ok"`)
}

func TestHealthHandler_503_SomeUnhealthy(t *testing.T) {
	hs := HealthStatus{
		Status:    StatusDown,
		Timestamp: time.Now(),
		Dependencies: []DependencyStatus{
			{Name: "db", Status: StatusUp, LatencyMs: 2},
			{Name: "cache", Status: StatusDown, Error: "connection refused", LatencyMs: 5001},
		},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	NewHTTPHandler(svc).HealthHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"status":"unhealthy"`)
	assert.Contains(t, body, `"name":"cache"`)
	assert.Contains(t, body, `"status":"error"`)
	assert.Contains(t, body, "connection refused")
}

func TestHealthHandler_NoCheckers_200(t *testing.T) {
	hs := HealthStatus{
		Status:       StatusUp,
		Timestamp:    time.Now(),
		Dependencies: []DependencyStatus{},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	NewHTTPHandler(svc).HealthHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"healthy"`)
	assert.Contains(t, rec.Body.String(), `"checks":[]`)
}

func TestHealthHandler_CheckResult_MapsCorrectly(t *testing.T) {
	hs := HealthStatus{
		Status:    StatusDown,
		Timestamp: time.Now(),
		Dependencies: []DependencyStatus{
			{Name: "postgres", Status: StatusUp, LatencyMs: 4},
			{Name: "redis", Status: StatusDown, Error: "timeout", LatencyMs: 5000},
		},
	}
	svc := &mockService{}
	svc.On("GetStatus").Return(hs)

	rec := httptest.NewRecorder()
	NewHTTPHandler(svc).HealthHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var resp HealthResponse
	assert.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, HealthStatusUnhealthy, resp.Status)
	assert.Len(t, resp.Checks, 2)
	assert.Equal(t, CheckStatusOK, resp.Checks[0].Status)
	assert.Empty(t, resp.Checks[0].Error)
	assert.Equal(t, CheckStatusError, resp.Checks[1].Status)
	assert.Equal(t, "timeout", resp.Checks[1].Error)
}

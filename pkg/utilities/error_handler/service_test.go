package error_handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockLogger is a mock implementation of logger.Service
type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]any) {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)  {}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields map[string]any) {
	m.Called(ctx, msg, fields)
}
func (m *mockLogger) Error(ctx context.Context, err error, fields map[string]any) {
	m.Called(ctx, err, fields)
}
func (m *mockLogger) FatalError(ctx context.Context, err error, fields map[string]any) {}
func (m *mockLogger) WrapError(err error, msg string) error                            { return err }
func (m *mockLogger) WithField(key string, value any) logger.Service                   { return m }
func (m *mockLogger) WithFields(fields map[string]any) logger.Service                  { return m }
func (m *mockLogger) GetLogLevel() string                                              { return "info" }
func (m *mockLogger) SetLogLevel(level string) error                                   { return nil }

func TestCommonApiError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CommonApiError
		expected string
	}{
		{
			name: "with cause",
			err: &CommonApiError{
				Code: CodeBadRequest,
				Msg:  "test message",
				Err:  errors.New("underlying error"),
			},
			expected: "Error ER-400: test message",
		},
		{
			name: "without cause",
			err: &CommonApiError{
				Code: CodeBadRequest,
				Msg:  "test message",
			},
			expected: "Error ER-400: test message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), tt.expected)
		})
	}
}

func TestCommonApiError_Unwrap(t *testing.T) {
	underlyingErr := errors.New("underlying")
	err := &CommonApiError{
		Code: CodeBadRequest,
		Msg:  "test",
		Err:  underlyingErr,
	}
	assert.Equal(t, underlyingErr, err.Unwrap())
}

func TestCommonApiError_WithRequestID(t *testing.T) {
	err := &CommonApiError{Code: CodeBadRequest, Msg: "test"}
	result := err.WithRequestID("req-123")

	assert.Equal(t, "req-123", result.RequestID)
	assert.NotSame(t, err, result, "the builder must return a copy")
	assert.Empty(t, err.RequestID, "the receiver must be left untouched")
}

func TestCommonApiError_WithDetail(t *testing.T) {
	err := &CommonApiError{Code: CodeBadRequest, Msg: "test"}
	result := err.WithDetail("key", "value")

	assert.Equal(t, "value", result.Details["key"])
	assert.NotSame(t, err, result, "the builder must return a copy")
	assert.Empty(t, err.Details, "the receiver must be left untouched")
}

// TestCommonApiError_SentinelIsNotCorrupted is the regression test for the
// builders mutating the receiver: a package-level sentinel decorated by one
// request stayed decorated for every request afterwards.
func TestCommonApiError_SentinelIsNotCorrupted(t *testing.T) {
	sentinel := NewNotFoundError("user not found", nil)

	first := sentinel.WithRequestID("req-1").WithDetail("user_id", "1")
	second := sentinel.WithRequestID("req-2").WithDetail("user_id", "2")

	assert.Equal(t, "req-1", first.RequestID)
	assert.Equal(t, "1", first.Details["user_id"])
	assert.Equal(t, "req-2", second.RequestID)
	assert.Equal(t, "2", second.Details["user_id"])

	assert.Empty(t, sentinel.RequestID, "the shared sentinel must never be decorated")
	assert.Empty(t, sentinel.Details, "the shared sentinel must never be decorated")
}

// TestCommonApiError_Is verifies the taxonomy works with errors.Is, which was
// impossible before because the type implemented no Is method.
func TestCommonApiError_Is(t *testing.T) {
	notFound := NewNotFoundError("user not found", nil)
	other := NewNotFoundError("order not found", errors.New("db"))
	badRequest := NewBadRequestError("bad", nil)

	assert.True(t, errors.Is(other, notFound), "same code must match")
	assert.False(t, errors.Is(badRequest, notFound), "different codes must not match")

	wrapped := fmt.Errorf("handling request: %w", other)
	assert.True(t, errors.Is(wrapped, notFound), "matching must survive wrapping")
}

// TestWrapError_DoesNotMutate guards the same immutability contract on the
// package-level WrapError helper.
func TestWrapError_DoesNotMutate(t *testing.T) {
	sentinel := NewInternalError("boom", errors.New("cause"))

	wrapped := WrapError(sentinel, "while saving")

	assert.Equal(t, "boom", sentinel.Msg, "the original must keep its message")
	var typed *CommonApiError
	require.True(t, errors.As(wrapped, &typed))
	assert.Equal(t, "while saving: boom", typed.Msg)
}

func TestNewCommonApiError(t *testing.T) {
	err := errors.New("test")
	apiErr := NewCommonApiError(CodeBadRequest, "message", err, http.StatusBadRequest)
	assert.Equal(t, CodeBadRequest, apiErr.Code)
	assert.Equal(t, "message", apiErr.Msg)
	assert.Equal(t, err, apiErr.Err)
	assert.Equal(t, http.StatusBadRequest, apiErr.HttpCode)
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		msg      string
		wantWrap bool
	}{
		{
			name:     "CommonApiError wrapped",
			err:      NewCommonApiError(CodeBadRequest, "original", nil, http.StatusBadRequest),
			msg:      "context",
			wantWrap: true,
		},
		{
			name:     "generic error not wrapped",
			err:      errors.New("generic"),
			msg:      "context",
			wantWrap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapError(tt.err, tt.msg)
			if tt.wantWrap {
				assert.Contains(t, result.Error(), tt.msg)
			} else {
				assert.Equal(t, tt.err, result)
			}
		})
	}
}

func TestNewBadRequestError(t *testing.T) {
	err := NewBadRequestError("bad request", nil)
	assert.Equal(t, CodeBadRequest, err.Code)
	assert.Equal(t, http.StatusBadRequest, err.HttpCode)
}

func TestNewUnauthorizedError(t *testing.T) {
	err := NewUnauthorizedError("unauthorized", nil)
	assert.Equal(t, CodeUnauthorized, err.Code)
	assert.Equal(t, http.StatusUnauthorized, err.HttpCode)
}

func TestNewForbiddenError(t *testing.T) {
	err := NewForbiddenError("forbidden", nil)
	assert.Equal(t, CodeForbidden, err.Code)
	assert.Equal(t, http.StatusForbidden, err.HttpCode)
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("not found", nil)
	assert.Equal(t, CodeNotFound, err.Code)
	assert.Equal(t, http.StatusNotFound, err.HttpCode)
}

func TestNewConflictError(t *testing.T) {
	err := NewConflictError("conflict", nil)
	assert.Equal(t, CodeConflict, err.Code)
	assert.Equal(t, http.StatusConflict, err.HttpCode)
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("validation failed", nil)
	assert.Equal(t, CodeValidationFailed, err.Code)
	assert.Equal(t, http.StatusUnprocessableEntity, err.HttpCode)
}

func TestNewInternalError(t *testing.T) {
	err := NewInternalError("internal error", nil)
	assert.Equal(t, CodeInternalError, err.Code)
	assert.Equal(t, http.StatusInternalServerError, err.HttpCode)
}

func TestHandleApiErrorResponse(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		logger         logger.Service
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "CommonApiError",
			err:            NewBadRequestError("bad request", nil),
			logger:         nil,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   CodeBadRequest,
		},
		{
			name:           "generic error",
			err:            errors.New("generic"),
			logger:         nil,
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   CodeInternalError,
		},
		{
			name: "with logger",
			err:  NewBadRequestError("bad request", errors.New("underlying")),
			logger: func() logger.Service {
				m := &mockLogger{}
				m.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()
				return m
			}(),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   CodeBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			err := HandleApiErrorResponse(tt.err, w, tt.logger)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, w.Code)

			var result CommonApiError
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			assert.Equal(t, tt.expectedCode, result.Code)
		})
	}
}

func TestHandleApiErrorResponseWithRequest(t *testing.T) {
	err := NewBadRequestError("bad request", nil)
	w := httptest.NewRecorder()
	requestID := "req-123"

	handleErr := HandleApiErrorResponseWithRequest(err, w, requestID, nil)
	assert.NoError(t, handleErr)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result CommonApiError
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	assert.Equal(t, CodeBadRequest, result.Code)
	assert.Equal(t, requestID, result.RequestID)
}

func TestHandleApiErrorResponseLegacy(t *testing.T) {
	err := NewBadRequestError("bad request", nil)
	w := httptest.NewRecorder()

	handleErr := HandleApiErrorResponseLegacy(err, w)
	assert.NoError(t, handleErr)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleApiErrorResponseWithRequestLegacy(t *testing.T) {
	err := NewBadRequestError("bad request", nil)
	w := httptest.NewRecorder()
	requestID := "req-123"

	handleErr := HandleApiErrorResponseWithRequestLegacy(err, w, requestID)
	assert.NoError(t, handleErr)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

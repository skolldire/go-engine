package error_handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockLogger is a mock implementation of logger.Service
type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {}
func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]interface{})  {}
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

func TestCommonApiError_WithContext(t *testing.T) {
	err := &CommonApiError{Code: CodeBadRequest, Msg: "test"}
	ctx := context.Background()
	result := err.WithContext(ctx)
	assert.Equal(t, ctx, result.Context)
	assert.Equal(t, err, result)
}

func TestCommonApiError_WithRequestID(t *testing.T) {
	err := &CommonApiError{Code: CodeBadRequest, Msg: "test"}
	result := err.WithRequestID("req-123")
	assert.Equal(t, "req-123", result.RequestID)
	assert.Equal(t, err, result)
}

func TestCommonApiError_WithDetail(t *testing.T) {
	err := &CommonApiError{Code: CodeBadRequest, Msg: "test"}
	result := err.WithDetail("key", "value")
	assert.Equal(t, "value", result.Details["key"])
	assert.Equal(t, err, result)
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

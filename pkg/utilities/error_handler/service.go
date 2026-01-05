package error_handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

const (
	CodeBadRequest       = "ER-400"
	CodeUnauthorized     = "ER-401"
	CodeForbidden        = "ER-403"
	CodeNotFound         = "ER-404"
	CodeConflict         = "ER-409"
	CodeValidationFailed = "ER-422"
	CodeInternalError    = "ER-500"
)

type CommonApiError struct {
	Code      string            `json:"code"`
	Msg       string            `json:"msg"`
	RequestID string            `json:"request_id,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
	Err       error             `json:"-"`
	HttpCode  int               `json:"-"`
	Context   context.Context   `json:"-"`
}

var _ error = (*CommonApiError)(nil)

func (e *CommonApiError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("Error %s: %s \ntrace: %s", e.Code, e.Msg, e.Err.Error())
	}
	return fmt.Sprintf("Error %s: %s", e.Code, e.Msg)
}

func (e *CommonApiError) Unwrap() error {
	return e.Err
}

func (e *CommonApiError) WithContext(ctx context.Context) *CommonApiError {
	e.Context = ctx
	return e
}

func (e *CommonApiError) WithRequestID(requestID string) *CommonApiError {
	e.RequestID = requestID
	return e
}

func (e *CommonApiError) WithDetail(key, value string) *CommonApiError {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}

func NewCommonApiError(code, msg string, err error, httpCode int) *CommonApiError {
	return &CommonApiError{
		Code:     code,
		Msg:      msg,
		Err:      err,
		HttpCode: httpCode,
	}
}

func WrapError(err error, msg string) error {
	var e *CommonApiError
	if errors.As(err, &e) {
		e.Msg = fmt.Sprintf("%s: %s", msg, e.Msg)
		return e
	}
	return err
}

func NewBadRequestError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeBadRequest, msg, err, http.StatusBadRequest)
}

func NewUnauthorizedError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeUnauthorized, msg, err, http.StatusUnauthorized)
}

func NewForbiddenError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeForbidden, msg, err, http.StatusForbidden)
}

func NewNotFoundError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeNotFound, msg, err, http.StatusNotFound)
}

func NewConflictError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeConflict, msg, err, http.StatusConflict)
}

func NewValidationError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeValidationFailed, msg, err, http.StatusUnprocessableEntity)
}

// NewInternalError creates a CommonApiError representing an internal server error with the standard internal error code and HTTP 500 status.
func NewInternalError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeInternalError, msg, err, http.StatusInternalServerError)
}

// HandleApiErrorResponse handles API errors and writes JSON response
// HandleApiErrorResponse writes an API-style JSON error response to w and logs the error using the provided logger when present.
// 
// If err is a *CommonApiError, the function uses err.Context if set (otherwise context.Background()), logs either a warning when
// the CommonApiError has no underlying Err or an error with structured fields (including request_id when present), writes the
// HTTP status from err.HttpCode and the JSON representation of the CommonApiError to the response. For non-*CommonApiError values,
// it logs an unhandled error event (when a logger is provided) and writes HTTP 500 with a JSON body containing CodeInternalError and
// the message "Internal server error".
// 
// The function always returns nil.
func HandleApiErrorResponse(err error, w http.ResponseWriter, log logger.Service) error {
	w.Header().Set("Content-Type", "application/json")

	var errType *CommonApiError
	if errors.As(err, &errType) {
		ctx := context.Background()
		if errType.Context != nil {
			ctx = errType.Context
		}

		if errType.Err == nil {
			if log != nil {
				log.Warn(ctx, "CommonApiError has nil Err field", map[string]interface{}{
					"error_code": errType.Code,
					"error_msg":  errType.Msg,
					"http_code":  errType.HttpCode,
				})
			}
		} else {
			if log != nil {
				logFields := map[string]interface{}{
					"error_code": errType.Code,
					"error_msg":  errType.Msg,
					"http_code":  errType.HttpCode,
				}
				if errType.RequestID != "" {
					logFields["request_id"] = errType.RequestID
				}
				log.Error(ctx, errType.Err, logFields)
			}
		}

		w.WriteHeader(errType.HttpCode)
		b, _ := json.Marshal(errType)
		_, _ = w.Write(b)
		return nil
	}

	// Unhandled error - log it if logger is available
	if log != nil {
		log.Error(context.Background(), err, map[string]interface{}{
			"error_type": "unhandled_error",
		})
	}

	w.WriteHeader(http.StatusInternalServerError)
	b, _ := json.Marshal(CommonApiError{
		Code: CodeInternalError,
		Msg:  "Internal server error",
	})
	_, _ = w.Write(b)
	return nil
}

// HandleApiErrorResponseWithRequest handles API errors with request ID and writes JSON response
// JSON body containing CodeInternalError, Msg "Internal server error", and the provided requestID.
func HandleApiErrorResponseWithRequest(err error, w http.ResponseWriter, requestID string, log logger.Service) error {
	w.Header().Set("Content-Type", "application/json")

	var errType *CommonApiError
	if errors.As(err, &errType) {
		errType.RequestID = requestID
		ctx := context.Background()
		if errType.Context != nil {
			ctx = errType.Context
		}

		if errType.Err == nil {
			if log != nil {
				log.Warn(ctx, "CommonApiError has nil Err field", map[string]interface{}{
					"error_code": errType.Code,
					"error_msg":  errType.Msg,
					"http_code":  errType.HttpCode,
					"request_id": requestID,
				})
			}
		} else {
			if log != nil {
				log.Error(ctx, errType.Err, map[string]interface{}{
					"error_code": errType.Code,
					"error_msg":  errType.Msg,
					"http_code":  errType.HttpCode,
					"request_id": requestID,
				})
			}
		}

		w.WriteHeader(errType.HttpCode)
		b, _ := json.Marshal(errType)
		_, _ = w.Write(b)
		return nil
	}

	// Unhandled error - log it if logger is available
	if log != nil {
		log.Error(context.Background(), err, map[string]interface{}{
			"error_type": "unhandled_error",
			"request_id": requestID,
		})
	}

	w.WriteHeader(http.StatusInternalServerError)
	b, _ := json.Marshal(CommonApiError{
		Code:      CodeInternalError,
		Msg:       "Internal server error",
		RequestID: requestID,
	})
	_, _ = w.Write(b)
	return nil
}

// HandleApiErrorResponseLegacy is a legacy version that doesn't require logger
// HandleApiErrorResponseLegacy delegates to HandleApiErrorResponse passing a nil logger.
// Deprecated: Use HandleApiErrorResponse with a logger.Service parameter instead.
func HandleApiErrorResponseLegacy(err error, w http.ResponseWriter) error {
	return HandleApiErrorResponse(err, w, nil)
}

// HandleApiErrorResponseWithRequestLegacy is a legacy version that doesn't require logger
// HandleApiErrorResponseWithRequestLegacy delegates to HandleApiErrorResponseWithRequest using a nil logger.
// Deprecated: Use HandleApiErrorResponseWithRequest with a logger parameter instead.
func HandleApiErrorResponseWithRequestLegacy(err error, w http.ResponseWriter, requestID string) error {
	return HandleApiErrorResponseWithRequest(err, w, requestID, nil)
}
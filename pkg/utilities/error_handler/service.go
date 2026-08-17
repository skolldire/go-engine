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

// Is reports whether target is (or wraps) a *CommonApiError carrying the same
// Code. Without it errors.Is could not be used against this taxonomy at all,
// so callers had to type-assert and compare codes by hand.
func (e *CommonApiError) Is(target error) bool {
	var t *CommonApiError
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// WithRequestID returns a copy carrying requestID.
//
// These builders return a copy rather than mutating the receiver. Mutating was
// unsafe for the common `var ErrNotFound = NewNotFoundError(...)` package-level
// sentinel: the first request to decorate it corrupted the shared value for
// every subsequent one.
func (e *CommonApiError) WithRequestID(requestID string) *CommonApiError {
	clone := e.clone()
	clone.RequestID = requestID
	return clone
}

// WithDetail returns a copy with key=value added to Details.
func (e *CommonApiError) WithDetail(key, value string) *CommonApiError {
	clone := e.clone()
	if clone.Details == nil {
		clone.Details = make(map[string]string, 1)
	}
	clone.Details[key] = value
	return clone
}

// clone produces an independent copy, deep-copying Details so the copy and the
// original never share the same map.
func (e *CommonApiError) clone() *CommonApiError {
	copied := *e
	if e.Details != nil {
		copied.Details = make(map[string]string, len(e.Details))
		for k, v := range e.Details {
			copied.Details[k] = v
		}
	}
	return &copied
}

func NewCommonApiError(code, msg string, err error, httpCode int) *CommonApiError {
	return &CommonApiError{
		Code:     code,
		Msg:      msg,
		Err:      err,
		HttpCode: httpCode,
	}
}

// WrapError returns a copy of err with msg prefixed to its message when err is
// (or wraps) a *CommonApiError. It never mutates the error it was given.
func WrapError(err error, msg string) error {
	var e *CommonApiError
	if errors.As(err, &e) {
		wrapped := e.clone()
		wrapped.Msg = fmt.Sprintf("%s: %s", msg, e.Msg)
		return wrapped
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

func NewInternalError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeInternalError, msg, err, http.StatusInternalServerError)
}

// HandleApiErrorResponse handles API errors and writes JSON response
// If logger is provided, errors are logged using structured logging
func HandleApiErrorResponse(err error, w http.ResponseWriter, log logger.Service) error {
	return HandleApiErrorResponseCtx(context.Background(), err, w, "", log)
}

// HandleApiErrorResponseCtx is the context-aware form of the handlers above.
// The context is passed explicitly instead of being stashed inside the error
// value, which is the documented Go anti-pattern the old Context field was.
// requestID may be empty.
func HandleApiErrorResponseCtx(ctx context.Context, err error, w http.ResponseWriter, requestID string, log logger.Service) error {
	w.Header().Set("Content-Type", "application/json")

	if ctx == nil {
		ctx = context.Background()
	}

	var errType *CommonApiError
	if errors.As(err, &errType) {
		// Copy before decorating: the caller's error (often a package-level
		// sentinel) must not acquire this request's ID.
		payload := errType.clone()
		if requestID != "" {
			payload.RequestID = requestID
		}
		errType = payload

		if errType.Err == nil {
			if log != nil {
				log.Warn(ctx, "CommonApiError has nil Err field", map[string]any{
					"error_code": errType.Code,
					"error_msg":  errType.Msg,
					"http_code":  errType.HttpCode,
				})
			}
		} else {
			if log != nil {
				logFields := map[string]any{
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
		log.Error(context.Background(), err, map[string]any{
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

// HandleApiErrorResponseWithRequest handles API errors with request ID and
// writes the JSON response. Prefer HandleApiErrorResponseCtx, which also takes
// the context explicitly.
func HandleApiErrorResponseWithRequest(err error, w http.ResponseWriter, requestID string, log logger.Service) error {
	return HandleApiErrorResponseCtx(context.Background(), err, w, requestID, log)
}

// HandleApiErrorResponseLegacy is a legacy version that doesn't require logger
// Deprecated: Use HandleApiErrorResponse with logger parameter instead
func HandleApiErrorResponseLegacy(err error, w http.ResponseWriter) error {
	return HandleApiErrorResponse(err, w, nil)
}

// HandleApiErrorResponseWithRequestLegacy is a legacy version that doesn't require logger
// Deprecated: Use HandleApiErrorResponseWithRequest with logger parameter instead
func HandleApiErrorResponseWithRequestLegacy(err error, w http.ResponseWriter, requestID string) error {
	return HandleApiErrorResponseWithRequest(err, w, requestID, nil)
}

package error_handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

func NewInternalError(msg string, err error) *CommonApiError {
	return NewCommonApiError(CodeInternalError, msg, err, http.StatusInternalServerError)
}

func HandleApiErrorResponse(err error, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")

	var errType *CommonApiError
	if errors.As(err, &errType) {
		if errType.Err == nil {
			fmt.Printf("[error_wrapper] Advertencia: El campo Err es nulo en CommonApiError\n")
		} else {
			fmt.Printf("CommonApiError: %v\n", err)
		}
		w.WriteHeader(errType.HttpCode)
		b, _ := json.Marshal(errType)
		_, _ = w.Write(b)
		return nil
	}

	fmt.Printf("Error no manejado: %v\n", err)
	w.WriteHeader(http.StatusInternalServerError)
	b, _ := json.Marshal(CommonApiError{
		Code: CodeInternalError,
		Msg:  "Error interno del servidor",
	})
	_, _ = w.Write(b)
	return nil
}

func HandleApiErrorResponseWithRequest(err error, w http.ResponseWriter, requestID string) error {
	w.Header().Set("Content-Type", "application/json")

	var errType *CommonApiError
	if errors.As(err, &errType) {
		errType.RequestID = requestID
		if errType.Err == nil {
			fmt.Printf("[error_wrapper] Advertencia: El campo Err es nulo en CommonApiError\n")
		} else {
			fmt.Printf("CommonApiError: %v (RequestID: %s)\n", err, requestID)
		}
		w.WriteHeader(errType.HttpCode)
		b, _ := json.Marshal(errType)
		_, _ = w.Write(b)
		return nil
	}

	fmt.Printf("Error no manejado: %v (RequestID: %s)\n", err, requestID)
	w.WriteHeader(http.StatusInternalServerError)
	b, _ := json.Marshal(CommonApiError{
		Code:      CodeInternalError,
		Msg:       "Error interno del servidor",
		RequestID: requestID,
	})
	_, _ = w.Write(b)
	return nil
}

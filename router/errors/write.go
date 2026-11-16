package errors

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
)

func (r *Responder) Write(ctx context.Context, writer http.ResponseWriter, original error) error {
	if ctx == nil {
		return ErrNilContext
	}
	if writer == nil {
		return fmt.Errorf("%w: writer is nil", ErrInvalidCall)
	}
	value := reflect.ValueOf(writer)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return fmt.Errorf("%w: writer is nil", ErrInvalidCall)
	}
	if original == nil {
		return fmt.Errorf("%w: error is nil", ErrInvalidCall)
	}
	problem, problemErr := r.prepare(ctx, original)
	body, err := json.Marshal(problem)
	if err != nil {
		return stderrors.Join(problemErr, err)
	}
	header := writer.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(problem.Status)
	n, err := writer.Write(body)
	if err == nil && n < len(body) {
		err = io.ErrShortWrite
	}
	if problemErr != nil {
		return stderrors.Join(problemErr, err)
	}
	return err
}

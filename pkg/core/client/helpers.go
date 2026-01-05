package client

import (
	"fmt"
)

// If the value is not of type T, it returns the zero value of T and an error describing the expected and actual types.
func SafeTypeAssert[T any](result interface{}) (T, error) {
	var zero T
	val, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected response type: expected %T, got %T", zero, result)
	}
	return val, nil
}



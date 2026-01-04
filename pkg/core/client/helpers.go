package client

import (
	"fmt"
)

func SafeTypeAssert[T any](result interface{}) (T, error) {
	var zero T
	val, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected response type: expected %T, got %T", zero, result)
	}
	return val, nil
}




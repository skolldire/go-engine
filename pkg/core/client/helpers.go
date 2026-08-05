package client

import (
	"fmt"
)

// SafeTypeAssert converts result to type T without panicking.
// It returns an error if result cannot be asserted to T, making it safe to use
// on the any values returned by BaseClient.Execute.
//
// Example:
//
//	raw, err := bc.Execute(ctx, "fetch", func(ctx context.Context) (any, error) {
//	    return fetchUser(id)
//	})
//	if err != nil { return err }
//	user, err := client.SafeTypeAssert[*User](raw)
func SafeTypeAssert[T any](result any) (T, error) {
	var zero T
	val, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected response type: expected %T, got %T", zero, result)
	}
	return val, nil
}

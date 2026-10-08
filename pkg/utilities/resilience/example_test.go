package resilience_test

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

// ExampleService_Execute wraps an operation with retry + circuit breaker.
// A nil/partial Config is accepted; sensible defaults are applied.
func ExampleService_Execute() {
	svc := resilience.NewResilienceService(resilience.Config{}, nil)

	result, err := svc.Execute(context.Background(), func(context.Context) (any, error) {
		return "ok", nil
	})

	fmt.Println(result, err)
	// Output: ok <nil>
}

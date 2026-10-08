package health_test

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/pkg/health"
)

// alwaysUp is a trivial dependency check that always reports healthy.
type alwaysUp struct{}

func (alwaysUp) Check(context.Context) error { return nil }

// ExampleNewService shows how to register dependency checkers and read the
// aggregated health status.
func ExampleNewService() {
	svc := health.NewService(health.Config{}, nil)
	svc.Register("database", alwaysUp{})
	svc.Register("cache", alwaysUp{})

	status := svc.GetStatus(context.Background())

	fmt.Println(status.Status)
	fmt.Println(len(status.Dependencies))
	// Output:
	// up
	// 2
}

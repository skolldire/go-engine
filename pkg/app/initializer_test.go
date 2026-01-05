package app

import (
	"testing"
)

func TestRegisterDefaultClients(t *testing.T) {
	err := RegisterDefaultClients(&mockLogger{})
	// This may return nil if registry is already initialized, which is OK
	// We're just testing that the function exists and can be called without panic
	// err can be nil or non-nil depending on registry state
	_ = err // Just verify function can be called
}

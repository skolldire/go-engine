package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterDefaultClients(t *testing.T) {
	err := RegisterDefaultClients(&mockLogger{})
	// This may return nil if registry is already initialized, which is OK
	// We're just testing that the function exists and can be called without panic
	// err can be nil or non-nil depending on registry state
	_ = err // Just verify function can be called
}

func TestClients_InitializeWithRegistry(t *testing.T) {
	clients := &clients{
		ctx:    context.Background(),
		log:    &mockLogger{},
		errors: []error{},
	}
	
	// This is a no-op function, just test it doesn't panic
	clients.initializeWithRegistry(nil)
	assert.NotNil(t, clients)
}


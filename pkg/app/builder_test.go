package app

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/stretchr/testify/assert"
)

func TestNewAppBuilder(t *testing.T) {
	builder := NewAppBuilder()
	assert.NotNil(t, builder)
	assert.NotNil(t, builder.engine)
}

func TestAppBuilder_WithContext(t *testing.T) {
	builder := NewAppBuilder()
	ctx := context.Background()
	result := builder.WithContext(ctx)
	assert.Equal(t, ctx, result.engine.ctx)
	assert.Equal(t, builder, result)
}

func TestAppBuilder_WithContext_Nil(t *testing.T) {
	builder := NewAppBuilder()
	result := builder.WithContext(nil)
	assert.NotNil(t, result.GetErrors())
	assert.Greater(t, len(result.GetErrors()), 0)
}

func TestAppBuilder_SetLogger(t *testing.T) {
	builder := NewAppBuilder()
	log := &mockLogger{}
	result := builder.SetLogger(log)
	assert.Equal(t, log, result.engine.Log)
	assert.Equal(t, builder, result)
}

func TestAppBuilder_SetLogger_Nil(t *testing.T) {
	builder := NewAppBuilder()
	result := builder.SetLogger(nil)
	assert.NotNil(t, result.GetErrors())
	assert.Greater(t, len(result.GetErrors()), 0)
}

func TestAppBuilder_WithMiddleware_NilRouter(t *testing.T) {
	builder := NewAppBuilder()
	result := builder.WithMiddleware(func(router.Service) {})
	assert.NotNil(t, result.GetErrors())
	assert.Greater(t, len(result.GetErrors()), 0)
}

func TestAppBuilder_WithCustomClient_EmptyName(t *testing.T) {
	builder := NewAppBuilder()
	result := builder.WithCustomClient("", "client")
	assert.NotNil(t, result.GetErrors())
	assert.Greater(t, len(result.GetErrors()), 0)
}

func TestAppBuilder_WithCustomClient_NilClient(t *testing.T) {
	builder := NewAppBuilder()
	result := builder.WithCustomClient("test", nil)
	assert.NotNil(t, result.GetErrors())
	assert.Greater(t, len(result.GetErrors()), 0)
}

func TestAppBuilder_WithCustomClient_Success(t *testing.T) {
	builder := NewAppBuilder()
	customClient := "my-custom-client"
	result := builder.WithCustomClient("test-client", customClient)
	
	// Verify no errors
	assert.Equal(t, 0, len(result.GetErrors()))
	
	// Verify client is stored
	engine, err := result.Build()
	if err != nil {
		// Build might fail due to router not initialized, but that's OK for this test
		// We just need to verify the client was stored
		engine = result.engine
	}
	
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.Services)
	assert.NotNil(t, engine.Services.CustomClients)
	assert.Equal(t, customClient, engine.Services.CustomClients["test-client"])
	
	// Verify retrieval via GetCustomClient
	retrieved := engine.GetCustomClient("test-client")
	assert.Equal(t, customClient, retrieved)
}

func TestEngine_GetCustomClient_NotExists(t *testing.T) {
	builder := NewAppBuilder()
	engine, _ := builder.Build()
	if engine == nil {
		engine = builder.engine
	}
	
	// Should return nil for non-existent client
	retrieved := engine.GetCustomClient("non-existent")
	assert.Nil(t, retrieved)
}

func TestAppBuilder_WithGracefulShutdown(t *testing.T) {
	builder := NewAppBuilder()
	result := builder.WithGracefulShutdown()
	assert.Equal(t, builder, result)
}

func TestAppBuilder_GetErrors(t *testing.T) {
	builder := NewAppBuilder()
	errors := builder.GetErrors()
	assert.NotNil(t, errors)
	assert.Equal(t, 0, len(errors))
}

func TestAppBuilder_Build_WithErrors(t *testing.T) {
	builder := NewAppBuilder()
	builder.WithContext(nil) // This adds an error
	engine, err := builder.Build()
	assert.Error(t, err)
	assert.Nil(t, engine)
}


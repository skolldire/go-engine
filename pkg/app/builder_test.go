package app

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/config/viper"
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
	//nolint:staticcheck // Testing nil context error handling is intentional
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
	builder.WithContext(nil) //nolint:staticcheck // Testing nil context error handling is intentional
	engine, err := builder.Build()
	assert.Error(t, err)
	assert.Nil(t, engine)
}

func TestAppBuilder_WithCustomClient_PreservedAfterInit(t *testing.T) {
	// This test verifies that custom clients added via WithCustomClient()
	// are preserved when WithInitialization() is called afterwards.
	// This is the fix for the issue where Init() was overwriting the registry.

	builder := NewAppBuilder()
	customClient := "my-custom-client"

	// Add custom client first
	builder = builder.WithCustomClient("test-client", customClient)

	// Verify client is stored before Init()
	assert.NotNil(t, builder.engine.Services)
	assert.NotNil(t, builder.engine.Services.CustomClients)
	assert.Equal(t, customClient, builder.engine.Services.CustomClients["test-client"])

	// Note: WithInitialization() requires WithConfigs() or WithDynamicConfig() first
	// For this test, we'll directly test the Init() method behavior
	// by simulating the scenario where Services already has CustomClients

	// Create an app with existing Services containing CustomClients
	app := &App{
		Engine: &Engine{
			ctx:      context.Background(),
			Log:      &mockLogger{},
			Services: NewServiceRegistry(),
		},
	}

	// Add a custom client to the existing registry
	app.Engine.Services.CustomClients["preserved-client"] = "preserved-value"

	// Set minimal config to allow Init() to proceed (it will fail AWS config but that's OK)
	// We're just testing that CustomClients are preserved
	app.Engine.Conf = &viper.Config{
		Aws: viper.AwsConfig{
			Region: "us-east-1",
		},
		Telemetry: nil, // Explicitly set to nil to avoid panic in createTelemetry
	}

	// Call Init() - this should preserve the existing CustomClients
	result := app.Init()

	// Verify that CustomClients were preserved
	// Note: Init() may fail due to AWS config, but CustomClients should still be preserved
	if result.Engine.Services != nil {
		preserved := result.Engine.Services.CustomClients["preserved-client"]
		assert.Equal(t, "preserved-value", preserved, "CustomClients should be preserved after Init()")
	}
}

package app

import (
	"sync"
	"testing"
)

func TestNewServiceRegistry(t *testing.T) {
	registry := NewServiceRegistry()

	if registry == nil {
		t.Fatal("NewServiceRegistry() returned nil")
	}

	if registry.RESTClients == nil {
		t.Error("RESTClients map not initialized")
	}

	if registry.GRPCClients == nil {
		t.Error("GRPCClients map not initialized")
	}

	if registry.SQSClients == nil {
		t.Error("SQSClients map not initialized")
	}

	if registry.SNSClients == nil {
		t.Error("SNSClients map not initialized")
	}

	if registry.DynamoDBClients == nil {
		t.Error("DynamoDBClients map not initialized")
	}

	if registry.RedisClients == nil {
		t.Error("RedisClients map not initialized")
	}

	if registry.SQLConnections == nil {
		t.Error("SQLConnections map not initialized")
	}

	if registry.SSMClients == nil {
		t.Error("SSMClients map not initialized")
	}

	if registry.SESClients == nil {
		t.Error("SESClients map not initialized")
	}

	if registry.S3Clients == nil {
		t.Error("S3Clients map not initialized")
	}

	if registry.MemcachedClients == nil {
		t.Error("MemcachedClients map not initialized")
	}

	if registry.MongoDBClients == nil {
		t.Error("MongoDBClients map not initialized")
	}

	if registry.RabbitMQClients == nil {
		t.Error("RabbitMQClients map not initialized")
	}
}

func TestNewConfigRegistry(t *testing.T) {
	registry := NewConfigRegistry()

	if registry == nil {
		t.Fatal("NewConfigRegistry() returned nil")
	}

	if registry.Repositories == nil {
		t.Error("Repositories map not initialized")
	}

	if registry.UseCases == nil {
		t.Error("UseCases map not initialized")
	}

	if registry.Handlers == nil {
		t.Error("Handlers map not initialized")
	}

	if registry.Batches == nil {
		t.Error("Batches map not initialized")
	}
}

func TestServiceRegistry_AddClient(t *testing.T) {
	registry := NewServiceRegistry()

	// Test that maps are initialized and can be used
	registry.RESTClients["test"] = nil
	if len(registry.RESTClients) != 1 {
		t.Error("Failed to add client to RESTClients")
	}

	registry.SQSClients["test"] = nil
	if len(registry.SQSClients) != 1 {
		t.Error("Failed to add client to SQSClients")
	}
}

func TestConfigRegistry_AddConfig(t *testing.T) {
	registry := NewConfigRegistry()

	// Test that maps are initialized and can be used
	registry.Repositories["test"] = "test-value"
	if len(registry.Repositories) != 1 {
		t.Error("Failed to add config to Repositories")
	}

	registry.UseCases["test"] = "test-value"
	if len(registry.UseCases) != 1 {
		t.Error("Failed to add config to UseCases")
	}

	registry.Handlers["test"] = "test-value"
	if len(registry.Handlers) != 1 {
		t.Error("Failed to add config to Handlers")
	}

	registry.Batches["test"] = "test-value"
	if len(registry.Batches) != 1 {
		t.Error("Failed to add config to Batches")
	}
}

// TestGetServices_ConcurrentAccess verifies that GetServices() is thread-safe
func TestGetServices_ConcurrentAccess(t *testing.T) {
	engine := &Engine{}
	const numGoroutines = 100
	var wg sync.WaitGroup
	results := make([]*ServiceRegistry, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = engine.GetServices()
		}(i)
	}
	wg.Wait()

	// All goroutines should get the same instance
	firstResult := results[0]
	if firstResult == nil {
		t.Fatal("GetServices() returned nil")
	}

	for i, result := range results {
		if result != firstResult {
			t.Errorf("Goroutine %d got different instance: expected %p, got %p", i, firstResult, result)
		}
	}
} // TestGetConfigs_ConcurrentAccess verifies that GetConfigs() is thread-safe
func TestGetConfigs_ConcurrentAccess(t *testing.T) {
	engine := &Engine{}
	const numGoroutines = 100
	var wg sync.WaitGroup
	results := make([]*ConfigRegistry, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = engine.GetConfigs()
		}(i)
	}
	wg.Wait()

	// All goroutines should get the same instance
	firstResult := results[0]
	if firstResult == nil {
		t.Fatal("GetConfigs() returned nil")
	}

	for i, result := range results {
		if result != firstResult {
			t.Errorf("Goroutine %d got different instance: expected %p, got %p", i, firstResult, result)
		}
	}
}

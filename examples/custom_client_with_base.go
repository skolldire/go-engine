//go:build example_custom_client || example_all
// +build example_custom_client example_all

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

type CustomServiceClient struct {
	*client.BaseClient
	endpoint string
}

type CustomServiceConfig struct {
	Endpoint       string
	EnableLogging  bool
	WithResilience bool
	Resilience     resilience.Config
	Timeout        time.Duration
}

func NewCustomServiceClient(config CustomServiceConfig, log logger.Service) *CustomServiceClient {
	baseConfig := client.BaseConfig{
		EnableLogging:  config.EnableLogging,
		WithResilience: config.WithResilience,
		Resilience:     config.Resilience,
		Timeout:        config.Timeout,
	}

	return &CustomServiceClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "CustomService"),
		endpoint:   config.Endpoint,
	}
}

func (c *CustomServiceClient) ProcessData(ctx context.Context, data string) (string, error) {
	result, err := c.Execute(ctx, "ProcessData", func() (interface{}, error) {
		return c.performProcessing(data)
	})

	if err != nil {
		return "", err
	}

	processed, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected result type: %T", result)
	}

	return processed, nil
}

func (c *CustomServiceClient) performProcessing(data string) (string, error) {
	time.Sleep(10 * time.Millisecond)
	return fmt.Sprintf("processed: %s", data), nil
}

func main() {
	ctx := context.Background()

	tracer := logger.NewService(logger.Config{
		Level: "debug",
	}, nil)

	config := CustomServiceConfig{
		Endpoint:       "https://api.example.com",
		EnableLogging:  true,
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig:          nil,
			CircuitBreakerConfig: nil,
		},
		Timeout: 5 * time.Second,
	}

	client := NewCustomServiceClient(config, tracer)

	result, err := client.ProcessData(ctx, "test-data")
	if err != nil {
		fmt.Printf("error processing data: %v\n", err)
		return
	}

	fmt.Printf("result: %s\n", result)
}

func ExampleCustomClient() {
	ctx := context.Background()

	tracer := logger.NewService(logger.Config{
		Level: "debug",
	}, nil)

	config := CustomServiceConfig{
		Endpoint:       "https://api.example.com",
		EnableLogging:  true,
		WithResilience: true,
		Resilience: resilience.Config{
			RetryConfig:          nil,
			CircuitBreakerConfig: nil,
		},
		Timeout: 5 * time.Second,
	}

	client := NewCustomServiceClient(config, tracer)

	result, err := client.ProcessData(ctx, "test-data")
	if err != nil {
		fmt.Printf("error processing data: %v\n", err)
		return
	}

	fmt.Printf("result: %s\n", result)
}

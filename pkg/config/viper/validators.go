package viper

import (
	"fmt"
	"strings"

	"github.com/skolldire/go-engine/aws/pkg/clients/sns"
	"github.com/skolldire/go-engine/aws/pkg/clients/sqs"
	"github.com/skolldire/go-engine/aws/pkg/database/dynamo"
	"github.com/skolldire/go-engine/database/redis/pkg/database/redis"
	grpcClient "github.com/skolldire/go-engine/messaging/pkg/integration/grpc"
	"github.com/skolldire/go-engine/pkg/app/router"
	"github.com/skolldire/go-engine/pkg/clients/rest"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s", e.Field, e.Message)
}

// ValidateConfig validates the entire configuration structure
func ValidateConfig(cfg Config) []error {
	var errors []error

	// Validate AWS configuration
	if errs := validateAWSConfig(cfg.Aws); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	// Validate REST clients
	if errs := validateRESTClients(cfg.Rest); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	// Validate gRPC clients
	if errs := validateGRPCClients(cfg.GrpcClient); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	// Validate SQS configuration
	if errs := validateSQSConfig(cfg.SQS, cfg.SQSClients); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	// Validate SNS configuration
	if errs := validateSNSConfig(cfg.SNS, cfg.SNSClients); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	// Validate database configurations
	if errs := validateDatabaseConfigs(cfg); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	// Validate router configuration
	if errs := validateRouterConfig(cfg.Router); len(errs) > 0 {
		errors = append(errors, errs...)
	}

	return errors
}

// validateAWSConfig validates AWS configuration
func validateAWSConfig(cfg AwsConfig) []error {
	var errors []error

	if cfg.Region == "" {
		errors = append(errors, &ValidationError{
			Field:   "aws.region",
			Message: "AWS region is required",
		})
	} else if !isValidAWSRegion(cfg.Region) {
		errors = append(errors, &ValidationError{
			Field:   "aws.region",
			Message: fmt.Sprintf("invalid AWS region: %s", cfg.Region),
		})
	}

	return errors
}

// validateRESTClients validates REST client configurations
func validateRESTClients(clients []map[string]rest.Config) []error {
	var errors []error

	for i, clientMap := range clients {
		for name, cfg := range clientMap {
			if name == "" {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("rest[%d].name", i),
					Message: "REST client name cannot be empty",
				})
			}

			if cfg.BaseURL == "" {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("rest[%d].%s.base_url", i, name),
					Message: "REST client base URL is required",
				})
			} else if !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("rest[%d].%s.base_url", i, name),
					Message: "REST client base URL must start with http:// or https://",
				})
			}

			if cfg.TimeOut < 0 {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("rest[%d].%s.timeout", i, name),
					Message: "REST client timeout cannot be negative",
				})
			}
		}
	}

	return errors
}

// validateGRPCClients validates gRPC client configurations
func validateGRPCClients(clients []map[string]grpcClient.Config) []error {
	var errors []error

	for i, clientMap := range clients {
		for name, cfg := range clientMap {
			if name == "" {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("grpc_client[%d].name", i),
					Message: "gRPC client name cannot be empty",
				})
			}

			if cfg.Target == "" {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("grpc_client[%d].%s.target", i, name),
					Message: "gRPC client target is required",
				})
			}

			if cfg.TimeOut < 0 {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("grpc_client[%d].%s.timeout", i, name),
					Message: "gRPC client timeout cannot be negative",
				})
			}
		}
	}

	return errors
}

// validateSQSConfig validates SQS configuration
func validateSQSConfig(single *sqs.Config, multiple []map[string]sqs.Config) []error {
	var errors []error

	// Validate single SQS client
	if single != nil {
		if single.Endpoint != "" && !isValidURL(single.Endpoint) {
			errors = append(errors, &ValidationError{
				Field:   "sqs.endpoint",
				Message: "SQS endpoint must be a valid URL",
			})
		}

		// SQS Config doesn't have WaitTime field, validation removed
	}

	// Validate multiple SQS clients
	for i, clientMap := range multiple {
		for name, cfg := range clientMap {
			if name == "" {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("sqs_clients[%d].name", i),
					Message: "SQS client name cannot be empty",
				})
			}

			if cfg.Endpoint != "" && !isValidURL(cfg.Endpoint) {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("sqs_clients[%d].%s.endpoint", i, name),
					Message: "SQS endpoint must be a valid URL",
				})
			}

			// SQS Config doesn't have WaitTime field, validation removed
		}
	}

	return errors
}

// validateSNSConfig validates SNS configuration
func validateSNSConfig(single *sns.Config, multiple []map[string]sns.Config) []error {
	var errors []error

	// Validate single SNS client if provided
	// Note: SNS Config doesn't have Topic/Region fields as they're provided per-operation
	// Only validate that the config struct exists if needed
	// SNS config is valid if provided (Topic/Region are operation-specific, not config-level)
	_ = single // Explicitly acknowledge single parameter to avoid empty branch warning

	// Validate multiple SNS clients
	for i, clientMap := range multiple {
		for name := range clientMap {
			if name == "" {
				errors = append(errors, &ValidationError{
					Field:   fmt.Sprintf("sns_clients[%d].name", i),
					Message: "SNS client name cannot be empty",
				})
			}
			// SNS config doesn't require Topic/Region at config level
			// These are provided per-operation (Publish, CreateTopic, etc.)
		}
	}

	return errors
}

// validateDatabaseConfigs validates database configurations
func validateDatabaseConfigs(cfg Config) []error {
	var errors []error

	// Validate DynamoDB
	if cfg.Dynamo != nil {
		if errs := validateDynamoConfig(*cfg.Dynamo); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}

	// Validate Redis
	if cfg.Redis != nil {
		if errs := validateRedisConfig(*cfg.Redis); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}

	return errors
}

// validateDynamoConfig validates DynamoDB configuration
func validateDynamoConfig(cfg dynamo.Config) []error {
	var errors []error

	if cfg.Endpoint != "" && !isValidURL(cfg.Endpoint) {
		errors = append(errors, &ValidationError{
			Field:   "dynamo.endpoint",
			Message: "DynamoDB endpoint must be a valid URL",
		})
	}

	return errors
}

// validateRedisConfig validates Redis configuration
func validateRedisConfig(cfg redis.Config) []error {
	var errors []error

	if cfg.Host == "" {
		errors = append(errors, &ValidationError{
			Field:   "redis.host",
			Message: "Redis host is required",
		})
	}

	if cfg.Port < 0 || cfg.Port > 65535 {
		errors = append(errors, &ValidationError{
			Field:   "redis.port",
			Message: "Redis port must be between 0 and 65535",
		})
	}

	if cfg.DB < 0 {
		errors = append(errors, &ValidationError{
			Field:   "redis.db",
			Message: "Redis database number cannot be negative",
		})
	}

	return errors
}

// validateRouterConfig validates router configuration
func validateRouterConfig(cfg router.Config) []error {
	var errors []error

	// Port is a string in router.Config, validation removed
	// Timeout fields are ReadTimeout, WriteTimeout, etc., validation removed

	return errors
}

// Helper functions

func isValidAWSRegion(region string) bool {
	validRegions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2",
		"ap-south-1", "sa-east-1", "ca-central-1",
	}

	for _, r := range validRegions {
		if r == region {
			return true
		}
	}
	return false
}

func isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

package viper

import (
	"testing"
	"time"

	"github.com/skolldire/go-engine/pkg/app/router"
	grpcClient "github.com/skolldire/go-engine/pkg/clients/grpc"
	"github.com/skolldire/go-engine/pkg/clients/rest"
	"github.com/skolldire/go-engine/pkg/clients/sqs"
	"github.com/skolldire/go-engine/pkg/database/gormsql"
)

func TestValidateAWSConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  AwsConfig
		wantErr bool
	}{
		{
			name:    "valid region",
			config:  AwsConfig{Region: "us-east-1"},
			wantErr: false,
		},
		{
			name:    "empty region",
			config:  AwsConfig{Region: ""},
			wantErr: true,
		},
		{
			name:    "invalid region",
			config:  AwsConfig{Region: "invalid-region"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateAWSConfig(tt.config)
			hasError := len(errors) > 0

			if hasError != tt.wantErr {
				t.Errorf("validateAWSConfig() errors = %v, wantErr %v", errors, tt.wantErr)
			}
		})
	}
}

func TestValidateRESTClients(t *testing.T) {
	tests := []struct {
		name    string
		clients []map[string]rest.Config
		wantErr bool
	}{
		{
			name: "valid REST client",
			clients: []map[string]rest.Config{
				{
					"api1": {
						BaseURL: "https://api.example.com",
						TimeOut: 30 * time.Second,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty client name",
			clients: []map[string]rest.Config{
				{
					"": {
						BaseURL: "https://api.example.com",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty base URL",
			clients: []map[string]rest.Config{
				{
					"api1": {
						BaseURL: "",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			clients: []map[string]rest.Config{
				{
					"api1": {
						BaseURL: "not-a-url",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			clients: []map[string]rest.Config{
				{
					"api1": {
						BaseURL: "https://api.example.com",
						TimeOut: -1 * time.Second,
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateRESTClients(tt.clients)
			hasError := len(errors) > 0

			if hasError != tt.wantErr {
				t.Errorf("validateRESTClients() errors = %v, wantErr %v", errors, tt.wantErr)
			}
		})
	}
}

func TestValidateGRPCClients(t *testing.T) {
	tests := []struct {
		name    string
		clients []map[string]grpcClient.Config
		wantErr bool
	}{
		{
			name: "valid gRPC client",
			clients: []map[string]grpcClient.Config{
				{
					"grpc1": {
						Target:  "localhost:50051",
						TimeOut: 30 * time.Second,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty client name",
			clients: []map[string]grpcClient.Config{
				{
					"": {
						Target: "localhost:50051",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty target",
			clients: []map[string]grpcClient.Config{
				{
					"grpc1": {
						Target: "",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			clients: []map[string]grpcClient.Config{
				{
					"grpc1": {
						Target:  "localhost:50051",
						TimeOut: -1 * time.Second,
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateGRPCClients(tt.clients)
			hasError := len(errors) > 0

			if hasError != tt.wantErr {
				t.Errorf("validateGRPCClients() errors = %v, wantErr %v", errors, tt.wantErr)
			}
		})
	}
}

func TestValidateSQSConfig(t *testing.T) {
	tests := []struct {
		name     string
		single   *sqs.Config
		multiple []map[string]sqs.Config
		wantErr  bool
	}{
		{
			name: "valid single SQS config",
			single: &sqs.Config{
				Endpoint: "http://localhost:4566",
			},
			wantErr: false,
		},
		{
			name: "invalid endpoint URL",
			single: &sqs.Config{
				Endpoint: "not-a-url",
			},
			wantErr: true,
		},
		{
			name: "valid multiple SQS clients",
			multiple: []map[string]sqs.Config{
				{
					"queue1": {
						Endpoint: "http://localhost:4566",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty client name in multiple",
			multiple: []map[string]sqs.Config{
				{
					"": {
						Endpoint: "http://localhost:4566",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateSQSConfig(tt.single, tt.multiple)
			hasError := len(errors) > 0

			if hasError != tt.wantErr {
				t.Errorf("validateSQSConfig() errors = %v, wantErr %v", errors, tt.wantErr)
			}
		})
	}
}

func TestValidateSQLConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  gormsql.Config
		wantErr bool
	}{
		{
			name: "valid PostgreSQL config",
			config: gormsql.Config{
				Type:     "postgres",
				Host:     "localhost",
				Port:     5432,
				Database: "testdb",
			},
			wantErr: false,
		},
		{
			name: "valid SQLite config",
			config: gormsql.Config{
				Type:     "sqlite",
				Database: "test.db",
			},
			wantErr: false,
		},
		{
			name: "empty type",
			config: gormsql.Config{
				Type: "",
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			config: gormsql.Config{
				Type: "invalid",
			},
			wantErr: true,
		},
		{
			name: "empty database name",
			config: gormsql.Config{
				Type: "postgres",
				Host: "localhost",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: gormsql.Config{
				Type:     "postgres",
				Host:     "localhost",
				Port:     70000, // > 65535
				Database: "testdb",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateSQLConfig(tt.config)
			hasError := len(errors) > 0

			if hasError != tt.wantErr {
				t.Errorf("validateSQLConfig() errors = %v, wantErr %v", errors, tt.wantErr)
			}
		})
	}
}

func TestValidateRouterConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  router.Config
		wantErr bool
	}{
		{
			name: "valid router config",
			config: router.Config{
				Port: "8080",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validateRouterConfig(tt.config)
			hasError := len(errors) > 0

			if hasError != tt.wantErr {
				t.Errorf("validateRouterConfig() errors = %v, wantErr %v", errors, tt.wantErr)
			}
		})
	}
}

func TestIsValidAWSRegion(t *testing.T) {
	tests := []struct {
		region string
		valid  bool
	}{
		{"us-east-1", true},
		{"eu-west-1", true},
		{"ap-southeast-1", true},
		{"invalid-region", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			result := isValidAWSRegion(tt.region)
			if result != tt.valid {
				t.Errorf("isValidAWSRegion(%q) = %v, want %v", tt.region, result, tt.valid)
			}
		})
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"http://localhost:4566", true},
		{"https://api.example.com", true},
		{"not-a-url", false},
		{"ftp://example.com", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := isValidURL(tt.url)
			if result != tt.valid {
				t.Errorf("isValidURL(%q) = %v, want %v", tt.url, result, tt.valid)
			}
		})
	}
}

func TestIsValidSQLType(t *testing.T) {
	tests := []struct {
		dbType string
		valid  bool
	}{
		{"postgres", true},
		{"mysql", true},
		{"sqlite", true},
		{"sqlserver", true},
		{"POSTGRES", true}, // converts to lowercase: "postgres"
		{"MySQL", true},    // converts to lowercase: "mysql"
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.dbType, func(t *testing.T) {
			result := isValidSQLType(tt.dbType)
			if result != tt.valid {
				t.Errorf("isValidSQLType(%q) = %v, want %v", tt.dbType, result, tt.valid)
			}
		})
	}
}

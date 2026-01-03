# Examples

This directory contains comprehensive examples demonstrating various features of Go Engine.

Each example uses build tags to avoid compilation conflicts. To run a specific example, use the corresponding build tag.

## Available Examples

### 1. Complete Application (`complete_application.go`)
Full-featured application with dynamic configuration, feature flags, and custom middleware.

**Features demonstrated:**
- Dynamic configuration loading
- Custom middleware setup
- Route registration
- Health check endpoint
- Feature flags endpoint
- Client information endpoint

**Usage:**
```bash
go run -tags example_complete examples/complete_application.go
# Or to run all examples:
go run -tags example_all examples/complete_application.go
```

### 2. Multiple Clients (`multiple_clients.go`)
Demonstrates how to use multiple clients of the same type.

**Features demonstrated:**
- Multiple REST clients
- Multiple SQS clients
- Multiple SNS clients
- Multiple DynamoDB clients
- Multiple Redis clients
- Multiple SQL connections
- Backward compatibility with singleton clients

**Usage:**
```bash
go run -tags example_multiple examples/multiple_clients.go
```

### 3. Feature Flags (`feature_flags.go`)
Shows how to use feature flags for dynamic feature control.

**Features demonstrated:**
- Checking boolean flags
- Getting string values
- Getting integer values
- Getting all flags
- Dynamic flag updates

**Usage:**
```bash
go run -tags example_feature_flags examples/feature_flags.go
```

### 4. Dynamic Config Reload (`dynamic_config_reload.go`)
Demonstrates automatic configuration reloading.

**Features demonstrated:**
- File watching
- Automatic reload on file changes
- Configuration monitoring
- Feature flag monitoring

**Usage:**
```bash
go run -tags example_dynamic_config examples/dynamic_config_reload.go
# Modify config files to see automatic reload
```

### 5. Custom Client with Base (`custom_client_with_base.go`)
Shows how to create custom clients using BaseClient.

**Features demonstrated:**
- Extending BaseClient
- Custom service implementation
- Resilience integration
- Logging integration

**Usage:**
```bash
go run -tags example_custom_client examples/custom_client_with_base.go
```

### 6. REST Client Usage (`rest_client_usage.go`)
Demonstrates REST client operations.

**Features demonstrated:**
- GET requests
- POST requests
- Multiple API clients
- Header management

**Usage:**
```bash
go run -tags example_rest examples/rest_client_usage.go
```

### 7. Redis Operations (`redis_operations.go`)
Shows Redis client operations.

**Features demonstrated:**
- Basic operations (Set/Get)
- Hash operations
- List operations
- Set operations
- Multiple Redis clients

**Usage:**
```bash
go run -tags example_redis examples/redis_operations.go
```

### 8. SQS/SNS Integration (`sqs_sns_integration.go`)
Demonstrates AWS SQS and SNS integration.

**Features demonstrated:**
- SQS message sending
- SQS message receiving
- SNS message publishing
- Multiple queues and topics
- Message attributes

**Usage:**
```bash
go run -tags example_sqs_sns examples/sqs_sns_integration.go
```

### 9. DynamoDB Operations (`dynamodb_operations.go`)
Shows DynamoDB operations.

**Features demonstrated:**
- PutItem operations
- GetItem operations
- Query operations
- Multiple DynamoDB clients

**Usage:**
```bash
go run -tags example_dynamodb examples/dynamodb_operations.go
```

## Running All Examples

To compile all examples at once (useful for testing):
```bash
go build -tags example_all ./examples/...
```

## Build Tags Reference

- `example_complete` - Complete application example
- `example_multiple` - Multiple clients example
- `example_feature_flags` - Feature flags example
- `example_dynamic_config` - Dynamic config reload example
- `example_custom_client` - Custom client example
- `example_rest` - REST client example
- `example_redis` - Redis operations example
- `example_sqs_sns` - SQS/SNS integration example
- `example_dynamodb` - DynamoDB operations example
- `example_all` - All examples (for building all at once)

## Configuration Examples

### Basic Configuration (`config/application.yaml`)
```yaml
log:
  level: "info"
  path: "logs/app.log"

router:
  port: "8080"
  read_timeout: 10
  write_timeout: 30

enable_config_watch: true

feature_flags:
  enable_new_api: true
  max_retries: 5
  api_version: "v2"

rest:
  - api1:
      base_url: "https://api1.example.com"
      timeout: 30
      enable_logging: true
      with_resilience: true
  - api2:
      base_url: "https://api2.example.com"
      timeout: 15

sqs_clients:
  - queue1:
      endpoint: "http://localhost:4566"
      enable_logging: true
  - queue2:
      endpoint: "http://localhost:4567"
      enable_logging: false

sns_clients:
  - topic1:
      base_endpoint: "http://localhost:4566"
  - topic2:
      base_endpoint: "http://localhost:4567"

dynamo_clients:
  - db1:
      endpoint: "http://localhost:4566"
      table_prefix: "dev_"
  - db2:
      endpoint: "http://localhost:4567"
      table_prefix: "prod_"

redis_clients:
  - cache1:
      host: "localhost"
      port: 6379
      db: 0
      enable_logging: true
  - cache2:
      host: "localhost"
      port: 6380
      db: 1

sql_connections:
  - db1:
      driver: "postgres"
      host: "localhost"
      port: 5432
      dbname: "mydb"
  - db2:
      driver: "mysql"
      host: "localhost"
      port: 3306
      dbname: "mydb"
```

## How to Run Examples

Each example uses build tags to avoid compilation conflicts. You must specify a build tag when running an example:

```bash
# Run a specific example
go run -tags example_complete examples/complete_application.go

# Or compile first, then run
go build -tags example_complete examples/complete_application.go
./complete_application
```

**Important:** Without build tags, you'll get compilation errors because multiple `main()` functions exist in the same package.

## Quick Start Examples

### Minimal Application
```go
package main

import (
    "context"
    "github.com/skolldire/go-engine/pkg/app"
)

func main() {
    ctx := context.Background()
    
    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithConfigs().
        WithInitialization().
        WithRouter().
        Build()
    
    if err != nil {
        panic(err)
    }
    
    engine.Run()
}
```

### With Dynamic Configuration
```go
package main

import (
    "context"
    "github.com/skolldire/go-engine/pkg/app"
)

func main() {
    ctx := context.Background()
    
    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithDynamicConfig().
        WithInitialization().
        WithRouter().
        Build()
    
    if err != nil {
        panic(err)
    }
    
    // Use feature flags
    if engine.GetFeatureFlags().IsEnabled("new_feature") {
        // Use new feature
    }
    
    engine.Run()
}
```

### Multiple Clients Example
```go
package main

import (
    "context"
    "github.com/skolldire/go-engine/pkg/app"
)

func main() {
    ctx := context.Background()
    
    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithConfigs().
        WithInitialization().
        WithRouter().
        Build()
    
    if err != nil {
        panic(err)
    }
    
    // Use multiple REST clients
    api1 := engine.GetRestClient("api1")
    api2 := engine.GetRestClient("api2")
    
    // Use multiple SQS clients
    queue1 := engine.GetSQSClientByName("queue1")
    queue2 := engine.GetSQSClientByName("queue2")
    
    // Use multiple Redis clients
    cache1 := engine.GetRedisClientByName("cache1")
    cache2 := engine.GetRedisClientByName("cache2")
    
    engine.Run()
}
```


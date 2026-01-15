# Go Engine

![GitHub Release](https://img.shields.io/github/v/release/skolldire/go-engine)
![Go Version](https://img.shields.io/github/go-mod/go-version/skolldire/go-engine)

**Go Engine** es un framework completo y moderno para construir aplicaciones empresariales en Go siguiendo principios de Clean Architecture. Proporciona una base sólida con integraciones listas para usar con servicios AWS, bases de datos, mensajería y autenticación.

## 🚀 Características Principales

### Core Framework
- **Builder Pattern**: Construcción fluida y declarativa de aplicaciones
- **Service Registry**: Sistema de registro centralizado para múltiples instancias de clientes
- **Dependency Injection**: Gestión automática de dependencias entre componentes
- **Graceful Shutdown**: Manejo controlado de cierre de aplicación
- **Error Handling**: Sistema robusto de manejo y propagación de errores

### Configuración y Observabilidad
- **Configuración Dinámica**: Sistema basado en Viper con soporte para múltiples formatos
- **Feature Flags**: Sistema de feature flags con actualización dinámica
- **Logging Estructurado**: Logging basado en Logrus con formato ECS
- **Telemetría**: Integración con métricas y tracing
- **Validación**: Validación de datos usando go-playground/validator

### Clientes Integrados

#### AWS Services
- **Cognito**: Autenticación completa con MFA, gestión de sesiones y validación JWT
- **S3**: Almacenamiento de objetos
- **SQS**: Colas de mensajes
- **SNS**: Notificaciones push
- **SES**: Envío de emails
- **SSM**: Parameter Store
- **DynamoDB**: Base de datos NoSQL

#### Bases de Datos
- **Redis**: Cache y almacenamiento en memoria
- **SQL**: Soporte para PostgreSQL, MySQL, SQLite vía GORM
- **MongoDB**: Base de datos NoSQL documental
- **Memcached**: Cache distribuido

#### Mensajería y Comunicación
- **REST**: Cliente HTTP con retry y circuit breaker
- **gRPC**: Cliente gRPC con soporte para múltiples servicios
- **RabbitMQ**: Message broker

### Utilidades y Resiliencia
- **Circuit Breaker**: Protección contra fallos en cascada
- **Retry & Backoff**: Reintentos inteligentes con backoff exponencial
- **Task Executor**: Ejecución de tareas con control de concurrencia
- **Error Handler**: Manejo centralizado de errores
- **File Utils**: Utilidades para manejo de archivos

## 📦 Instalación

```bash
go get github.com/skolldire/go-engine
```

## 🎯 Quick Start

### Aplicación Mínima

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/skolldire/go-engine/pkg/app"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Manejo de señales para graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
    go func() {
        <-sigCh
        fmt.Println("Cerrando aplicación...")
        cancel()
    }()

    // Construir aplicación
    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithConfigs().
        WithInitialization().
        WithRouter().
        WithGracefulShutdown().
        Build()

    if err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }

    // Verificar errores de inicialización
    if errs := engine.GetErrors(); len(errs) > 0 {
        fmt.Printf("Errores de inicialización: %v\n", errs)
        os.Exit(1)
    }

    // Ejecutar aplicación
    if err := engine.Run(); err != nil {
        fmt.Printf("Error ejecutando aplicación: %v\n", err)
        os.Exit(1)
    }
}
```

## ⚙️ Configuración

Go Engine utiliza archivos de configuración YAML. Crea un archivo `config/application.yaml`:

```yaml
# Logging
log:
  level: "info"
  format: "json"  # json o text
  path: "logs/app.log"

# Router HTTP
router:
  port: 8080
  timeout: 30
  read_timeout: 15
  write_timeout: 15

# AWS
aws:
  region: "us-east-1"
  endpoint: ""  # Opcional, para LocalStack

# Feature Flags
feature_flags:
  enabled: true
  file_path: "config/features.yaml"
  watch: true  # Observar cambios en archivo

# Telemetría
telemetry:
  enabled: true
  metrics_port: 9090

# Cognito (Opcional)
cognito:
  region: "us-east-1"
  user_pool_id: "us-east-1_XXXXXXXXX"
  client_id: "your-client-id"
  client_secret: ""  # Opcional
  enable_logging: true
  timeout: 30

# Redis (Cliente único)
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
  max_retries: 3

# Redis (Múltiples clientes)
redis_clients:
  - cache1:
      addr: "localhost:6379"
      db: 0
  - cache2:
      addr: "localhost:6380"
      db: 1

# SQL (Cliente único)
database_sql:
  driver: "postgres"
  host: "localhost"
  port: 5432
  dbname: "mydb"
  username: "user"
  password: "pass"
  ssl_mode: "disable"

# SQL (Múltiples conexiones)
sql_connections:
  - db1:
      driver: "postgres"
      host: "localhost"
      port: 5432
      dbname: "db1"
  - db2:
      driver: "mysql"
      host: "localhost"
      port: 3306
      dbname: "db2"

# DynamoDB (Cliente único)
dynamo:
  endpoint: "http://localhost:4566"  # LocalStack
  table_prefix: "dev_"

# DynamoDB (Múltiples clientes)
dynamo_clients:
  - dynamo1:
      endpoint: "http://localhost:4566"
      table_prefix: "app1_"
  - dynamo2:
      endpoint: "http://localhost:4566"
      table_prefix: "app2_"

# SQS (Cliente único)
sqs:
  endpoint: "http://localhost:4566"
  wait_time: 20

# SQS (Múltiples clientes)
sqs_clients:
  - queue1:
      endpoint: "http://localhost:4566"
      wait_time: 20
  - queue2:
      endpoint: "http://localhost:4566"
      wait_time: 10

# SNS (Múltiples clientes)
sns_clients:
  - topic1:
      endpoint: "http://localhost:4566"
      topic_prefix: "dev_"
  - topic2:
      endpoint: "http://localhost:4566"

# S3 (Múltiples clientes)
s3_clients:
  - bucket1:
      region: "us-east-1"
      bucket: "my-bucket-1"
  - bucket2:
      region: "us-west-2"
      bucket: "my-bucket-2"

# SES (Múltiples clientes)
ses_clients:
  - sender1:
      region: "us-east-1"
      from_email: "noreply@example.com"
  - sender2:
      region: "us-west-2"

# SSM (Múltiples clientes)
ssm_clients:
  - params1:
      region: "us-east-1"
  - params2:
      region: "us-west-2"

# REST (Múltiples clientes)
rest:
  - api1:
      base_url: "https://api1.example.com"
      timeout: 30
      headers:
        Content-Type: "application/json"
        Authorization: "Bearer token"
  - api2:
      base_url: "https://api2.example.com"
      timeout: 15

# gRPC (Múltiples clientes)
grpc:
  - service1:
      address: "localhost:50051"
      timeout: 10
  - service2:
      address: "localhost:50052"
      timeout: 15

# RabbitMQ (Múltiples clientes)
rabbitmq_clients:
  - queue1:
      url: "amqp://guest:guest@localhost:5672/"
      exchange: "events"
  - queue2:
      url: "amqp://guest:guest@localhost:5672/"
      exchange: "notifications"

# MongoDB (Múltiples clientes)
mongodb_clients:
  - db1:
      uri: "mongodb://localhost:27017"
      database: "app1"
  - db2:
      uri: "mongodb://localhost:27017"
      database: "app2"

# Memcached (Múltiples clientes)
memcached_clients:
  - cache1:
      servers:
        - "localhost:11211"
  - cache2:
      servers:
        - "localhost:11212"
```

## 🏗️ Arquitectura

Go Engine sigue principios de Clean Architecture:

```
┌─────────────────────────────────────────┐
│         Application Layer               │
│  (Handlers, Routes, Middleware)        │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│         Domain Layer                    │
│  (Use Cases, Business Logic)            │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│         Infrastructure Layer            │
│  (Repositories, Clients, Databases)    │
└─────────────────────────────────────────┘
```

### Componentes Principales

- **Engine**: Núcleo de la aplicación que gestiona todos los componentes
- **Service Registry**: Registro centralizado de todos los clientes de servicios
- **Config Registry**: Gestión de configuraciones por capas (repositorios, casos de uso, handlers)
- **Router**: Enrutador HTTP con soporte para middleware
- **Logger**: Sistema de logging estructurado
- **Feature Flags**: Sistema de feature flags dinámico

## 📚 Guías de Uso

### Cliente Cognito (Autenticación)

El cliente Cognito proporciona autenticación completa con soporte para MFA:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/skolldire/go-engine/pkg/app"
    "github.com/skolldire/go-engine/pkg/clients/cognito"
)

func main() {
    ctx := context.Background()
    
    engine, _ := app.NewAppBuilder().
        WithContext(ctx).
        WithConfigs().
        WithInitialization().
        Build()

    cognitoClient := engine.GetCognitoClient()
    if cognitoClient == nil {
        log.Fatal("Cognito no configurado")
    }

    // Registrar usuario
    user, err := cognitoClient.RegisterUser(ctx, cognito.RegisterUserRequest{
        Username: "johndoe",
        Email:    "john@example.com",
        Password: "SecurePass123!",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Usuario registrado: %s\n", user.ID)

    // Confirmar registro
    err = cognitoClient.ConfirmSignUp(ctx, cognito.ConfirmSignUpRequest{
        Username:         "johndoe",
        ConfirmationCode: "123456",
    })

    // Autenticar
    tokens, err := cognitoClient.Authenticate(ctx, cognito.AuthenticateRequest{
        Username: "johndoe",
        Password: "SecurePass123!",
    })
    
    // Manejar MFA si es requerido
    if mfaErr, ok := err.(*cognito.MFARequiredError); ok {
        // Usuario ingresa código MFA
        tokens, err = cognitoClient.RespondToMFAChallenge(ctx, cognito.MFAChallengeRequest{
            Username:      "johndoe",
            SessionToken:  mfaErr.SessionToken,
            MFACode:       "123456",
            ChallengeType: mfaErr.ChallengeType,
        })
    }

    // Configurar MFA TOTP
    association, _ := cognitoClient.AssociateSoftwareToken(ctx, tokens.AccessToken)
    fmt.Printf("QR Code: %s\n", association.QRCode)
    
    // Verificar código TOTP
    cognitoClient.VerifySoftwareToken(ctx, tokens.AccessToken, "123456", association.Session)
    
    // Configurar preferencias MFA
    cognitoClient.SetUserMFAPreference(ctx, tokens.AccessToken, false, true)

    // Obtener estado MFA
    status, _ := cognitoClient.GetUserMFAStatus(ctx, tokens.AccessToken)
    fmt.Printf("MFA Habilitado: %v\n", status.MFAEnabled)

    // Cerrar sesión
    cognitoClient.SignOut(ctx, tokens.AccessToken)
    
    // Cerrar todas las sesiones
    cognitoClient.GlobalSignOut(ctx, tokens.AccessToken)

    // Renovar tokens
    newTokens, _ := cognitoClient.RefreshToken(ctx, cognito.RefreshTokenRequest{
        RefreshToken: tokens.RefreshToken,
        Username:     "johndoe",
    })

    // Validar token
    claims, _ := cognitoClient.ValidateToken(ctx, tokens.AccessToken)
    fmt.Printf("Usuario: %s\n", claims.Username)
}
```

### Múltiples Clientes del Mismo Tipo

```go
// Obtener clientes específicos por nombre
redis1 := engine.GetRedisClientByName("cache1")
redis2 := engine.GetRedisClientByName("cache2")

sqs1 := engine.GetSQSClientByName("queue1")
sqs2 := engine.GetSQSClientByName("queue2")

db1 := engine.GetSQLConnectionByName("db1")
db2 := engine.GetSQLConnectionByName("db2")

rest1 := engine.GetRestClient("api1")
rest2 := engine.GetRestClient("api2")
```

### Feature Flags

```go
flags := engine.GetFeatureFlags()
if flags == nil {
    return
}

// Verificar flag booleano
if flags.IsEnabled("new_feature") {
    // Usar nueva funcionalidad
}

// Obtener valor string
apiVersion := flags.GetString("api_version")

// Obtener valor integer
maxRetries := flags.GetInt("max_retries")

// Actualizar dinámicamente
flags.Set("new_feature", true)
```

### REST Client

```go
restClient := engine.GetRestClient("api1")
if restClient == nil {
    return
}

// GET request
response, err := restClient.Get(ctx, "/users/123", nil)

// POST request
body := map[string]interface{}{
    "name": "John",
    "email": "john@example.com",
}
response, err := restClient.Post(ctx, "/users", body)

// Con headers personalizados
headers := map[string]string{
    "X-Custom-Header": "value",
}
response, err := restClient.GetWithHeaders(ctx, "/users", nil, headers)
```

### SQS (Message Queue)

```go
sqsClient := engine.GetSQSClientByName("queue1")
if sqsClient == nil {
    return
}

// Enviar mensaje
messageID, err := sqsClient.SendMessage(ctx, "queue-url", map[string]interface{}{
    "event": "user.created",
    "user_id": "123",
})

// Recibir mensajes
messages, err := sqsClient.ReceiveMessages(ctx, "queue-url", 10)

// Eliminar mensaje
err := sqsClient.DeleteMessage(ctx, "queue-url", "receipt-handle")
```

### DynamoDB

```go
dynamoClient := engine.GetDynamoDBClientByName("dynamo1")
if dynamoClient == nil {
    return
}

// Put item
item := map[string]interface{}{
    "id": "123",
    "name": "John",
}
err := dynamoClient.PutItem(ctx, "table-name", item)

// Get item
result, err := dynamoClient.GetItem(ctx, "table-name", map[string]interface{}{
    "id": "123",
})

// Query
items, err := dynamoClient.Query(ctx, "table-name", "index-name", 
    "partition-key", "sort-key")
```

### Redis

```go
redisClient := engine.GetRedisClientByName("cache1")
if redisClient == nil {
    return
}

// Set
err := redisClient.Set(ctx, "key", "value", time.Hour)

// Get
value, err := redisClient.Get(ctx, "key")

// Delete
err := redisClient.Delete(ctx, "key")
```

### SQL (GORM)

```go
db := engine.GetSQLConnectionByName("db1")
if db == nil {
    return
}

// Usar GORM directamente
type User struct {
    ID   uint
    Name string
}

var user User
db.DB.First(&user, 1)
```

### Router y Middleware

```go
router := engine.GetRouter()

// Middleware personalizado
router.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Lógica de middleware
        next.ServeHTTP(w, r)
    })
})

// Rutas
router.AddRoute("GET", "/health", healthHandler)
router.AddRoute("POST", "/users", createUserHandler)

// Con middleware específico
router.AddRouteWithMiddleware("GET", "/protected", protectedHandler, authMiddleware)
```

### Configuración por Capas

```go
// Acceder a configuraciones por capa
configs := engine.GetConfigs()

// Repositorios
repos := configs.GetRepositories()

// Casos de uso
useCases := configs.GetUseCases()

// Handlers
handlers := configs.GetHandlers()

// Procesadores batch
processors := configs.GetBatches()
```

## 🔧 Utilidades Avanzadas

### Circuit Breaker

```go
import "github.com/skolldire/go-engine/pkg/utilities/circuit_breaker"

breaker := circuit_breaker.New(circuit_breaker.Config{
    MaxRequests: 5,
    Interval:    time.Minute,
    Timeout:     time.Second * 30,
})

result, err := breaker.Execute(func() (interface{}, error) {
    return riskyOperation()
})
```

### Retry con Backoff

```go
import "github.com/skolldire/go-engine/pkg/utilities/retry_backoff"

retrier := retry_backoff.New(retry_backoff.Config{
    MaxRetries: 3,
    InitialDelay: time.Second,
    MaxDelay: time.Second * 10,
})

err := retrier.Execute(func() error {
    return operation()
})
```

### Task Executor

```go
import "github.com/skolldire/go-engine/pkg/utilities/task_executor"

executor := task_executor.New(task_executor.Config{
    MaxConcurrency: 10,
    QueueSize: 100,
})

executor.Submit(func() {
    // Tarea a ejecutar
})

executor.Wait()
executor.Shutdown()
```

## 🚀 Integración con AWS Lambda

```go
package main

import (
    "context"
    "encoding/json"

    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/skolldire/go-engine/pkg/integration/inbound"
)

func Handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Normalizar evento
    req, err := inbound.NormalizeAPIGatewayEvent(&event)
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 500,
            Body:       `{"error":"failed to normalize"}`,
        }, nil
    }

    // Procesar request
    response := map[string]interface{}{
        "message": "success",
    }

    body, _ := json.Marshal(response)
    return events.APIGatewayProxyResponse{
        StatusCode: 200,
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        Body: string(body),
    }, nil
}

func main() {
    lambda.Start(Handler)
}
```

## 📖 Estructura de Proyecto Recomendada

```
project/
├── cmd/
│   └── api/
│       └── main.go
├── config/
│   ├── application.yaml
│   └── features.yaml
├── internal/
│   ├── domain/
│   │   └── user.go
│   ├── repository/
│   │   └── user_repository.go
│   ├── usecase/
│   │   └── user_usecase.go
│   └── handler/
│       └── user_handler.go
├── pkg/
│   └── (utilidades compartidas)
├── go.mod
├── go.sum
└── README.md
```

## 🎯 Mejores Prácticas

1. **Usar el Builder Pattern**: Siempre usa `NewAppBuilder()` para construir aplicaciones
2. **Manejar Errores**: Verifica `engine.GetErrors()` después de la inicialización
3. **Graceful Shutdown**: Implementa manejo de señales para cierre controlado
4. **Feature Flags**: Usa feature flags para controlar funcionalidades en producción
5. **Logging Estructurado**: Usa el logger del engine para logging consistente
6. **Configuración Externa**: Mantén configuraciones sensibles fuera del código
7. **Múltiples Clientes**: Usa el sistema de registro para múltiples instancias del mismo cliente
8. **Validación**: Usa el validador del engine para validar datos de entrada

## 📝 Licencia

Este proyecto está bajo la licencia MIT.

## 🤝 Contribuir

Las contribuciones son bienvenidas. Por favor, abre un issue o pull request.

## 📞 Soporte

Para soporte, abre un issue en GitHub.

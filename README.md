# Go Engine

![GitHub Release](https://img.shields.io/github/v/release/skolldire/go-engine)
![Go Version](https://img.shields.io/github/go-mod/go-version/skolldire/go-engine)

## Descripción

Go Engine es un framework ligero para desarrollar aplicaciones en Go siguiendo los principios de Clean Architecture. Facilita la creación de aplicaciones modulares, testables y mantenibles mediante un patrón de construcción estructurado.

## Características

- **Patrón Builder**: Construcción flexible de aplicaciones mediante un enfoque paso a paso
- **Inyección de dependencias**: Gestión clara de dependencias entre componentes
- **Configuración centralizada**: Sistema basado en Viper para manejar configuraciones
- **Logging integrado**: Sistema de logging basado en Logrus con formato ECS
- **Gestión de errores**: Captura y propagación de errores durante la inicialización
- **Cierre controlado**: Manejo de señales del sistema para un apagado graceful
- **Integración con AWS**: Soporte nativo para servicios como DynamoDB, SQS, SNS
- **Conectores de base de datos**: Integraciones con Redis, SQL (vía GORM) y DynamoDB

## Instalación

```bash
go get github.com/skolldire/go-engine
```
## Uso Básico

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
    // Crear contexto con cancelación
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Configurar manejo de señales para cierre controlado
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
    go func() {
        <-sigCh
        fmt.Println("Señal de cierre recibida")
        cancel()
    }()

    // Crear aplicación usando el builder
    aplicacion := app.NewAppBuilder().
        WithContext(ctx).
        WithMiddleware().
        WithGracefulShutdown().
        Build()

    // Verificar errores durante la inicialización
    if errs := aplicacion.GetErrors(); len(errs) > 0 {
        fmt.Printf("Error al inicializar la aplicación: %v\n", errs[0])
        os.Exit(1)
    }

    // Ejecutar la aplicación
    fmt.Println("Iniciando la aplicación...")
    if err := aplicacion.Run(); err != nil {
        fmt.Printf("Error en la aplicación: %v\n", err)
        os.Exit(1)
    }
}
```

### Aplicación Completa con Estructura Clean Architecture

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/skolldire/go-engine/pkg/app"
    "miproyecto/internal/handlers"
    "miproyecto/internal/repositories"
    "miproyecto/internal/usecases"
)

// Aplicación personalizada
type Aplicacion struct {
    engine       *app.Engine
    repositories *repositories.Container
    usecases     *usecases.Container
    handlers     *handlers.Container
}

func main() {
    // Crear contexto con cancelación
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Configurar señales
    configurarSenales(cancel)

    // Construir aplicación
    app, err := construirAplicacion(ctx)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }

    // Ejecutar
    if err := app.Run(); err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }
}

func configurarSenales(cancel context.CancelFunc) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        sig := <-sigCh
        fmt.Printf("Señal %v recibida, iniciando cierre controlado\n", sig)
        cancel()
    }()
}

func construirAplicacion(ctx context.Context) (*Aplicacion, error) {
    // Crear engine
    builder := app.NewEngineBuilder()
    builder.SetContext(ctx)

    // Inicializar componentes
    appEngine := builder.LoadConfig().
        InitRepositories().
        InitUseCases().
        InitHandlers().
        InitRoutes().
        Build()

    // Verificar errores
    engineObj, ok := appEngine.(*app.Engine)
    if !ok {
        return nil, fmt.Errorf("tipo de engine inesperado")
    }

    if errs := engineObj.GetErrors(); len(errs) > 0 {
        return nil, errs[0]
    }

    // Construir aplicación personalizada
    aplicacion := &Aplicacion{
        engine: engineObj,
    }

    // Inicializar componentes
    if err := aplicacion.inicializarComponentes(); err != nil {
        return nil, err
    }

    return aplicacion, nil
}

func (a *Aplicacion) inicializarComponentes() error {
    // Implementar inicialización de repositorios, casos de uso y handlers
    return nil
}

func (a *Aplicacion) Run() error {
    return a.engine.Run()
}
```

## Arquitectura
Go Engine da una guia para la implementación de una arquitectura limpia con las siguientes capas:
1. Repositorios: Acceso a datos (bases de datos, servicios externos)
2. Casos de Uso: Lógica de negocio
3. Handlers: Manejo de peticiones HTTP/API
4. Router: Enrutamiento de peticiones
5. Middleware: Procesamiento intermedio de peticiones

## Configuración
Go Engine utiliza archivos de configuración en formato YAML. Ejemplo:
```yaml
log:
  level: "info"
  path: "logs/app.log"

router:
  port: 8080
  timeout: 30

aws:
  region: "us-east-1"

dynamo:
  endpoint: "http://localhost:4566"
  table_prefix: "dev_"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0

database_sql:
  driver: "postgres"
  host: "localhost"
  port: 5432
  dbname: "mydatabase"
  username: "user"
  password: "password"
  ssl_mode: "disable"

sqs:
  endpoint: "http://localhost:4566"
  wait_time: 20

sns:
  endpoint: "http://localhost:4566"
  topic_prefix: "dev_"

rest:
  - api1:
      base_url: "https://api1.example.com"
      timeout: 30
      headers:
        Content-Type: "application/json"
  - api2:
      base_url: "https://api2.example.com"
      timeout: 15
      headers:
        Authorization: "Bearer token"

middleware:
  cors:
    enabled: true
    allowed_origins: ["*"]
    allowed_methods: ["GET", "POST", "PUT", "DELETE"]
  
repositories:
  # Configuración específica de repositorios

cases:
  # Configuración específica de casos de uso

endpoints:
  # Configuración específica de endpoints

processors:
  # Configuración de procesadores batch
```
## Estructura de Proyecto Recomendada
```batch
├── .env
├── .github
│   ├── pull_request_template.md
│   └── workflows
│       ├── ci.yml
│       ├── makefile.yml
│       └── release.yml
├── .gitignore
├── .golangci.yml
├── Dockerfile
├── Makefile
├── README.md
├── cmd
│   └── api
│       ├── main.go
│       └── routes
│           └── ping
│               ├── entity.go
│               └── service.go
├── config
│   └── application.yaml
├── docker-compose.yml
├── docs
│   ├── docs.go
│   ├── swagger.json
│   └── swagger.yaml
├── go.mod
├── go.sum
├── init.sh
├── internal
│   ├── description.go
│   └── platform
│       └── description.go
├── kit
│   └���─ description.go
├── pkg
│   └── description.go
└── scripts
    └── description.sh
```
## Clientes Integrados
Go Engine incluye clientes para conectarse a varios servicios:

* REST: Cliente HTTP para APIs REST
* DynamoDB: Cliente para Amazon DynamoDB
* SQS: Cliente para Amazon Simple Queue Service
* SNS: Cliente para Amazon Simple Notification Service
* Redis: Cliente para Redis
* SQL: Cliente SQL vía GORM
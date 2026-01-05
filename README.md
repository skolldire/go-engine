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
- **Sistema de Plugins**: Registro de clientes personalizados mediante el sistema de registry
- **BaseClient**: Cliente base reutilizable con logging y resiliencia integrados
- **API Mejorada**: Getters para acceso controlado a recursos de la aplicación

## Nuevas Mejoras

Para más información sobre las mejoras arquitectónicas recientes, consulta [MEJORAS.md](./MEJORAS.md).

### Principales Mejoras Implementadas

1. **BaseClient**: Cliente base que elimina duplicación de código en todos los clientes
2. **Registry System**: Sistema de registro para clientes personalizados
3. **AppBuilder Completo**: Builder pattern fluido y completo para construcción de aplicaciones
4. **Mejor Encapsulación**: Getters para acceso controlado a recursos
5. **Facilidad de Extensión**: Agregar nuevos clientes es más simple usando BaseClient

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
    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithConfigs().
        WithInitialization().
        WithRouter().
        WithGracefulShutdown().
        Build()

    if err != nil {
        fmt.Printf("Error al construir la aplicación: %v\n", err)
        os.Exit(1)
    }

    // Verificar errores durante la inicialización
    if errs := engine.GetErrors(); len(errs) > 0 {
        fmt.Printf("Error al inicializar la aplicación: %v\n", errs[0])
        os.Exit(1)
    }

    // Ejecutar la aplicación
    fmt.Println("Iniciando la aplicación...")
    if err := engine.Run(); err != nil {
        fmt.Printf("Error en la aplicación: %v\n", err)
        os.Exit(1)
    }
}
```

### Aplicación Completa con Configuración Dinámica y Feature Flags

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/skolldire/go-engine/pkg/app"
    "github.com/skolldire/go-engine/pkg/app/router"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
    go func() {
        <-sigCh
        fmt.Println("shutdown signal received")
        cancel()
    }()

    engine, err := app.NewAppBuilder().
        WithContext(ctx).
        WithDynamicConfig().
        WithInitialization().
        WithRouter().
        WithMiddleware(setupCustomMiddleware).
        WithGracefulShutdown().
        Build()

    if err != nil {
        fmt.Printf("failed to build application: %v\n", err)
        os.Exit(1)
    }

    if errs := engine.GetErrors(); len(errs) > 0 {
        fmt.Printf("initialization errors: %v\n", errs)
        os.Exit(1)
    }

    setupRoutes(engine)

    fmt.Println("starting application...")
    if err := engine.Run(); err != nil {
        fmt.Printf("application error: %v\n", err)
        os.Exit(1)
    }
}

func setupCustomMiddleware(r router.Service) {
    r.Use(func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
            start := time.Now()
            next.ServeHTTP(w, req)
            duration := time.Since(start)
            fmt.Printf("request completed in %v\n", duration)
        })
    })
}

func setupRoutes(engine *app.Engine) {
    r := engine.GetRouter()

    r.AddRoute("GET", "/health", healthCheckHandler(engine))
    r.AddRoute("GET", "/feature-flags", featureFlagsHandler(engine))
}

func healthCheckHandler(engine *app.Engine) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, `{"status":"ok","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
    }
}

func featureFlagsHandler(engine *app.Engine) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        flags := engine.GetFeatureFlags()
        if flags == nil {
            http.Error(w, "feature flags not available", http.StatusServiceUnavailable)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, `{"flags":%v}`, flags.GetAll())
    }
}
```

### Uso de Múltiples Clientes

```go
package main

import (
    "context"
    "fmt"

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

    // Usar múltiples clientes REST
    api1Client := engine.GetRestClient("api1")
    api2Client := engine.GetRestClient("api2")
    
    if api1Client != nil {
        fmt.Println("✓ API1 client available")
    }
    if api2Client != nil {
        fmt.Println("✓ API2 client available")
    }

    // Usar múltiples clientes SQS
    queue1Client := engine.GetSQSClientByName("queue1")
    queue2Client := engine.GetSQSClientByName("queue2")
    
    if queue1Client != nil {
        fmt.Println("✓ Queue1 client available")
    }
    if queue2Client != nil {
        fmt.Println("✓ Queue2 client available")
    }

    // Usar múltiples clientes Redis
    cache1Client := engine.GetRedisClientByName("cache1")
    cache2Client := engine.GetRedisClientByName("cache2")
    
    if cache1Client != nil {
        fmt.Println("✓ Cache1 client available")
    }
    if cache2Client != nil {
        fmt.Println("✓ Cache2 client available")
    }

    engine.Run()
}
```

### Uso de Feature Flags

```go
package main

import (
    "context"
    "fmt"

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

    flags := engine.GetFeatureFlags()
    if flags == nil {
        fmt.Println("feature flags not available")
        return
    }

    // Verificar flags booleanos
    if flags.IsEnabled("enable_new_api") {
        fmt.Println("✓ New API is enabled")
        useNewAPI(ctx)
    } else {
        fmt.Println("✗ New API is disabled, using legacy API")
        useLegacyAPI(ctx)
    }

    // Obtener valores string
    apiVersion := flags.GetString("api_version")
    fmt.Printf("API Version: %s\n", apiVersion)

    // Obtener valores integer
    maxRetries := flags.GetInt("max_retries")
    fmt.Printf("Max Retries: %d\n", maxRetries)

    // Obtener todos los flags
    allFlags := flags.GetAll()
    for key, value := range allFlags {
        fmt.Printf("%s: %v\n", key, value)
    }

    // Actualizar flags dinámicamente
    flags.Set("enable_new_api", true)
    fmt.Println("Updated enable_new_api to true")
}

func useNewAPI(ctx context.Context) {
    fmt.Println("→ Using new API implementation")
}

func useLegacyAPI(ctx context.Context) {
    fmt.Println("→ Using legacy API implementation")
}
```

### Integración con AWS Lambda (API Gateway Handler)

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/skolldire/go-engine/pkg/integration/aws"
    "github.com/skolldire/go-engine/pkg/integration/inbound"
)

type RequestPayload struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

func Handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Normalizar evento API Gateway
    req, err := inbound.NormalizeAPIGatewayEvent(&event)
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 500,
            Body:       `{"error":"failed to normalize event"}`,
        }, nil
    }

    // Parsear body
    var payload RequestPayload
    if err := json.Unmarshal(req.Body, &payload); err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 400,
            Body:       `{"error":"invalid request body"}`,
        }, nil
    }

    // Crear cliente AWS
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 500,
            Body:       `{"error":"failed to load AWS config"}`,
        }, nil
    }
    client := aws.New(cfg)

    // Enviar notificación a SNS
    topicARN := "arn:aws:sns:us-east-1:123456789:notifications"
    notification := map[string]interface{}{
        "type":  "user_registered",
        "name":  payload.Name,
        "email": payload.Email,
    }

    _, err = aws.SNSPublish(ctx, client, topicARN, notification)
    if err != nil {
        log.Printf("failed to publish notification: %v", err)
    }

    // Retornar respuesta
    response := map[string]interface{}{
        "message": "User registered successfully",
        "name":    payload.Name,
    }

    responseBody, _ := json.Marshal(response)
    return events.APIGatewayProxyResponse{
        StatusCode: 200,
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        Body: string(responseBody),
    }, nil
}

func main() {
    lambda.Start(Handler)
}
```

### Aplicación Completa con Estructura Clean Architecture

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/skolldire/go-engine/pkg/app"
	"github.com/skolldire/miapp/internal/handlers"
)

func main() {
	// Crear una nueva instancia de la aplicación
	aplicacion := app.NewApp()

	// Configurar el contexto (opcional)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aplicacion = aplicacion.SetContext(ctx)

	// Cargar configuraciones
	aplicacion = aplicacion.GetConfigs()
	if len(aplicacion.Engine.GetErrors()) > 0 {
		log.Fatalf("Error al cargar configuración: %v", aplicacion.Engine.GetErrors())
	}

	// Inicializar servicios externos (bases de datos, clientes HTTP, etc.)
	aplicacion = aplicacion.Init()
	if len(aplicacion.Engine.GetErrors()) > 0 {
		log.Fatalf("Error al inicializar servicios: %v", aplicacion.Engine.GetErrors())
	}

	// Inicializar el router
	aplicacion = aplicacion.InitializeRouter()

	// Configurar middleware común
	aplicacion = configurarMiddleware(aplicacion)

	// Configurar cierre controlado
	aplicacion = configurarCierreControlado(aplicacion)

	// Registrar rutas específicas de la aplicación
	registrarRutas(aplicacion)

	// Construir y ejecutar la aplicación
	Engine := aplicacion.Build()

	if err := Engine.Run(); err != nil {
		log.Fatalf("Error al iniciar la aplicación: %v", err)
	}
}

func configurarMiddleware(a *app.App) *app.App {
	// Añadir middleware para todas las rutas
	a.Engine.Router.Use(middlewareLogger(a.Engine.Log))
	a.Engine.Router.Use(middlewareCORS())

	return a
}

func configurarCierreControlado(a *app.App) *app.App {
	go func() {
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-signalChan

		a.engine.Log.Infof("Señal recibida: %s, iniciando cierre controlado", sig.String())

		// Tiempo máximo para cierre
		timeout := 30 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Cerrar conexiones
		if a.engine.RedisClient != nil {
			a.engine.RedisClient.Close()
		}

		if a.engine.SqlConnection != nil {
			a.engine.SqlConnection.Close()
		}

		a.engine.Log.Info("Cierre controlado completado")
		os.Exit(0)
	}()

	return a
}

func registrarRutas(a *app.App) {
	usuarioHandler := handlers.NewUsuarioHandler(
		a.engine.SqlConnection,
		a.engine.Log,
	)

	// Registrar rutas
	router := a.engine.Router

	// Grupo de rutas API
	api := router.Group("/api")
	{
		// Rutas de usuarios
		usuarios := api.Group("/usuarios")
		{
			usuarios.GET("", usuarioHandler.ListarUsuarios)
			usuarios.GET("/:id", usuarioHandler.ObtenerUsuario)
			usuarios.POST("", usuarioHandler.CrearUsuario)
			usuarios.PUT("/:id", usuarioHandler.ActualizarUsuario)
			usuarios.DELETE("/:id", usuarioHandler.EliminarUsuario)
		}

		// Otras rutas...
	}

	// Ruta de health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
}

// middlewareLogger crea un middleware para registrar información sobre cada solicitud HTTP
func middlewareLogger(log logger.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			path := r.URL.Path

			// Wrapper para capturar el código de estado
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Procesa la solicitud
			next.ServeHTTP(ww, r)

			// Después de procesar
			latency := time.Since(start)
			statusCode := ww.Status()
			if statusCode == 0 {
				statusCode = http.StatusOK // Por defecto 200 si no se establece
			}

			// Registrar en el log
			log.Infof("| %3d | %13v | %15s | %-7s %s",
				statusCode,
				latency,
				r.RemoteAddr,
				r.Method,
				path,
			)
		})
	}
}

// middlewareCORS configura las opciones CORS para la aplicación
func middlewareCORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Requested-With"},
		ExposedHeaders:   []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           int((12 * time.Hour).Seconds()),
	})
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
│   └─ description.go
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
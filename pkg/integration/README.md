# AWS Integration Layer

Una capa de integración HTTP-like para servicios AWS que simplifica el trabajo con AWS SDK, proporcionando una API consistente y familiar.

## Características

- ✅ **API HTTP-like**: `Do(ctx, req) -> (resp, error)` - familiar para todos los desarrolladores
- ✅ **Normalización de eventos**: Convierte eventos Lambda (SQS, SNS, APIGateway) a Requests normalizados
- ✅ **Observabilidad integrada**: Logging, métricas y tracing opcionales via middleware
- ✅ **Errores normalizados**: Manejo consistente de errores con códigos y flags retriables
- ✅ **Zero-config defaults**: Funciona inmediatamente con valores por defecto sensatos
- ✅ **Coexistencia pacífica**: Puede usarse junto con los clientes AWS existentes sin breaking changes

## Estructura

```
pkg/integration/
├── cloud/          # API estable (Client, Request, Response, Error, Middleware)
├── aws/            # Implementación AWS (New, adapters, helpers)
├── inbound/        # Normalización de eventos Lambda
└── observability/  # Middleware de observabilidad (opcional)
```

## Quick Start

### Instalación

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/skolldire/go-engine/pkg/integration/aws"
    "github.com/skolldire/go-engine/pkg/integration/cloud"
)
```

### Uso Básico

```go
// 1. Crear cliente (zero-config)
cfg, _ := config.LoadDefaultConfig(ctx)
client := aws.New(cfg)

// 2. Enviar mensaje a SQS (usando helper)
msgID, err := aws.SQSSend(ctx, client, "my-queue", map[string]string{
    "order_id": "12345",
    "status":   "created",
})
if err != nil {
    log.Fatal(err)
}

// 3. O usar Do() directamente para más control
req := &cloud.Request{
    Operation: "sqs.send",
    Path:      "my-queue",
}
req.WithJSONBody(map[string]string{"key": "value"})

resp, err := client.Do(ctx, req)
if err != nil {
    log.Fatal(err)
}
```

### Con Observabilidad

```go
import (
    "github.com/skolldire/go-engine/pkg/integration/aws"
    "github.com/skolldire/go-engine/pkg/integration/observability"
    "github.com/skolldire/go-engine/pkg/utilities/logger"
    "github.com/skolldire/go-engine/pkg/utilities/telemetry"
)

// Crear cliente con observabilidad
logger := logger.NewService(...)
telemetry := telemetry.NewTelemetry(...)
metricsRecorder := observability.NewTelemetryMetricsRecorder(telemetry)

client := aws.NewWithOptions(cfg, aws.WithObservability(
    logger,
    metricsRecorder,
    telemetry,
))

// Ahora todas las operaciones tienen logging, métricas y tracing automáticos
msgID, err := aws.SQSSend(ctx, client, "my-queue", payload)
```

### Normalización de Eventos Lambda

```go
import (
    "encoding/json"
    "github.com/aws/aws-lambda-go/events"
    "github.com/skolldire/go-engine/pkg/integration/inbound"
    "github.com/skolldire/go-engine/pkg/integration/aws"
)

func HandleSQSEvent(ctx context.Context, event events.SQSEvent) error {
    // Normalizar evento SQS a Requests
    requests, err := inbound.NormalizeSQSEvent(&event)
    if err != nil {
        return err
    }

    client := aws.New(awsConfig)

    for _, req := range requests {
        // Parsear body desde bytes
        var payload MyPayload
        if err := json.Unmarshal(req.Body, &payload); err != nil {
            return err
        }

        // Procesar payload...
        result := processPayload(ctx, payload)

        // Invocar Lambda usando helper
        _, err := aws.LambdaInvoke(ctx, client, "my-function", result)
        if err != nil {
            return err
        }
    }
    return nil
}
```

## Operaciones Soportadas

### SQS
- `sqs.send` - Enviar mensaje
- `sqs.receive` - Recibir mensajes
- `sqs.delete` - Eliminar mensaje

### SNS
- `sns.publish` - Publicar mensaje

### Lambda
- `lambda.invoke` - Invocar función

## Helpers Disponibles

```go
// SQS
msgID, err := aws.SQSSend(ctx, client, queueURL, payload)
msgID, err := aws.SQSSendBytes(ctx, client, queueURL, []byte("raw message"))

// SNS
msgID, err := aws.SNSPublish(ctx, client, topicARN, payload)

// Lambda
resp, err := aws.LambdaInvoke(ctx, client, functionName, payload)
```

## Integración con Engine

El `CloudClient` está disponible opcionalmente en el Engine:

```go
engine := app.NewApp().
    GetConfigs().
    Init().
    Build()

// Usar CloudClient del Engine
client := engine.GetCloudClient()
if client != nil {
    msgID, err := aws.SQSSend(ctx, client, "my-queue", payload)
}
```

## Middleware de Observabilidad

### Logging

```go
client := aws.NewWithOptions(cfg, aws.Options{
    Middlewares: []cloud.Middleware{
        observability.Logging(logger),
    },
})
```

### Metrics

```go
metricsRecorder := observability.NewTelemetryMetricsRecorder(telemetry)
client := aws.NewWithOptions(cfg, aws.Options{
    Middlewares: []cloud.Middleware{
        observability.Metrics(metricsRecorder),
    },
})
```

### Tracing

```go
client := aws.NewWithOptions(cfg, aws.Options{
    Middlewares: []cloud.Middleware{
        observability.Tracing(telemetry),
    },
})
```

### Todos juntos

```go
client := aws.NewWithOptions(cfg, aws.WithObservability(
    logger,
    metricsRecorder,
    telemetry,
))
```

## Manejo de Errores

Los errores están normalizados con códigos y flags retriables:

```go
resp, err := client.Do(ctx, req)
if err != nil {
    if cloudErr, ok := err.(*cloud.Error); ok {
        switch cloudErr.Code {
        case cloud.ErrCodeThrottling:
            // Retry logic
        case cloud.ErrCodeNotFound:
            // Handle not found
        }
        
        if cloudErr.IsRetriable() {
            // Retry logic
        }
    }
}
```

## Ejemplos Completos

Ver la carpeta `examples/` para ejemplos completos y ejecutables.

## Migración desde Clientes Existentes

La nueva capa puede coexistir con los clientes existentes:

```go
// Código existente sigue funcionando
oldSQSClient := sqs.NewClient(...)
oldSQSClient.SendJSON(...)

// Nuevo código usa la nueva capa
client := aws.New(cfg)
aws.SQSSend(ctx, client, queueURL, payload)

// Migrar gradualmente, servicio por servicio
```

## Más Información

- [Plan de Implementación](../PLAN_AWS_INTEGRATION_LAYER.md)
- [Análisis de Integración](../ANALISIS_INTEGRACION_VS_LIB_NUEVA.md)


# Cliente REST para web-simplify

Este paquete proporciona una implementación de un cliente REST en Go, diseñado para manejar solicitudes HTTP de manera eficiente, incluyendo reintentos automáticos, manejo de errores y un Circuit Breaker basado en gobreaker/v2.

## Características

* Circuit Breaker: Implementado con gobreaker/v2 para evitar fallos catastróficos en caso de errores continuos.
* Reintentos Automáticos: Utiliza resty con backoff exponencial y jitter para gestionar reintentos en errores transitorios.
* Configuración Flexible: Permite personalizar el número de reintentos, umbrales de error y tiempos de espera.
* Soporte para Métodos HTTP: Implementa GET, POST, PUT, PATCH, y DELETE.
* Manejo de Errores Avanzado: Registra errores HTTP 5xx y errores de red con logs detallados.

## Instalación
```sh
go get github.com/skolldire/web-simplify/pkg/client/rest
````

## Configuración

El cliente REST se configura mediante la estructura Config:

import "github.com/skolldire/web-simplify/pkg/client/rest"

```go
cfg := rest.Config{
    BaseURL:            "https://api.ejemplo.com",
    WithRetry:          true,
    RetryCount:         3,
    RetryWaitTime:      100 * time.Millisecond,
    RetryMaxWaitTime:   500 * time.Millisecond,
    WithCB:             true,
    CBName:             "client_rest_cb",
    CBMaxRequests:      5,
    CBInterval:         10 * time.Second,
    CBTimeout:          5 * time.Second,
    CBRequestThreshold: 5,
    CBFailureRateLimit: 0.5, 
}

client := rest.NewClient(cfg, logger) // logger debe ser una implementación de log.Service
```
## Uso del Cliente REST

### Realizar una solicitud GET
```go
response, err := client.Get(ctx, "/users")
if err != nil {
log.Fatal(err)
}
fmt.Println("Response:", response)
```
### Enviar una solicitud POST con JSON
```go
data := map[string]interface{}{"name": "John Doe", "email": "john@example.com"}
response, err := client.Post(ctx, "/users", data)
if err != nil {
log.Fatal(err)
}
fmt.Println("Response:", response)
```

## Pruebas Unitarias

Ejecutar pruebas con:
```sh
go test ./pkg/client/rest -v
```
* Las pruebas incluyen:
* Simulación de errores HTTP 5xx
* Activación del Circuit Breaker
* Verificación de reintentos en errores transitorios

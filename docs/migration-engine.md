# Migración: `pkg/app` → `pkg/engine`

`pkg/engine` es el núcleo desacoplado introducido en la Fase C del
[plan de auditoría](plan-auditoria-2026-08.md). `pkg/app` sigue funcionando y
sigue estando testeado; esta guía existe para migrar cuando quieras, no porque
algo se haya roto.

## Por qué

Medido con `go list -deps` sobre este checkout:

| Punto de entrada | Módulos externos | Paquetes |
|---|---:|---:|
| `pkg/app` (antiguo) | **110** | **547** |
| `pkg/engine` (núcleo) | **19** | **41** |
| `preset/http` (servicio HTTP puro) | **19** | **41** |
| `preset/aws` (solo AWS) | **28** | **177** |
| `preset/full` (todo, equivalente a `pkg/app`) | 109 | 543 |

Un servicio HTTP que antes arrastraba el SDK completo de AWS, el driver de
MongoDB, Kafka, RabbitMQ y una librería de códigos QR ahora resuelve 19 módulos.

## El cambio de forma

Antes, el `Engine` tenía un campo y un getter por adaptador, y `viper.Config`
nombraba los tipos concretos de los 15. Añadir un adaptador exigía tocar cuatro
ficheros del núcleo.

Ahora el núcleo no conoce ningún adaptador. Cada uno implementa
`engine.Provider` en su propio paquete y se registra explícitamente.

### Servicio HTTP puro

```go
import (
    "github.com/skolldire/go-engine/pkg/engine"
    presethttp "github.com/skolldire/go-engine/preset/http"
)

eng, err := engine.New(ctx, presethttp.Options()...)
if err != nil { return err }
defer eng.Close(ctx)

eng.Router().AddRoute("GET", "/users", usersHandler)
return eng.Run(ctx)
```

### Añadir componentes: dos imports, dos líneas

```go
import (
    "github.com/skolldire/go-engine/pkg/engine"
    "github.com/skolldire/go-engine/provider/redis"
    "github.com/skolldire/go-engine/provider/sqs"
)

eng, err := engine.New(ctx,
    engine.WithRouter(),
    engine.WithHealth(),
    engine.WithProvider(
        sqs.New("orders"),     // registra su closer automáticamente
        redis.New("cache"),    // y además su health checker
    ),
)
```

### El engine completo de siempre, en una línea

```go
import presetfull "github.com/skolldire/go-engine/preset/full"

eng, err := engine.New(ctx, presetfull.Options()...)
```

El preset descubre las instancias declaradas en el YAML, así que añadir una cola
nueva al fichero no requiere tocar código.

## Equivalencias

| `pkg/app` | `pkg/engine` |
|---|---|
| `NewAppBuilder().WithContext(ctx)` | `engine.New(ctx, ...)` |
| `.WithConfigs()` / `.WithDynamicConfig()` | `engine.WithConfigDir(...)` (por defecto `CONF_DIR` o `./config`) |
| `.WithInitialization()` | `engine.WithProvider(...)` o un preset |
| `.WithRouter()` | `engine.WithRouter()` |
| `.WithHealth(cfg)` | `engine.WithHealthConfig(cfg)` |
| `.WithOTEL(cfg)` | `engine.WithProvider(otel.New())` + `engine.WithTelemetry(...)` |
| `.WithMiddleware(fn)` | `engine.WithMiddleware(fn)` |
| `.WithGracefulShutdown()` | no existe: siempre activo (era un no-op) |
| `.Build()` | lo devuelve `engine.New` |
| `engine.GetSQSClientByName("orders")` | `sqs.From(eng, "orders")` |
| `engine.GetRedisClientByName("cache")` | `redis.From(eng, "cache")` |
| `engine.GetRestClient("api")` | `rest.From(eng, "api")` |
| `engine.GetCognito()` | `cognito.From(eng)` |
| `engine.GetKafkaProducer()` | `kafka.From(eng)` |
| `engine.GetCustomClient(name)` | `engine.Get[T](eng, name)` |
| `engine.Run()` | `eng.Run(ctx)` |
| `engine.Close(ctx)` | `eng.Close(ctx)` |

### Recuperación tipada

Los 39 getters desaparecen porque el tipo viaja con quien llama:

```go
orders, err := sqs.From(eng, "orders")   // sqs.Service
cache,  err := redis.From(eng, "cache")  // *redis.RedisClient

// Y para cualquier componente, incluidos los propios:
thing, err := engine.Get[*MyThing](eng, "mything")
```

Pedir un componente inexistente o con el tipo equivocado devuelve un error que
enumera lo que sí hay registrado, en lugar de un `nil` silencioso.

## Cambios de comportamiento a vigilar

Estos son los puntos donde el mismo YAML **no** significa lo mismo. Ninguno
falla de forma ruidosa por sí solo, así que conviene revisarlos al migrar.

### 1. Claves de instancia única (`sqs:`, `sns:`, `dynamo:`, `redis:`)

El esquema antiguo aceptaba tanto `sqs:` (una sola cola) como `sqs_clients:`
(varias). Los providers solo leen la forma plural, que ya era la recomendada;
la singular queda **ignorada en silencio**.

```yaml
# Antes (sigue funcionando en pkg/app, ignorado por los providers)
sqs:
  endpoint: "http://localhost:4566"

# Ahora
sqs_clients:
  - default:
      endpoint: "http://localhost:4566"
```

### 2. La sección `telemetry:` cambió de nombres

`pkg/utilities/telemetry` usaba `otel_endpoint` y `sample_rate`;
`pkg/telemetry/otel` usa `exporter_endpoint` y `sampling_rate`. Decodificarlos
en silencio dejaría el endpoint vacío y el muestreo a cero **mientras la
telemetría se reporta como habilitada**, así que `provider/otel` **rechaza el
arranque** con un error que nombra la clave antigua y la nueva.

```yaml
telemetry:
  service_name: "my-service"
  exporter_endpoint: "localhost:4317"   # antes: otel_endpoint
  sampling_rate: 0.1                    # antes: sample_rate
  enabled: true
  insecure: true                        # TLS por defecto; opt-out explícito
```

### 3. Secciones sin provider registrado

Una sección declarada en el YAML para la que nadie registró un provider no
construye nada. Es deliberado — es lo que permite que un servicio no pague por
lo que no usa — pero significa que olvidar un `WithProvider` no da error, sino
un componente ausente. `eng.ComponentNames()` lista lo que sí se construyó, y
`engine.Get` falla nombrando lo disponible.

### 4. No existe `WithConfigWatch`

La recarga en caliente exige republicar la configuración a componentes ya
construidos, algo que el contrato `Provider` todavía no expresa. Se dejó **sin
implementar antes que a medias**: una opción que arranca un watcher que nadie
escucha es exactamente el defecto que la auditoría encontró en
`WithDynamicConfig`.

## Orden de las opciones

No hay. `engine.New` aplica los pasos en orden de dependencias, no en orden de
llamada, así que las reglas del builder antiguo («`WithJWTAuth` después de
`WithRouter`», «`WithHealth` después de `WithDynamicConfig`») ya no existen.

## Escribir un adaptador propio

No hace falta modificar el núcleo: implementa `engine.Provider` en tu paquete.

```go
type Provider struct{ instance string; client *MyClient }

func (p *Provider) Name() string      { return "mything:" + p.instance }
func (p *Provider) ConfigKey() string { return "mything_clients" }

func (p *Provider) Init(ctx context.Context, raw engine.RawConfig, deps engine.Deps) (any, error) {
    cfg, err := engine.DecodeNamed[MyConfig](raw, p.instance)
    if err != nil { return nil, err }

    client, err := NewMyClient(cfg)
    if err != nil { return nil, err }
    p.client = client

    deps.Health.RegisterCheck(p.Name(), client.Ping)
    return client, nil
}

func (p *Provider) Close(ctx context.Context) error { return p.client.Close() }
```

`make lint-arch` falla el build si algún fichero de `pkg/engine/**` importa un
provider, un preset o un SDK de adaptador. Es lo que impide que el acoplamiento
vuelva a crecer import a import.

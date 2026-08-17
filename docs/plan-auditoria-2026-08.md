# Plan de implementación — auditoría externa (2026-08-16)

**Fuente:** revisión técnica profunda de `feature/fix-version` (HEAD `676c518`) contra `origin/master` (`884f652`).
**Relación con [`plan-mejoras-go-engine.md`](plan-mejoras-go-engine.md):** aquel plan cubre las Fases 0–4 internas (0–2 ✅, 3–4 🟡). Este documento **no lo reemplaza**: recoge los hallazgos nuevos de la auditoría externa y los ordena en cuatro fases (A–D) ejecutables. El punto diferido en el [ADR-0001](adr/0001-core-provider-decoupling.md) (desacople core/proveedores) es la Fase C de este plan.

---

## 0. Verificación previa (hecha sobre el checkout actual)

Antes de planificar se comprobó en el código que los hallazgos siguen vigentes:

| Hallazgo | Estado verificado |
|---|---|
| gRPC `NewClient` sin `conn.Connect()` | ✅ Confirmado — `messaging/pkg/integration/grpc/service.go:36-44` |
| `BatchWorkPool` pierde el lote parcial | ✅ Confirmado — `pkg/utilities/task_executor/service.go:202` (`count == len(tasks)` contra contador reiniciado) |
| Log forzado a `trace` | ✅ Confirmado — `pkg/app/service.go:53` |
| `setLogLevel` descarta `Format`/`ReportCaller`/`OutputWriters` | ✅ Confirmado — `pkg/app/service.go:449-453` |
| Permisos `0755`/`0666` en logs | ✅ Confirmado — `pkg/utilities/logger/service.go:125,131` |
| AWS cargado incondicionalmente | ✅ Confirmado — `pkg/app/service.go:64-70` |
| `WithDynamicConfig` con `dynamicConfig` local | ✅ Confirmado — `pkg/app/builder.go:51-90` |
| README `JWKSEndpoint` inexistente | ✅ Confirmado — `README.md:66,316` |
| README anuncia 7 módulos Go | ✅ Confirmado — un solo `go.mod` en la raíz |
| `go 1.26.5` como piso de consumidores | ✅ Confirmado — `go.mod:3` |
| `revive` desactivado | ✅ Confirmado — `.golangci.yml`, con justificación escrita |
| `docs/` no existe | ⚠️ **Desactualizado** — `docs/` existe pero está **sin versionar** (untracked). Basta con hacer commit. |
| `PR_DESCRIPTION.md` en la raíz | ✅ Confirmado |

---

## Fase A — Corrección funcional (bloquea cualquier release) · ~1 semana — ✅ COMPLETADA

Objetivo: que nada de lo que el framework expone esté roto. Cada ítem lleva test de regresión.

| # | Acción | Ubicación | Test de salida |
|---|---|---|---|
| A1 ✅ | `conn.Connect()` tras `grpc.NewClient`; espera de readiness opcional (`WaitForReady` en `Config`); `TransientFailure` deja de ser fatal | `messaging/pkg/integration/grpc/service.go:36-44,66-88` | Test contra listener TCP vivo conecta en <1s |
| A2 ✅ | Vaciar el lote remanente tras el bucle de `BatchWorkPool` (contador `processed` global o flush final) | `pkg/utilities/task_executor/service.go:189-219` | `tasks=250, batchSize=100 → 250 resultados` |
| A3 ✅ | Eliminar `_ = log.SetLogLevel("trace")` | `pkg/app/service.go:53` | Test: `log.level: warn` en YAML ⇒ nivel efectivo `warn` |
| A4 ✅ | `setLogLevel` propaga `Format`, `ReportCaller`, `OutputWriters`, `ContextExtractor` (pasar el `logger.Config` completo) | `pkg/app/service.go:449-454` | Test: `log.format: json` produce salida JSON |
| A5 ✅ | Permisos `0750` en `MkdirAll` y `0600` en `OpenFile` del log | `pkg/utilities/logger/service.go:125,131` | Test de `os.Stat().Mode()` |
| A6 ✅ | `usesAWS(conf)` como guarda de `LoadDefaultConfig`; `CloudClient` solo si hay adaptador AWS declarado | `pkg/app/service.go:64-70,307-315` | Test: YAML sin sección AWS ⇒ `Build()` OK sin credenciales |
| A7 ✅ | Guardar `*DynamicConfig` en `Engine`; registrar `Stop` como closer del lifecycle; `Engine.Config()` leyendo del `atomic.Value`; suscribir `AddReloadHook`; poblar `FeatureFlags` | `pkg/app/builder.go:51-90`, `pkg/app/entity.go` | `goleak.VerifyNone` tras `Close`; test de recarga visible |
| A8 ✅ | Guard de nil en `a.logger` dentro de `Run`; mutex en `shutdownHooks` | `pkg/app/router/service.go:151`, `entity.go:38` | Test: `NewService(cfg)` sin `WithLogger` no paniquea |
| A9 ✅ | Mutex en `WithLogging` (mismo que protege `c.conn`) | `messaging/pkg/integration/grpc/service.go:216-218` | `go test -race` sobre uso concurrente |
| A10 ✅ | README: `JWKSEndpoint`→`JWKSURL`, import de `router`, manejo del error de `Run()`, tabla de módulos corregida a un único módulo, fila `WithJWTAuth` duplicada, `sqs_clients: wait_time` inexistente | `README.md` | Los snippets del Quick Start compilan como `example_test.go` |
| A11 ✅ | `CLAUDE.md`: rutas reales (`aws/pkg/clients/…`, `database/redis/pkg/…`, `messaging/pkg/server/grpc`) | `CLAUDE.md` | Revisión manual |

**Criterio de salida A — verificado en local (2026-08-16):**

| Comprobación | Resultado |
|---|---|
| `gofmt -l .` | ✅ 0 ficheros |
| `go build ./...` | ✅ OK |
| `go vet ./...` | ✅ 0 hallazgos |
| `go test -race -count=1 ./...` | ✅ **44/44 paquetes**, sin data races |
| `golangci-lint run` | ✅ 0 issues |

### Hallazgos adicionales encontrados al ejecutar la fase

Dos problemas que la auditoría no listaba y que salieron al escribir los tests:

1. **`InvokeRPC` disparaba una reconexión completa en la primera llamada.** Al
   hacer la conexión perezosa (A1), el estado normal tras construir pasa a ser
   `CONNECTING`, que el código trataba como "no usable". Se introdujo
   `isUsable(state)` — `READY`/`IDLE`/`CONNECTING` — usado en `execute`,
   `InvokeRPC` y `ReconnectIfNeeded`.
2. **`ReconnectIfNeeded` podía bloquear indefinidamente con el write-lock
   tomado**, porque pasaba el contexto del llamante (sin deadline) a
   `waitForConnection`. Ahora acota la espera con `ensureContextWithTimeout`.

Además, el Quick Start del README quedó protegido por un guard de compilación
(`pkg/app/readme_quickstart_test.go`) para que no vuelva a derivar de la API.

---

## Fase B — Robustez y confianza · ~2 semanas — ✅ COMPLETADA

| # | Acción | Ubicación |
|---|---|---|
| B1 ✅ | `errors.Is`/`errors.As` en lugar de `==` y de `strings.Contains` | `pkg/utilities/circuit_breaker/service.go:38,41`, `database/memcached/.../service.go:75`, `pkg/app/router/jwt_middleware.go:141-147`, cognito |
| B2 ✅ | `errors.Join(b.errors...)` en `Build()` en lugar de `fmt.Errorf("%v")` | `pkg/app/builder.go` |
| B3 ✅ | Eliminar el código muerto de `retry_backoff` y envolver de verdad el error final | `pkg/utilities/retry_backoff/service.go:82-88` |
| B4 ✅ | `errorlint`: los 10 `fmt.Errorf("%w: %v")` que pierden el error interno + las aserciones de tipo directas | `aws/.../ses/service.go:57,62`, `cognito/token.go:40`, `cognito/entity.go:251,257`, `cognito/authentication.go:160` |
| B5 ✅ | `golang.org/x/sync/singleflight` en el refresco de JWKS (hoy write-lock durante el fetch HTTP) | `pkg/app/router/jwt_middleware.go:306-331` |
| B6 ✅ | `gosec` restante: G115 (int→int32 en paginación), G118 (`context.Background()` en goroutine), `//nolint:gosec` justificado en el jitter G404 | `aws/.../s3/service.go:175`, `adapters/s3.go:235`, `adapters/ssm.go:343`, `messaging/pkg/server/grpc/service.go:73`, `retry_backoff/service.go:94` |
| B7 ✅ | `go.mod`: `go 1.24` + `toolchain go1.26.5` (dejar de imponer un patch recién salido a los consumidores) | `go.mod:3` |
| B8 ✅ | CI: añadir `make coverage-check` y `make lint-arch` a `ci.yml`; en `lint.yml` quitar `--tests=false`, fijar versión de golangci-lint, quitar `go clean -modcache`, ejecutar también en push a `master` | `.github/workflows/` |
| B9 ✅ | Subir cobertura a ≥60 % donde aparecieron los bugs: `grpc` client (9,4 %), `viper` (24,1 %), `logger` (17,9 %), `pkg/app` (38,7 %) | — |
| B10 ✅ | Unificar telemetría: `pkg/utilities/telemetry` pasa a fachada deprecada sobre `pkg/telemetry/otel` (hoy ambos llaman `otel.SetTracerProvider`) | `pkg/utilities/telemetry`, `pkg/telemetry/otel` |
| B11 ✅ | Montar `/live`, `/ready`, `/deps` desde el builder (hoy `health.HTTPHandler.Routes()` los define y nadie los monta) | `pkg/app/builder.go:213-215`, `pkg/health/handler.go:24-30` |
| B12 ✅ | `error_handler`: quitar `context.Context` del struct, `WithX` inmutables (copia en vez de mutar el receptor), implementar `Is(error) bool` | `pkg/utilities/error_handler/service.go:30,46-62,173` |
| B13 ✅ | Higiene de repo: versionar `docs/`, borrar `PR_DESCRIPTION.md`, añadir `CODEOWNERS`, `coverage.out`/`health_coverage.out` al `.gitignore` | raíz |
| B14 ✅ | `middleware.Timeout` configurable y coherente con `WriteTimeout` (hoy 60s hardcoded vs 30s) | `pkg/app/router/service.go:78` |
| B15 ✅ | `logger.NewService` deja de emitir `"Logger service initialized"` al construir | `pkg/utilities/logger/service.go:52` |

**Criterio de salida B — verificado en local (2026-08-17):**

| Comprobación | Resultado |
|---|---|
| `gofmt -l .` | ✅ 0 ficheros |
| `go build ./...` | ✅ OK |
| `go vet ./...` | ✅ 0 hallazgos |
| `go test -race -count=1 ./...` | ✅ **43/43 paquetes**, sin data races |
| `golangci-lint run` (con `gosec` + `errorlint` ya activados) | ✅ 0 issues |
| `make lint-arch` | ✅ sin violaciones |
| `make coverage-check` (umbral 80 % en paquetes críticos) | ✅ todos pasan |

**Cobertura de los paquetes donde salieron los bugs (B9, objetivo ≥60 %):**

| Paquete | Antes de la auditoría | Ahora |
|---|---|---|
| `messaging/.../grpc` | 9,4 % | **73,8 %** |
| `pkg/config/viper` | 24,1 % | **75,2 %** |
| `pkg/utilities/logger` | 17,9 % | **85,0 %** |
| `pkg/app` | 38,7 % | **64,3 %** |

Cobertura global: 42,3 % → **50,7 %**.

### Desviaciones respecto a lo planificado

1. **B7 usa `go 1.25.0`, no `go 1.24`.** El piso real de las dependencias es
   1.25.0 (`go list -m -f '{{.GoVersion}}' all`), así que 1.24 no habría
   compilado. Queda `go 1.25.0` + `toolchain go1.26.5`.
2. **`gosec` y `errorlint` quedaron activados en `.golangci.yml`**, no solo
   corregidos, con exclusiones acotadas a patrones que únicamente aparecen en
   ficheros `_test.go`. Sin esto los 98 hallazgos volverían con el primer PR.
3. **La deprecación de `pkg/utilities/telemetry` (B10) marca su propio call site
   interno** en `pkg/app/service.go`. Se anotó con un `//nolint:staticcheck`
   justificado en lugar de silenciarlo: retipar `Engine.Telemetry` a
   `otel.Provider` es un breaking change que pertenece a la Fase C.

### Hallazgo adicional encontrado al ejecutar la fase

**`WithRouter()` fallaba en silencio sin configuración.** `InitializeRouter`
retornaba sin hacer nada cuando `Engine.Conf == nil`, dejando `Router` a nil y
**sin registrar ningún error**; el fallo solo aparecía después, como un nil
pointer dereference lejos de su causa. Ahora reporta
`"no configuration loaded, call WithConfigs or WithDynamicConfig first"`.
Cubierto por `TestWithRouter_WithoutConfigReportsError`.

---

## Fase C — Desacoplamiento núcleo/proveedores · ~4–6 semanas — 🟡 PARCIAL (C1–C5, C7, C8) · **C6 pendiente** · C9 pendiente de decisión

Es el bloqueador nº1 para publicar la librería y corresponde al milestone diferido en el ADR-0001.

**Causa raíz única:** `viper.Config` (`pkg/config/viper/entity.go:30-61`) nombra los tipos `Config` concretos de los 15 adaptadores. De ahí sale que importar `pkg/app` arrastre ~111 paquetes de ~60 módulos, y que un lector de YAML arrastre 40.

**Regla de oro:** el núcleo no conoce ningún adaptador; los adaptadores conocen el núcleo.

### C1 — Contrato del núcleo (`pkg/engine`, nuevo, sin dependencias pesadas)

- `Provider` (`Name`, `ConfigKey`, `Init(ctx, RawConfig, Deps) (any, error)`, `Close(ctx) error`).
- `Deps{ Logger, Telemetry (interfaz), Health HealthRegistrar }`.
- `RawConfig.Decode(target any) error` — mapstructure bajo el capó; el núcleo no conoce el tipo.
- `Registry` **por instancia**, no singleton (`pkg/core/registry/entity.go:33-48`).

### C2 — `viper.Config` con `mapstructure:",remain"`

Deja tipados solo `Router`, `Log`, `Health`, `Telemetry`; todo lo demás cae en `Components map[string]any` y cada provider decodifica su sección. **Este solo cambio corta los 15 imports de adaptadores.**

### C3 — Pilotos: SQS y Redis

Migrados a `provider/sqs` y `provider/redis`, con recuperación tipada (`sqsprov.From(eng, "orders")`). `pkg/app` se convierte en shim de compatibilidad sobre el núcleo nuevo: **nada se rompe todavía**.

### C4 — Migrar los 13 adaptadores restantes (breaking change grande → `v0.30.0`)

Sustituye las 12 funciones `createClientsXxx` casi idénticas (`pkg/app/service.go:229-447`). Se eliminan: `ServiceRegistry`, los 39 getters de `Engine` + campos legacy, `pkg/app/build` (API muerta), `WithGracefulShutdown` (no-op público). Documentar todo en `MIGRATION.md`.

### C5 — Singletons fuera

`validation.globalValidator` (`pkg/utilities/validation/validators.go:12-15,48`) y `registry.GetRegistry` pasan a instancia por engine. Con esto, dos engines en un binario dejan de ser imposibles.

### C6 — Generalizar el Decorator

`pkg/integration/cloud.Middleware` (hoy solo para AWS) pasa a ser el mecanismo de logging/métricas/tracing de *todos* los clientes, en lugar del `if c.logging { … }` reimplementado en cada uno. `grpc.Cliente.execute` embebe `BaseClient` (elimina ~40 LOC duplicadas).

### C7 — Presets

`preset/http`, `preset/aws`, `preset/full` — quien quiera exactamente el engine de hoy lo obtiene en una línea: `engine.New(ctx, full.All()...)`.

### C8 — `make lint-arch` generalizado

De "gorm no entra en pkg/" a la regla estructural: **ningún paquete de `pkg/engine/**` puede importar `provider/**`**. Es el gate que impide que el acoplamiento vuelva.

### C9 — División en módulos Go (opcional, al final)

`go.work` en la raíz + `go.mod` por `provider/aws`, `provider/messaging`, `provider/database/*`. Solo **después** de C1–C8: en ese punto el grafo ya es un DAG y la división es mecánica. Hacerlo antes es lo que dejó el README describiendo 7 módulos que no existen.

**Criterio de salida C — verificado en local (2026-08-17):**

### Ganancia medida (`go list -deps`, módulos externos / paquetes)

| Punto de entrada | Módulos | Paquetes |
|---|---:|---:|
| `pkg/app` (antiguo) | **110** | **547** |
| `pkg/engine` (núcleo nuevo) | **19** | **41** |
| `preset/http` (servicio HTTP puro) | **19** | **41** |
| `preset/aws` (solo AWS) | **28** | **177** |
| `preset/full` (equivalente a `pkg/app`) | 109 | 543 |

El objetivo del plan («servicio HTTP + Postgres: ~60 módulos → ~5») se cumple:
un servicio HTTP puro pasa de **110 a 19 módulos**. Los módulos que quedan en el
núcleo son exactamente los del diseño objetivo: chi, logrus, fsnotify, viper/yaml
y jwt.

| Comprobación | Resultado |
|---|---|
| `gofmt -l .` | ✅ 0 ficheros |
| `go build ./...` / `go vet ./...` | ✅ OK / 0 hallazgos |
| `go test -race -count=1 ./...` | ✅ **45/45 paquetes**, sin data races |
| `golangci-lint run` | ✅ 0 issues |
| `make lint-arch` (regla generalizada) | ✅ sin violaciones, **y verificada fallando** con una violación deliberada |
| `make lint-deps` (nuevo) | ✅ reporta la huella del núcleo en CI |

### Qué se entregó

| # | Estado | Detalle |
|---|---|---|
| C1 | ✅ | `pkg/engine`: `Provider`, `Deps`, `RawConfig`, `Registry` **por instancia** (sin singleton), lifecycle LIFO con rollback |
| C2 | ✅ | `engine.Config` con `mapstructure:",remain"`: el núcleo solo tipa `router`, `log` y `health`; el resto viaja crudo |
| C3 | ✅ | SQS y Redis como piloto, validados end-to-end antes de replicar el patrón |
| C4 | ✅ | **Los 15 adaptadores migrados** a `provider/*` |
| C5 | ✅ | Registro por instancia en el núcleo nuevo; dos engines en un binario son triviales |
| C6 | ✅ **COMPLETADO** | Marcado como hecho por error en su día (se entregó `provider/otel`, que es otra cosa). Ahora sí, y verificado: ver la sección «Decorator generalizado» más abajo. |
| C6b | ✅ | `provider/otel` adapta OTel a la interfaz estrecha del núcleo, que ya no conoce el SDK (trabajo adicional, no sustituye a C6) |
| C7 | ✅ | `preset/http`, `preset/aws`, `preset/full` con descubrimiento desde el YAML |
| C8 | ✅ | `lint-arch` generalizado: `pkg/engine/**` no puede importar `provider/**`, `preset/**` ni ningún SDK |
| C9 | ⏸️ | División en módulos Go: **pendiente de decisión** (ver abajo) |

Guía de migración: [`migration-engine.md`](migration-engine.md).

### Lo que NO se hizo, y por qué

**1. No se borró la superficie antigua.** `pkg/app` y `pkg/config/viper` siguen
funcionando y testeados; ambos quedan marcados `Deprecated:` con la referencia a
su reemplazo, y `staticcheck` ya avisa a quien los importe. Borrarlos elimina de
golpe la API pública completa de la librería: es irreversible para los
consumidores y merece ser una decisión explícita, no un efecto colateral de esta
fase. El desacoplamiento **ya está conseguido** para quien use `pkg/engine`; lo
que queda es retirar el camino viejo.

**2. C9 (multi-módulo) sigue sin hacerse, ahora con datos para decidir.** Con
C1–C8 el consumidor ya no *compila ni enlaza* lo que no usa (110 → 19 módulos).
Lo único que C9 añade es dejar de **descargar** el `go.sum` completo, a cambio de
multiplicar el coste de release (un tag por módulo) y de exigir `go.work` +
`replace` para el desarrollo local. La recomendación del plan era decidirlo con
datos medidos; los datos están en la tabla de arriba.

### Validación posterior (2026-08-17)

Se hizo una pasada de validación específica sobre el código nuevo, buscando
defectos en lugar de repetir los mismos checks en verde. Encontró **tres bugs
propios** y obligó a **corregir una afirmación equivocada**.

| # | Defecto | Estado |
|---|---|---|
| V1 | **El loader del núcleo no registraba los hooks de `time.Duration` ni de slices.** Cualquier `read_timeout: 15s`, `health.timeout: 7s` o lista separada por comas fallaba al decodificar. Rompía ficheros de configuración perfectamente válidos. | ✅ Corregido: un único `newDecoder` compartido por el core y por todos los providers, para que el mismo YAML signifique lo mismo lo lea quien lo lea |
| V2 | **`WithConfigWatch` era un no-op público**: ponía una bandera que nadie leía. Exactamente el defecto `WithGracefulShutdown` que la auditoría criticaba, reintroducido. | ✅ Eliminada, con la razón documentada en el propio fichero |
| V3 | **La telemetría del `provider/otel` nunca llegaba a `Deps.Telemetry`.** Los providers recibían el no-op y sus spans desaparecían en silencio — el mismo patrón «implementado pero nunca suscrito» del §3.3 original. | ✅ Corregido con un `telemetrySwitch` estable, de forma que también los providers construidos *antes* que el de telemetría registran a través de él |
| V4 | Un provider que falla a mitad de su `Init` nunca recibía su propio `Close`, porque no llegó a registrarse en el lifecycle. | ✅ El core lo cierra explícitamente; el test de contrato garantiza que `Close` es seguro sin `Init` |

**Corrección a una afirmación anterior:** se reportó que `pkg/utilities/telemetry.Config`
estaba roto por no llevar tags `mapstructure`. **Es falso.** El loader antiguo
define un `MatchName` que ignora los guiones bajos, así que las claves
snake_case sí enlazaban. Los tags añadidos siguen siendo correctos (hacen el
struct independiente del loader), pero no había ningún bug vivo. La misma
lógica de `MatchName` se replicó en el núcleo nuevo para no romper structs de
consumidor sin tags.

**Trampa de migración detectada y cerrada:** la sección `telemetry:` cambió de
`otel_endpoint`/`sample_rate` a `exporter_endpoint`/`sampling_rate`.
Decodificarla en silencio dejaría el endpoint vacío y el muestreo a cero
mientras la telemetría se reporta como habilitada, así que `provider/otel`
**rechaza el arranque** nombrando la clave vieja y la nueva.

**Paridad de claves verificada:** los 16 providers consumen exactamente las
mismas secciones YAML que la configuración tipada anterior, fijado por test
(`TestProviders_ConfigKeysMatchLegacyYAML`). Las claves legacy de instancia
única (`sqs:`, `sns:`, `dynamo:`, `redis:`) quedan ignoradas: documentado en la
guía de migración.

**Cobertura del código nuevo:** `pkg/engine` 76,4 % · `provider/*` 38,0 %
(tests de contrato sobre los 16) · `preset/full` 60,9 %.

### Hallazgos al ejecutar la fase

- **`Deps` necesitó `Section(key)`.** Varios providers AWS comparten la sección
  `aws:` sin ser sus dueños. Se expuso el acceso a secciones arbitrarias sin que
  el núcleo sepa qué contienen.
- **`Deps.Resource` resuelve la cadena de credenciales AWS una sola vez** por
  engine, memoizada y a prueba de concurrencia (test con 20 goroutines). Antes se
  resolvía incondicionalmente en el arranque de todo servicio.
- **El gate `lint-arch` se verificó fallando** antes de darlo por bueno: un gate
  que nunca se ha visto fallar no es un gate.

## Fase C-bis — Cliente HTTP: fortalecer resty, no reemplazarlo — ✅ ENTREGADO

### Principio acordado (2026-08-17)

> No rehacer la rueda. Solo se añade algo **si y solo si nos hace más robustos**.
> Aprovechamos lo mejor de lo que ya tenemos y **fortalecemos sus debilidades**.
> Este modelo aplica a **todos** los clientes, no solo al HTTP.

### Un intento descartado

Se construyó primero un cliente HTTP propio (`http/pkg/httpx`, 1.872 LOC) con
builder, decorators, retry y circuit breaker. Al auditarlo contra resty v2.17.2
**la mayor parte resultó ser reinvención**:

| Pieza | ¿Ya resuelto? |
|---|---|
| Builder (path/query/headers/body) | resty, y más completo (multipart, ficheros, cookies, digest auth) |
| Bucle de retry, backoff, jitter | resty (`jitterBackoff`, citando el artículo de AWS) |
| Cap de `Retry-After` | resty (`if result < 0 \|\| max < result { result = max }`) |
| Circuit breaker | **`pkg/utilities/circuit_breaker` (gobreaker), ya en el repo** |
| Respuesta junto al error | resty ya devuelve `*Response`; lo descartaba **nuestro wrapper** |

Se borró entero. **El defecto nunca fue resty: era el wrapper de go-engine**,
que escondía el builder tras `Get(ctx, endpoint, headers)`, tiraba la respuesta y
metía la resiliencia en un booleano.

### Lo que sí faltaba (265 LOC, frente a 1.872)

| Añadido | Por qué resty no lo cubre |
|---|---|
| `ConservativeRetryCondition` | resty da el mecanismo (`AddRetryCondition`) pero **ninguna política**: por defecto no reintenta nada. Esta replica solo métodos idempotentes y solo 429/5xx |
| `RetryAfter` | resty tiene el hook `SetRetryAfter` pero **ninguna implementación**: sin callback ignora la cabecera y usa su propio backoff |
| `breakerTransport` | resty v2 **no tiene circuit breaker**. Enchufa el gobreaker existente como `http.RoundTripper` |
| `R(ctx)` en la interfaz | expone el builder de resty en vez de reimplementarlo |
| Respuesta junto al error | arregla el wrapper |

### La decisión de composición, verificada

El breaker va como **`http.RoundTripper`**, no envolviendo la llamada a resty.
Esa distinción es todo: el `RoundTripper` corre **dentro** del bucle de retry de
resty, así que el breaker cuenta intentos individuales.

`TestBreakerOpensAndShortCircuitsRetries`: con umbral 3 y **11 intentos
configurados**, el upstream recibe **3**. El primer intento de implementación
envolvía la llamada completa a resty — es decir `breaker(retry(op))`, el orden
descartado — y el test lo detectó dejando pasar los 11.

### Verificación

`gofmt` · `go build` · `go vet` · `golangci-lint` **0 issues** · `lint-arch` ·
`go test -race` **46/46 paquetes** · cobertura de `rest` **88,1 %**
(desde 0 % antes de la auditoría).

### Aplicar el mismo modelo al resto de clientes

Pendiente, con el mismo criterio: auditar cada cliente contra su SDK y **borrar
todo lo que el SDK ya resuelva**, dejando solo el refuerzo real.

| Cliente | Debilidad conocida del SDK | Refuerzo candidato |
|---|---|---|
| gRPC | sin breaker; reconexión manual reimplementada | breaker como `UnaryInterceptor`; borrar `ReconnectIfNeeded` y usar `WaitForReady` |
| SQS / SNS / S3 / SSM / DynamoDB | el SDK de AWS **ya trae** retry con backoff y jitter configurable | **no añadir retry**; solo breaker y observabilidad |
| Redis / Mongo / Memcached | los drivers ya traen pooling y reintentos | solo health check y breaker |
| Kafka | `kafka-go` ya gestiona reintentos y rebalanceo | solo DLQ (ya existe) y observabilidad |

El riesgo a vigilar es el opuesto al de antes: **duplicar retry** sobre SDKs que
ya reintentan, multiplicando los intentos reales sin querer.

## C6 — Decorator generalizado a todos los clientes — ✅ COMPLETADO

### El problema

`BaseClient.Execute` ya era el punto de enganche correcto, pero llevaba el
`if logging` incrustado en dos ramas casi idénticas y no contemplaba métricas ni
tracing. Peor: **7 de los 14 clientes ni siquiera lo usaban** — duplicaban su
trío `execute`/`executeWithResilience`/`executeWithLogging` entero, unas 40
líneas cada uno. Añadir métricas habría significado editar los 20 paquetes.

### La solución, reutilizando lo que ya existía

No se escribió un framework nuevo. `cloud.Middleware` ya tenía la forma correcta
(`func(next) next`) pero atada a tipos de AWS; se generalizó el **patrón** sobre
`BaseClient`, que es lo que ya usaban los clientes.

`pkg/core/client` expone ahora:

| Decorator | Qué añade |
|---|---|
| `WithLogging(log, enabled)` | consulta el flag en cada llamada, así `SetLogging` sigue funcionando en caliente |
| `WithMetrics(rec)` | duración y resultado por operación |
| `WithTracing(tracer)` | span `<cliente>.<operación>`; la firma coincide con `engine.Telemetry`, así que el provider de OTel encaja sin que el core toque el SDK |
| `WithResilience(cfg, log)` | retry + breaker como decorator, no como rama `if` |

`Chain` los compone de fuera hacia dentro. Logging por fuera ⇒ **una llamada
lógica produce una línea de log**, por muchos reintentos que haya debajo.

### Migración

| Cliente | Cambio |
|---|---|
| SQS, SNS, DynamoDB, Redis, gormsql, Cognito, gRPC | dejaron de duplicar el trío y **embeben `BaseClient`** |
| SES, S3, SSM, Mongo, Memcached, RabbitMQ, REST | ya lo embebían; se les quitó el logging inline |
| **gRPC** | además se **eliminó `ReconnectIfNeeded`**: `grpc.ClientConn` ya reconecta solo con su propio backoff. La versión manual redialaba a mano, podía bloquear con el write-lock tomado y descartaba los subcanales sanos que el SDK estaba restableciendo. Ahora basta `conn.Connect()` para sacarlo de IDLE |

**Resultado medido:** condicionales de logging **78 → 11** (los que quedan son
el propio middleware y el bucle del consumidor de RabbitMQ, que no es una
operación discreta). Duplicaciones del trío `execute`: **7 → 0**.
Neto: **74 ficheros, +1.471 / −1.520**.

### Efecto colateral: logs de construcción eliminados

Ocho clientes emitían un `Debug` al construirse («SQS client initialized»,
«Redis connection established successfully»…). Se eliminaron por el mismo
criterio de B15: **una librería no debe escribir líneas de log que la aplicación
no pidió**. Las operaciones sí se registran; la construcción no.

### Regresión aceptada, para que conste

Los bloques de logging inline de Cognito adjuntaban un `email` enmascarado a los
campos del log. El logging genérico registra solo `operation` y `service`, sin
PII, así que `maskEmail` quedó sin uso y se borró. **Se pierde ese detalle de
diagnóstico** a cambio de que ningún dato personal llegue al log por defecto. Si
hiciera falta recuperarlo, el camino es añadir atributos a `Invocation`, no
volver al condicional por cliente.

### Verificación

`gofmt` · `go build` · `go vet` · `golangci-lint` **0 issues** · `lint-arch` ·
`go test -race` **46/46 paquetes**. Cobertura: `pkg/core/client` 94,4 %,
`rest` 91,6 %, gRPC 79,7 %, Redis 62,2 %.

## Fase D — Pulido para `v1.0.0` · ~2 semanas

| # | Acción |
|---|---|
| D1 | Godoc en los ~50 símbolos exportados que reporta `revive`; reactivar `revive` en `.golangci.yml` |
| D2 | Renombrar paquetes a Go Style: `circuit_breaker`→`breaker`, `retry_backoff`→`retry`, `task_executor`→`taskpool`, `error_handler`→`apierror`, `app_profile`→`profile`, `file_utils`→`fileutil` |
| D3 | Inglés en todo el código, logs y comentarios; `grpc.Cliente`→`grpc.Client`; barrido de los strings en español restantes (`rest/service.go:133`, `router/service.go:152`, `task_executor/service.go:87,96`, `viper/service.go:277`, `app/entity.go:280`) |
| D4 | `examples/` ejecutables: `http-only`, `http+postgres`, `full-aws`, `lambda`, `kafka-consumer` |
| D5 | `Example*` godoc en todos los paquetes públicos; benchmarks en router, JWT y logger |
| D6 | Sanitizador de logs configurable vía `logger.Config` (email/IP **opt-in**, hoy redacta por defecto y corre 6 regex por campo) |
| D7 | `rest`: devolver la `*resty.Response` junto al error; reintentos solo en métodos idempotentes |
| D8 | Unificar en una sola librería JWT (hoy `golang-jwt/jwt/v5` en router + `lestrrat-go/jwx/v2` en cognito) |
| D9 | `integration.yml`: añadir Postgres, Mongo, Kafka, LocalStack (hoy solo Redis) |
| D10 | Borrar `init.sh` y `make init`; quitar `makefile.yml` o reducirlo a build+test (hoy ejecuta un `go mod edit -module` + `sed` masivo de plantilla sobre un repo ya establecido) |
| D11 | **`v1.0.0`** con `COMPATIBILITY.md` en vigor y la API congelada |

---

## Secuencia y decisiones abiertas

```
Fase A ──> release v0.2x publicable
   │
Fase B ──> librería fiable (3 semanas acumuladas)
   │
Fase C ──> v0.30.0, la librería modular (6 semanas más) ── C9 opcional
   │
Fase D ──> v1.0.0
```

**Decisiones que hay que tomar antes de arrancar C:**

1. **¿C9 (multi-módulo) entra o no?** Con C1–C8 el consumidor ya solo paga lo que importa *en el binario*, pero sigue descargando el `go.sum` completo. Dividir módulos es lo único que reduce también la descarga; a cambio, multiplica el coste de release. Recomendación: hacer C1–C8, medir, y decidir C9 con datos.
2. **¿`pkg/app` se mantiene como shim indefinidamente o se borra en `v0.30.0`?** El plan asume shim durante C3 y borrado en C4.
3. **D2 (renombrado de paquetes) rompe todos los imports de los consumidores.** Debe ir en el mismo release que C4 o esperar a `v1.0.0`, no en un release intermedio.

## Seguimiento

Este plan se ejecuta fase a fase. Cada fase cierra con su criterio de salida verificado en local **y** en CI antes de pasar a la siguiente.

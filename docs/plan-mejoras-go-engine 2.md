# Plan de mejoras y remediación — go-engine

> Basado en la revisión técnica profunda (`go-engine-review.md`, 2026-08-05).
> Objetivo: llevar el proyecto a una **release de estabilización** con timeouts/duraciones
> correctos, lifecycle completo, worker pool seguro, Kafka sin pérdida silenciosa, cero
> vulnerabilidades alcanzables y suite hermética, antes de intentar un `v1.0`.
>
> Decisiones de alcance acordadas:
> - **Priorizar correctitud**: se aceptan cambios rompientes de API donde sean necesarios
>   (p.ej. `Operation` con contexto), apuntando a un `v1.0` de estabilización.
> - Las rutas del informe corresponden al commit `884f652` (multi-módulo). Tras el colapso
>   a módulo único los directorios `aws/`, `messaging/`, `database/` se conservan **bajo un
>   único `go.mod`**; abajo se mapean a las rutas actuales verificadas.

## Estado verificado (checkout actual)

| Hallazgo | Estado | Ruta actual |
|---|---|---|
| Dobles conversiones de duración (retry) | Vigente | `pkg/utilities/retry_backoff/service.go:14-15` |
| Dobles conversiones de duración (CB) | Vigente | `pkg/utilities/circuit_breaker/service.go:16-17` |
| Dobles conversiones de duración (router) | Vigente | `pkg/app/router/service.go:59-61` |
| `Operation` sin contexto | Vigente | `pkg/core/client/entity.go:42` |
| Panic por resilience config nil + mutación de config | Vigente | `pkg/utilities/resilience/service.go:17-29`, `retry_backoff/service.go:26` |
| Worker pool: panic no contenido, carrera y fuga | Vigente | `pkg/utilities/task_executor/service.go:262-317` |
| `Run()` ignora contexto, sin `signal.Stop` | Vigente | `pkg/app/router/service.go:131-178` |
| TrustedProxies sobrescribe XFF sin validar | Vigente | `pkg/app/router/service.go:85-89` |
| Shutdown incompleto (no cierra clients) | Vigente | `pkg/app/service.go:59-159` |
| Kafka confirma aun si falla DLQ | Vigente | `messaging/pkg/integration/kafka/consumer.go` |
| gRPC insecure + reconexión sin mutex | Vigente | `messaging/pkg/integration/grpc/service.go` |
| Health timeout no duro + slice sin mutex | Vigente | `pkg/health/service.go`, `handler.go` |
| OTLP fuerza insecure, shutdown parcial | Vigente | `pkg/telemetry/otel/provider.go` |
| JWT/JWKS sin endurecer | Vigente | `pkg/app/router/jwt_middleware.go` |
| Fugas en constructores fallidos (redis/mongo/sql) | Vigente | `database/*/pkg/database/*/service.go` |
| Go 1.26.3 / gRPC 1.81.1 / x/text 0.37.0 | Vigente | `go.mod` |
| Falta LICENSE, SECURITY.md, CoC, dependabot | Vigente | raíz / `.github/` |
| `coverage-check` con `|| true`, singleton Viper | Vigente | `Makefile`, `pkg/config/viper/service.go` |

## Fase 0 — Contención (1–3 días) — ✅ COMPLETADA

Objetivo: dejar build, test, race, vet, lint y vulncheck verdes desde checkout limpio.

1. ✅ **Dependencias**: Go `1.26.5`, gRPC `1.82.1`, `x/text 0.39.0`; `go mod tidy`.
   `govulncheck` reporta **0 vulnerabilidades alcanzables**.
2. ✅ **Dobles duraciones**: eliminado `* time.Second` / `* time.Millisecond` en
   `retry_backoff`, `circuit_breaker` y `router` (los campos ya eran `time.Duration`).
   Tests que afirman duraciones exactas.
3. ✅ **Resiliencia nil-safe**: `normalizeConfig`/`normalizeCBConfig` toleran
   `RetryConfig`/`CircuitBreakerConfig` nil con defaults y **no mutan** la config del
   usuario (copian antes de normalizar). Test `NotPanics` con `Config{}`.
4. ✅ **Gobernanza**: añadidos `LICENSE` (MIT), `SECURITY.md`, `.github/dependabot.yml`;
   `version.yml` pasa a `workflow_dispatch` manual con selección de major/minor/patch
   (fin del bump automático por merge).
5. ✅ **CI y test hermético**: `ci.yml` añade job `govulncheck` y `go test -race`, y fija
   la versión de Go a `go.mod`; `coverage-check` ya no usa `|| true` (un test que falla
   marca FAIL); el test de Redis es hermético vía puerto muerto reservado en `TestMain` +
   `require.Error`.

Criterio de salida — verificado en local:
`go build ./...` ✅ · `go vet ./...` ✅ · `golangci-lint run` ✅ (0 issues) ·
`go test -race ./...` ✅ (0 data races) · `govulncheck ./...` ✅ (0 alcanzables).

## Fase 1 — Correctitud del runtime (1–2 semanas) — ✅ COMPLETADA

1. ✅ `Operation` migrado a `func(context.Context) (interface{}, error)`; `Execute` y los
   wrappers de resiliencia por-cliente pasan el contexto **acotado** a la operación. Todos
   los clientes (REST, S3, SES, SSM, SQS, SNS, Cognito, Dynamo, Redis, Mongo, SQL, gRPC,
   RabbitMQ, Memcached) migrados. Test que prueba que la operación recibe el deadline.
   (Genéricos con `T` en vez de `interface{}` se difieren a Fase 3.)
2. ✅ Worker pool: recover en la **misma** goroutine que ejecuta la tarea; resultado
   inmutable por canal buffered (fuga acotada); tests de panic, timeout no cooperativo,
   `-race` y goleak.
3. ✅ Kafka: `sendToDLQ` devuelve error; el offset se confirma **solo** tras handler o DLQ
   exitoso (sin pérdida silenciosa); backoff cancelable; `ErrNoDLQ`. Tests del contrato.
4. ✅ Lifecycle manager (`pkg/app/lifecycle.go`): cierre LIFO con `errors.Join`, idempotente,
   rechaza registro tras cierre; `Engine.Close(ctx)` cableado al shutdown del router y
   rollback en `Build()` fallido; registra Redis/Mongo/RabbitMQ/gRPC/Kafka/telemetry.
5. ✅ `Router.Run(ctx)` responde a contexto + señal + error; `signal.Stop` con defer;
   `Engine.Run()` pasa `e.ctx`. Test de cancelación y ejecución de hooks.
6. ✅ Health: `GetStatus(ctx)` con límite duro (recolección por canal, checkers no
   terminados → down), `Register` con `RWMutex`, handler usa `r.Context()`. Test de timeout duro.
7. ✅ Redis/Mongo/SQL cierran su cliente/conexión cuando falla el `Ping` del constructor.

Criterio de salida — verificado en local:
`go build` ✅ · `go vet` ✅ · `golangci-lint` ✅ (0 issues) ·
`go test -race ./...` ✅ (34 paquetes, 0 races) · `govulncheck` ✅ (0 alcanzables).

**Nota de breaking changes** (v1.0 de estabilización):
- `client.Operation` ahora es `func(context.Context) (interface{}, error)`.
- `resilience.Service.Execute` recibe `func(context.Context) (interface{}, error)`.
- `router.Service.Run` ahora es `Run(ctx context.Context) error`.
- `health.Service`: `IsReady(ctx)` y `GetStatus(ctx)`.
- Nuevo: `Engine.Close(ctx) error`.

## Fase 2 — Configuración y seguridad (1–2 semanas) — ✅ COMPLETADA

1. ✅ Eliminado el singleton de Viper (`NewService` devuelve instancia fresca); `ApplyDynamic`
   valida la carga inicial **y** cada reload vía `buildConfig` compartido (snapshot rechazado
   si no valida); eliminado el watcher nativo duplicado. Test de no-singleton.
2. ✅ El reload reutiliza `buildConfig`, por lo que cada snapshot recargado se valida antes de
   publicarse. (Wiring de hooks tipados al Engine para recarga en caliente → Fase 3.)
3. ✅ Router: `trustedRealIP` confía en XFF/X-Real-IP **solo** si el peer directo es un proxy
   confiable (IP o CIDR); sin proxies configurados los headers se ignoran (default seguro).
   pprof ahora es opt-in (`EnablePprof`) y nunca en prod. Tests de spoofing, CIDR y pprof.
4. ✅ gRPC: `TLSConfig` (CA/mTLS/serverName), default de timeout, y `ReconnectIfNeeded` reusa
   las mismas credenciales bajo `RWMutex` (arregla la carrera de reconexión). OTLP: TLS por
   defecto con `Insecure` opt-out + `Headers`; cierra el trace provider si falla el de métricas;
   `Shutdown` agrega errores con `errors.Join`. Tests de credenciales.
5. ✅ JWT/JWKS: RS256 explícito (`jwt.WithValidMethods`), issuer/audience obligatorios por
   defecto con opt-out (fail-closed 500), cliente HTTP/reloj inyectables, status 2xx,
   `io.LimitReader` (1 MiB), validación `use=sig`/`alg=RS256`, y umbral de refresh/stale
   acotado (fix del TTL pequeño + ventana absoluta de 6 h). Tests nuevos.
6. ✅ El validador exige `aws.region` **solo** si hay un adaptador AWS configurado; formato de
   region por regex (no lista fija incompleta). Test actualizado.

Criterio de salida — verificado en local:
`go build` ✅ · `go vet` ✅ · `golangci-lint` ✅ (0 issues) ·
`go test -race ./...` ✅ (35 paquetes, 0 races) · `govulncheck` ✅ (0 alcanzables).

**Nota de breaking changes** (v1.0 de estabilización):
- `router.Config`: nuevo `EnablePprof` (pprof deja de exponerse por defecto en no-prod).
- `router.JWTAuthConfig`: issuer/audience requeridos por defecto (usar `AllowEmptyIssuer`/
  `AllowEmptyAudience` para el comportamiento anterior); nuevo `HTTPClient`.
- `grpc.Config`: nuevo `TLS *TLSConfig`. `otel.OTELConfig`: nuevos `Insecure`/`Headers`
  (OTLP pasa a TLS por defecto).
- `viper.NewService` deja de ser singleton.

## Fase 3 — API y modularidad pre-v1 (2–4 semanas) — 🟡 PARCIAL

1. ⏭️ **Diferido**: core sin imports de proveedores. Es un rediseño de la superficie pública
   (registry tipado, quitar getters/campos tipados del `Engine`, config sin tipos de proveedor);
   hacerlo parcialmente rompe el build por mucho tiempo. Documentado en
   [`docs/adr/0001-core-provider-decoupling.md`](adr/0001-core-provider-decoupling.md) como
   milestone dedicado con major bump.
2. ⏭️ **Diferido** junto con el punto 1 (mismo ADR).
3. ✅ Nombres normalizados a inglés: `NewCliente`→`NewClient` (gRPC), `SendMsj`→`SendMessage`,
   `ReceiveMsj`→`ReceiveMessages`, `DeleteMsj`→`DeleteMessage`, `PublishMsj`→`PublishMessage`
   (SNS) y errores `Err*` en español → inglés (SQS). Interfaces, impls, mocks y callers.
4. ✅ `interface{}` → `any` en todo el módulo (`gofmt -r`, 530 sitios). Los genéricos con `T`
   en `Operation` se difieren (el fix de correctitud ya no los necesita — Fase 1).
5. 🟡 Parcial: documentación de thread-safety/lifecycle/errores añadida en las APIs tocadas
   (lifecycle, health, worker pool, JWT, gRPC, config). Barrido completo de los 44 paquetes → Fase 4.
6. ✅ Publicadas `COMPATIBILITY.md` y `MIGRATION.md` (breaking changes de Fases 0–3).

Criterio de salida (de la parte entregada) — verificado en local:
`go build` ✅ · `go vet` ✅ · `golangci-lint` ✅ · `go test -race ./...` ✅ · `govulncheck` ✅.

**Nota**: los items 1–2 (desacople del core) quedan como el siguiente milestone mayor.

## Fase 4 — Calidad open source (continuo) — 🟡 EN PROGRESO

1. 🟡 Cobertura por riesgo: ejemplos y benchmarks añaden cobertura a health/resilience/
   worker pool/client. El barrido de adaptadores hoy en 0% (SES/SNS/SSM/Dynamo/Memcached/
   Mongo/SQL) sigue pendiente (mejor cubierto por los jobs de integración del punto 2).
2. 🟡 Scaffolding de integración: test con build tag `integration` para Redis
   (`database/redis/.../integration_test.go`, excluido del suite hermético) + workflow
   `integration.yml` que levanta Redis 7. Mongo/SQL/Kafka/RabbitMQ/LocalStack quedan por
   añadir con el mismo patrón.
3. ✅ Benchmarks: `BenchmarkWorkerPool` (fan-out del pool), `BenchmarkBaseClientExecute`
   (overhead del wrapper con/sin resiliencia), `BenchmarkGetStatus` (fan-out de health).
4. ✅ Ejemplos compilables verificados por CI: `ExampleNewService` (health),
   `ExampleService_Execute` (resilience), más el `Example_customClient` existente.
5. ✅ Gobernanza: `CODE_OF_CONDUCT.md`, plantillas de issues (bug/feature) + `config.yml`.
   (LICENSE, SECURITY.md, dependabot y versionado deliberado ya en Fase 0.) SBOM/provenance
   y release notes automáticas desde changelog quedan pendientes.

Criterio de salida (de lo entregado) — verificado en local:
`go build` ✅ · `go vet` ✅ · `golangci-lint` ✅ (0 issues) · `go test -race ./...` ✅
(35 paquetes, hermético) · `govulncheck` ✅ · benchmarks y ejemplos ejecutan.

**Pendiente de Fase 4** (continuo): más jobs de integración (Mongo/SQL/Kafka/RabbitMQ/
LocalStack), cobertura de adaptadores en 0%, SBOM/provenance y release notes automatizadas.

## Seguimiento

El progreso se lleva en la lista de tareas de la sesión. Estado: Fases 0–2 ✅, Fase 3 🟡
(core decoupling diferido a milestone mayor — ver ADR 0001), Fase 4 🟡 en progreso.

# Inventario de funcionalidades y mapa de migración

Estado: inventario descriptivo, no normativo. Prevalecen [decisiones](decisions.md)
y contratos corregidos. No se preservan garantías cambiadas mediante bridges.
Revisión estática del árbol, no migración ejecutada.
Se contabilizan **69 directorios con archivos Go no-test**, incluyendo providers,
mocks distribuidos y ejemplos. Los paquetes solo-test y benchmarks se trasladan
con su código dueño. Los paths destino son propuestos bajo
`github.com/skolldire/go-engine/`; no se crean módulos vacíos.

## Cobertura de código actual

| Origen actual | Funcionalidad encontrada | Módulo/paquete destino | Tratamiento y condición de migración |
|---|---|---|---|
| `aws/pkg/clients/cognito` | Registro/login/confirmación, sesión/tokens/password, MFA, grupos, JWT y JWKS cache | aws/cognito; auth/jwt para funcionalidad verificablemente neutral | No mover auth Cognito a un validador universal; comparar ambos caches JWKS antes de unificar |
| `aws/pkg/clients/s3` | Put/Get/Delete/Head/List/Copy, presigned GET y PUT | aws/s3 | Solo SDK S3; conservar expiración, content type y streaming/ownership de bodies |
| `aws/pkg/clients/ses` | Email/raw/bulk/attachments, quotas/stats y gestión de identidades | aws/ses | Solo SDK SES; separar retry seguro de envío y resultados por destinatario |
| `aws/pkg/clients/sns` | Topics/subscriptions/publish/JSON, SMS/bulk SMS/opt-out, platform app/endpoints/push | aws/sns | Conservar todas las APIs: SNS no es únicamente publish de eventos |
| `aws/pkg/clients/sqs` | Send/JSON/receive/delete, create/delete/list/URL de colas | aws/sqs | Documentar receipt handles, visibility y dedup FIFO donde exista; no afirmar exactly-once |
| `aws/pkg/clients/ssm` | Parameter/secure parameter, paths/history, tags, exists y delete | aws/ssm; config/ssm para lectura de configuración | Cliente de servicio separado de fuente config; KMS/decrypt y redacción explícitos |
| `aws/pkg/database/dynamo` | Typed CRUD/query/scan, batch, transact write y key helpers | aws/dynamodb | Transacciones Dynamo no se abstraen como *sql.Tx; conservar operaciones nativas y límites |
| `aws/pkg/database/dynamo/mock` | Mock generado para interfaz Dynamo | aws/dynamodb/testkit | Solo pruebas; regenerar contra contrato del SDK elegido |
| `aws/pkg/integration/aws` | Fachada Client.Do, helpers por servicio, opciones retry y observabilidad | integration/aws agregado + integration/aws/<servicio> | Mantener agregador opt-in; helpers por servicio no deben obligar a importar todos los SDK |
| `aws/pkg/integration/aws/adapters` | Adapters S3/SQS/SNS/SES/SSM/Lambda, dispatch y normalización de errores | integration/aws/{s3,sqs,sns,ses,ssm,lambda} | Dividir directorio por servicio; Lambda actual existe en fachada, no como cliente dedicado equivalente a S3 |
| `aws/pkg/integration/inbound` | Normalización de eventos API Gateway/SNS/SQS | integration/aws/events | Solo SDK de events + fachada; handlers entrantes no requieren clientes salientes |
| `aws/pkg/testutil` | Mocks/assertions S3 y SQS | aws/s3/testkit y aws/sqs/testkit | Dividir mocks por servicio para no volver a unir dependencias |
| `aws/preset` | Bundle de providers AWS según secciones | presets/aws | Agregador deliberado opt-in; no usarlo para resolver credenciales en un cliente individual |
| `aws/provider/awsbase` | Resolución compartida AWS config y helper provider genérico | aws/config; engine/providers/awsbase | Separar credenciales/config SDK de engine.Deps; región solo cuando se selecciona AWS |
| `aws/provider/cognito` | Provider nombrado cognito: decode, Init, Close y From | engine/providers/aws/cognito | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `aws/provider/dynamo` | Provider nombrado dynamo: decode, Init, Close y From | engine/providers/aws/dynamodb | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `aws/provider/s3` | Provider nombrado s3: decode, Init, Close y From | engine/providers/aws/s3 | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `aws/provider/ses` | Provider nombrado ses: decode, Init, Close y From | engine/providers/aws/ses | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `aws/provider/sns` | Provider nombrado sns: decode, Init, Close y From | engine/providers/aws/sns | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `aws/provider/sqs` | Provider nombrado sqs: decode, Init, Close y From | engine/providers/aws/sqs | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `aws/provider/ssm` | Provider nombrado ssm: decode, Init, Close y From | engine/providers/aws/ssm | Separar del cliente; depende del cliente concreto y del engine, nunca a la inversa |
| `concurrency` | Candidato Pool/Job/Future/Stats, streaming y limiter ponderado; sin dependencias | concurrency | C1–C7 implementados; validación y RC en [evidencia](../modules/concurrency.md), E6/E7 pendientes; no publicar todavía |
| `database/memcached/pkg/database/memcached` | Get/Set/Delete/Add/Replace, counters, multi-get, FlushAll y prefijos | database/memcached | Extraer conservando semántica de miss; operaciones peligrosas explícitas, sin promesa de durabilidad |
| `database/memcached/provider/memcached` | Named provider Memcached y health/lifecycle | engine/providers/database/memcached | Separar integración |
| `database/mongodb/pkg/database/mongodb` | Conexión, database/collection access, ping y disconnect | database/mongodb | Extraer funcionalidades reales; no anunciar transacciones Mongo nuevas por mover este cliente |
| `database/mongodb/provider/mongodb` | Named provider MongoDB y health/lifecycle | engine/providers/database/mongodb | Separar contrato de integración del SDK |
| `database/redis/pkg/database/redis` | Strings/TTL/counters/hashes/lists/sets/sorted sets, prefijos, pipeline/tx pipeline y client escape hatch | database/redis | Extraer sin engine/BaseClient; pipeline no equivale a transacción SQL ni idempotencia financiera |
| `database/redis/pkg/testutil` | MockRedisClient y setups de respuestas | database/redis/testkit | Solo pruebas; revisar nuevas interfaces y mantener helpers necesarios |
| `database/redis/provider/redis` | Provider Redis, configuración por nombre y health | engine/providers/database/redis | Separar integración; conservar close y prefijo por instancia |
| `database/sql/pkg/database/gormsql` | GORM, CRUD, Find/Count/Exec, transactions, WithContext, pool y dialector externo | database/gorm; adaptadores por dialecto | Extraer manteniendo gorm.DB escape hatch; type config actual no construye el dialector; factories futuras deben ser explícitas |
| `database/sql/provider/sql` | GORM provider por nombre con dialector recibido desde código | engine/providers/database/gorm | Separar wiring; registro de dialector/backend permite config sin importar drivers ajenos |
| `database/sqlc/pkg/database/sqlcdb` | Driver/DSN, DBTX compatible con sqlc, queries, Tx/TxWithOptions, connector, stats y Close | database/atomic; database/postgres y database/mysql para apertura | Extraer conservando DBTX y *sql.Tx; código sqlc generado pertenece a cada aplicación; prohibir retry ciego de writes |
| `database/sqlc/provider/sqlc` | Named provider sqlc, driver/DSN y health | engine/providers/database/atomic | Mantener aliases de sqlc_clients mientras exista compatibilidad; mapear a database.instances en nuevo esquema |
| `example/cmd/api` | Bootstrap de engine, handlers y wiring | examples/assembled-app | Ejemplo de ensamblado; nunca módulo requerido por consumidores |
| `example/internal/handler` | Handlers de aplicación para ejemplo de órdenes | examples/assembled-app/internal/handler | No convertir lógica de órdenes en API del framework |
| `example/internal/repository` | Repositorio/SQL de órdenes de ejemplo | examples/assembled-app/internal/repository | Código de aplicación; adaptar a unidad de trabajo elegida |
| `example/internal/usecase` | Caso de uso de órdenes | examples/assembled-app/internal/usecase | Mantener dependencias hacia interfaces de aplicación |
| `http/pkg/rest` | Resty builder R y verbos, body/query/headers, retry seguro, Retry-After y breaker de transport | httpclient/resty; httpclient para contratos estándar | Extraer sin BaseClient/engine; conservar API resty solo en adaptador y evitar doble retry resty/decorator |
| `http/pkg/testutil` | MockRestClient y respuestas HTTP | httpclient/resty/testkit | Pruebas del adaptador; no dependencia productiva |
| `http/provider/rest` | Decodificación por instancia, construcción y retrieval de REST | engine/providers/httpclient/resty | Mover a módulo de integración que depende del cliente; nunca al revés |
| `messaging/pkg/integration/grpc` | Cliente, conexión lazy, InvokeRPC, TLS, metadata/headers y close | rpc/grpc/client | Conservar opciones nativas del SDK explícitas; no Kafka/RabbitMQ |
| `messaging/pkg/integration/kafka` | Producer/consumer combinados, keys, retry lineal, DLQ, commits, async publishing y checker | messaging/kafka | Separar políticas; definir contratos de offset, rebalances, poison message y orden por partición; retry exponencial cambia semántica actual |
| `messaging/pkg/integration/rabbitmq` | Publish/consume, ack/nack, panic containment, queue/exchange declaration y binding | messaging/rabbitmq | Extraer sin BaseClient; formalizar QoS/prefetch, confirms, reconnect, DLQ y callback ctx como ampliaciones tras revisar implementación |
| `messaging/pkg/server/grpc` | Registro de servicios, Start/Stop, listener, drain, forced stop y dirección | rpc/grpc/server | Preservar shutdown acotado e idempotente; servidor no arranca al construir |
| `messaging/provider/grpcclient` | Provider de cliente gRPC por nombre | engine/providers/rpc/grpcclient | Integración separada de cliente |
| `messaging/provider/grpcserver` | Provider de servidor gRPC por nombre | engine/providers/rpc/grpcserver | Integración separada de servidor |
| `messaging/provider/kafka` | Provider de cliente Kafka y health | engine/providers/messaging/kafka | Integración separada del módulo Kafka |
| `messaging/provider/rabbitmq` | Provider de cliente RabbitMQ | engine/providers/messaging/rabbitmq | Integración separada del módulo RabbitMQ |
| `pkg/core/client` | BaseClient, timeout, Invocation/Chain, SafeTypeAssert y decorators logging/metrics/tracing/resilience | client; integraciones en client/logging, client/otel y client/resilience | Dividir: contratos sin SDK; retirar embedding obligatorio y preservar causas/deadlines; comparar semántica del middleware real, no solo sus comentarios |
| `pkg/engine` | Carga Viper, env, decoder/raw sections; Provider/Deps/Get, componentes, salud/telemetría, lifecycle y rollback | config (+ config/viper), engine y engine/providers por capacidad | Dividir config.go/env.go/decoder.go/rawconfig.go/decode.go; config tipada de router/log/health sale del contrato mínimo; conservar cierre LIFO e init transaccional |
| `pkg/health` | Checkers SQL/Redis por interfaces, checker HTTP, registro, timeout, estados y handlers live/ready/deps | health; handlers públicos en paquete health/http o router/operations | Extraer sin importar drivers; verificar HTTPChecker que actualmente acepta respuestas menores que 500; no cambiar criterio sin contrato |
| `pkg/integration/cloud` | Client.Do, request/response JSON, middleware y Error con códigos/retriable AWS | integration/cloud | Extraer contratos stdlib; conservar shape de fachada pero auditar semántica AWS en nombres; no imponerlos a clientes nativos |
| `pkg/integration/observability` | Decorators de fachada: Logging, Metrics y Tracing; MetricsRecorder conectado a Telemetry | integration/cloud/logging e integration/cloud/otel | Dividir logger/SDK de contratos; estos decorators no deben volver obligatoria la telemetría |
| `pkg/router` | Chi, middleware/CORS/proxies, JWT/JWKS/grupos/claims, ping, health/pprof, servidor y hooks de shutdown | router, router/chi, router/operations, auth/jwt y auth/jwt/http | Dividir por responsabilidad; escape hatch *chi.Mux permanece solo en compatibilidad; host/subdomain routing es ampliación nueva |
| `pkg/telemetry/otel` | Providers, OTLP gRPC, métricas, spans, helpers, middleware, no-op, global providers y Shutdown | telemetry/otel, telemetry/otlpgrpc; integración client/otel | Separar SDK de exporter y registro global; OTLP HTTP, Prometheus y OTel Logs son adicionales, no extraídos de aquí |
| `pkg/testutil` | MockLogger, contexto de pruebas vacío/con timeout | testkit/logging y testkit/context | Solo pruebas; no publicar mocks o testify como dependencia runtime de herramientas |
| `pkg/utilities/app_profile` | SCOPE, local/test/stage/prod y clasificación por sufijos | utilities/profile; wrapper env en compatibilidad | Extraer parser puro; documentar scope compuesto, desconocido, vacío y fallback local heredado |
| `pkg/utilities/circuit_breaker` | gobreaker, thresholds, estado, callbacks y mapping errores | circuitbreaker | Quitar logger obligatorio; decidir si State expone tipo propio o mantiene gobreaker en API de compatibilidad |
| `pkg/utilities/error_handler` | CommonApiError, códigos/status, wrapping/copy-on-write/details, response HTTP y log | router/errors | Extraer contrato HTTP; no usar HTTPCode para clasificar errores de todas las herramientas; preservar redacción/no filtrar internos |
| `pkg/utilities/helpers` | Checks, coalesce/defaults, maps/slices/strings, pointers y wrappers errors | utilities/{collections,strings,pointers,defaults}; errores estándar | Separar paquetes puros dentro de un módulo stdlib; revisar nil/zero, orden de mapas y nombres antes de retirar wrappers redundantes |
| `pkg/utilities/logger` | Logger logrus, nivel/formato/writers, contexto, redacción de secretos y logs | logging, logging/logrus | Slog base; conservar redacción y contextualización; salida fatal/os.Exit no forma parte del contrato de herramientas |
| `pkg/utilities/logger/logrusadapter` | Adaptación bidireccional de interfaz logger y tipos logrus | logging/logrus | Conservar como puente opt-in, no dependencia de la nueva abstracción |
| `pkg/utilities/resilience` | Composición breaker exterior/retry interior y estado del breaker | resilience | Cambio de ordering documentado como ruptura explícita, sin puente; clasificación de fallos e intentos; sin logger propio obligatorio |
| `pkg/utilities/retry_backoff` | Normalización/defaults, retry, backoff exponencial y jitter aditivo, logging | retry | Adaptar a ctx por intento, full jitter, cap seguro y cero retries real; son cambios semánticos, no mero rename |
| `pkg/utilities/task_executor` | Task/Tasker, prioridades, WorkerPool/BatchWorkPool, timeouts, callbacks y métricas | concurrency; legado congelado | Deprecated sin wrapper ni equivalencia falsa; timeout puede retornar con tarea viva; retirada en release de migración. Ver [migración](../modules/concurrency.md) |
| `pkg/utilities/validation` | Validator custom not_empty, nombres JSON y singleton global | validation | Extraer instancia explícita; reemplazar dependencia en helpers por regla local pequeña; globals solo compatibilidad |
| `preset/full` | Bundle amplio de providers actuales, excluye SQL/GORM/sqlc y concurrency | presets/full | No ampliar con todos los módulos nuevos silenciosamente; publicar footprint y selección explícita |
| `preset/http` | Options de ensamblado HTTP con router/salud/log | presets/http | Mantener como conveniencia; nombre del preset no debe confundirse con cliente HTTP |
| `provider/otel` | Provider engine OTel y validación de claves heredadas | engine/providers/telemetry/otel | Integración depende del módulo OTel; tool no importa el engine |

## Funcionalidades nuevas o incompletas

| Capacidad solicitada | Situación comprobada | Destino y validación requerida |
|---|---|---|
| Loader independiente | Hoy está dentro de engine y usa Viper | config + fuentes opcionales; conservar env/duración/unknown keys sin engine |
| Utilities independiente | Helpers/app-profile están en raíz | utilities por paquetes puros; no traer logger/validator |
| Admisión y orden por clave | Implementados en el candidato, no en Map | Pool separado con tests de saturación, FIFO/round-robin y lifecycle; [evidencia](../modules/concurrency.md) |
| Reintentos clasificados con full jitter | Hay retry genérico, resty y SDK, más Kafka lineal | retry + integraciones; presupuesto único y seguridad de replay |
| Idempotencia durable | No hay store/claim/completion implementados | idempotency + postgres/mysql; unique keys, fingerprints y fencing |
| Outbox/inbox | No hay implementación | outbox/inbox + stores por dialecto + publishers elegidos |
| Saga durable | No hay estado/resume/compensación implementados | saga + stores por dialecto; pruebas de crash y recuperación |
| Auditoría de negocio | Log sanitizer no es un audit store | audit + postgres/mysql; atomicidad y retención explícitas |
| Backend SQL desde host/user/password | sqlc usa driver/DSN; GORM recibe dialector por código | database/atomic + adapters + registry de aplicación |
| Dominios/subdominios HTTP | Router actual no define host routing por config | router; allowlist, proxy trust, precedence y conflictos |
| Routers app/operaciones independientes | Parte de la superficie comparte el router actual | router + router/operations con bind y lifecycle separados |
| OTel HTTP/Prometheus/log export | Hoy OTel configura OTLP/gRPC para trazas/métricas | Exportadores opcionales por señal/transporte; no mezclar con logging base |
| Config dinámica | No hay paquete dynamic en árbol actual | Integración futura tras contrato de snapshots/recarga; no prometer watcher existente |
| Registro genérico de clientes | No hay pkg/core/registry en árbol actual | Assembly registry nuevo, local a aplicación, solo si aporta selección real |
| pprof y benchmarks | Hay pprof opt-in y benchmarks actuales | operations opt-in; benchmarks siguen *_test.go, sin paquete runtime de benchmark |

## Hallazgos que requieren decisión semántica

- **Concurrencia:** timeout del executor antiguo puede retornar sin que el código
  de usuario termine; el scheduler nuevo no libera el slot hasta el retorno.
  Son contratos distintos: no escribir un alias fingiendo equivalencia.
- **Retry:** defaults actuales no distinguen siempre cero de ausencia; el jitter
  actual es aditivo y Kafka usa backoff lineal. Adoptar full jitter y clasificación
  requiere migración de comportamiento, no solamente mover imports.
- **SQL:** `gormsql.Config.Type` no basta hoy para construir el dialector; sqlc sí
  usa un driver registrado externamente. El ensamblador registra factories por
  tipo y no vuelve el módulo neutral dependiente de todos los dialectos.
- **BaseClient:** reúne timeouts, logger, resiliencia y observabilidad. Separar
  contratos y decorators antes de extraer adaptadores; no moverlo a un nuevo
  paquete que siga importando todo. Revisar deadlines ya presentes para que el
  presupuesto propio no quede ignorado cuando el caller tiene otro mayor.
- **Auth:** existen JWKS en router y Cognito; comparar reglas de validación,
  caché y fallback antes de consolidar. No degradar issuer/audience/algoritmo.
- **Globals:** perfil lee SCOPE, validator tiene singleton y OTel puede registrar
  providers globales. No trasladar globals al nuevo diseño; el legado se congela y se migra mediante ruptura explícita si cambian garantías.
- **Kafka:** revisar implementación completa de commit por partición: dejar un
  mensaje sin commit y seguir leyendo no demuestra por sí solo que un commit
  posterior no adelante el offset de uno fallido. Este es un riesgo a validar,
  no un bug probado únicamente por el inventario estático.
- **HTTP:** REST saliente, router entrante, CORS, hosts permitidos y errores HTTP
  son piezas diferentes. Los Config no deben volver a agruparlas obligatoriamente.
- **SDK escape hatches:** APIs que devuelven `*resty.Request`, `*chi.Mux`, GORM,
  Redis client o tipos AWS permanecen en adaptadores específicos, no en módulos
  neutrales. Se debe mapear cada método público antes de retirar una API vieja.

## Cobertura fuera de paquetes

`go.work`, go.mod/go.sum por módulo, Makefile, scripts de consumidor y versiones,
workflows CI/lint/integration/release/version, documentación de compatibilidad,
README, ejemplos YAML y fixtures son parte de la migración. Sustituir listas
manuales divergentes por un catálogo de módulos con validación, manteniendo una
lista explícita de publicables y excluyendo ejemplos/test harness. No confundir
el catálogo de módulos de build/release con un registro runtime de plugins.

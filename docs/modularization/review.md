# Revisión integral: brechas, acoplamientos y decisiones

Estado: revisión estática y contratos previos al código, 12-11-2026. No confundir
una decisión especificada con una garantía ya probada. Los contratos normativos
están en [public-contracts](public-contracts.md) y
[concurrency-api](concurrency-api.md); prevalecen sobre ejemplos previos.

## Funcionalidades o garantías que faltaban en el diseño

| Hallazgo | Resolución de contrato | Implementación |
|---|---|---|
| Rate limiting confundible con concurrencia | Nuevo módulo ratelimit token-bucket, separado de Limiter ponderado | Pendiente |
| Pool sin admisión propia ni Future | Pool/Job/Future y ledger vivo acotado; no output global que impida shutdown | Pendiente |
| Presupuesto de outputs sin tamaño conocido | Peso limita inputs queued+running; output y memoria del caller fuera del presupuesto | Explícitamente limitado; no vender heap bound |
| Cancelación de Wait vs ejecución ambigua | Wait cancela solo espera; Cancel cancela job; Submit ctx rige toda su vida | Pendiente |
| Orden por key sin regla tras fallo | continue o cancel_pending obligatorios; no key suspendida indefinida | Pendiente |
| Orden de resultados mezclado con orden de ejecución | Map completion-order; keyed execution en Pool; salida ordenada fuera de v1 | Decisión cerrada |
| Retry sin tipo de decisión de replay | RetryDecision=stop/retry/reconcile y classifier del consumidor | Pendiente |
| Resultados desconocidos tras commit/publicación | Estado uncertain y reconciliación; no retriable automático | Pendiente en stores/adaptadores |
| Incomplete ack/lease fencing | Stores verifican owner token/versión y filas afectadas; lease deadline controlado por reloj DB | Pendiente |
| Clock skew y timestamps | UTC para registros, monotonic local para duraciones, reloj DB para leases; clock inyectado solo por deps de tests/algoritmos que lo necesiten | Pendiente |
| Schema migrations y capacidad por dialecto | API migrations explícita de adapter, no AutoMigrate implícito; soporte declarado por versión | Pendiente |
| Retención/cleanup y tombstones | Cleanup por batches y deadline; nunca borra live claim; cada capacidad tiene política propia | Pendiente |
| Tamaño de bodies/eventos/respuestas/secrets | Máximos configurados y error antes de admisión/persistencia; no logging del contenido | Pendiente |
| Reconexión y seguridad del transporte | TLS/SASL/QoS/confirms/acks explícitos por SDK, sin retry implícito de writes inseguros | Pendiente |
| Pool/transport compartido y ownership | Dependencias borrowed por defecto; quien abre cierra; Close de wrapper no cierra recurso prestado | Pendiente |
| Config JSON/env sin YAML | Fuente JSON stdlib y env binding explícito, además de YAML/Viper/SSM | Pendiente |
| Profiles desconocidos y env snapshot | ParseScope puro/estricto; legacy wrapper preserva fallback; config usa snapshot env de carga | Pendiente |
| Observación bloqueante no especificada | Stats pull y logging/hook síncrono explícito fuera de locks; no goroutine/cola oculta infinita | Pendiente |
| Default severidad/redacción/secretos | Cero credenciales en errores/Stats; secret providers inyectados; source config no se imprime completa | Pendiente |
| Pruebas de consumidor por módulo solo bajo workspace | API fixtures GOWORK=off y matriz de módulos/versions resolubles | Parcial: solo Map experimental |
| Config root schema/unknown components | Root lo define aplicación; providers validan campos y registry valida referencias/tipos | Pendiente |
| Estabilidad v1 vs release 0.x coordinado | concurrency podrá versionarse a v1 por manifiesto por módulo; otros siguen 0.x | Workflow debe cambiar |

## Acoplamientos existentes y potenciales

| Borde | Problema comprobado o potencial | Frontera fijada |
|---|---|---|
| tools → root BaseClient | Logger, retry y SDK de observabilidad terminan en clientes nativos | client stdlib + wrappers separados; no embedding obligatorio |
| engine → Config router/logger/health | Registry no es independiente aunque no importe adapters | engine recibe Provider genérico y lifecycle; providers tipan configs |
| config → todos los Config | Reintroducir un mega-config trae todos los drivers | root schema de la aplicación; módulos reciben structs propios |
| neutral SQL → postgres+mysql | Factory dentro de atomic vuelve obligatorios los drivers | Registry local del ensamblador, factories importadas explícitamente |
| durable SQL store → crea su propio Tx | Rompe atomicidad con negocio/outbox/audit | DBTX supplied; store transaccional no hace Begin/Commit |
| utilities → validator/logger/config | Monolito utils reaparece por conveniencia | Utilities puro, config lectura aparte, validación regla local |
| middleware en subpaquete con mismo go.mod | Sigue declarando SDK aunque consumidor no lo use | Wrapper con SDK opcional en módulo separado |
| router → auth AWS/telemetry/pprof | Entran funcionalidades no elegidas | JWT neutral, middleware opt-in y operations separado |
| retry → tipos error de todos los SDK | Classifier universal fuerza imports cruzados | Decision callback por adaptador/aplicación |
| telemetry/otel → exporter gRPC | El SDK base fuerza transporte incluso sin exporter | Reader/exporters inyectados; otlphttp/otlpgrpc separados |
| logging → tracing global | Solo log requiere SDK de tracing | Slog base; puente OTel separado |
| façade AWS → todos los servicios | Consumidor S3 descarga SDK no elegidos | Per-service facade; agregado integration/aws opt-in |
| config/ssm → cliente SSM → config | Ciclo si cliente lee config desde dentro | Cliente solo Config tipado, sin lector/config dependency |
| shared common → todos | Tipos genéricos se convierten en dependencia transversal | Interfaces del consumidor; wire types solo compartidos donde hay protocolo real |
| root compatibility ↔ nuevos tools | Ciclo o ambiguous imports | Tools sin root require; paths nuevos; ruptura sin puente cuando cambian garantías |

## Decisiones normativas

La fuente de decisiones es [decisions.md](decisions.md), con D01–D24 y las
aclaraciones posteriores del arquitecto recibidas en esta conversación. Este
informe no duplica la tabla para evitar divergencia. Notion y ADR 0002 no son
normativos; D32 obliga a corregir un documento en conflicto antes de implementar.

## Pendientes reales y quién los resuelve

No quedan alternativas de arquitectura abiertas para concurrency v1; su archivo
no se modifica por las respuestas recibidas. Las firmas client, categorías y evidencias fueron concretadas con autorización
del usuario; la ausencia de answers.md no es bloqueo. Quedan
comprobaciones que no pueden cerrarse con prosa:

| Pendiente | Dueño | Evidencia necesaria / efecto |
|---|---|---|
| API signatures, scheduler y state machine | Implementación concurrency | Cumplir C1–C8 y gates; hasta entonces no estable |
| Versión mínima PostgreSQL/MySQL por adapter | Implementación SQL/CI | Ejecutar matriz, declarar floor exacto; bloquea publicar ese adapter, no concurrency |
| Versiones de SDK concretas y compatibilidad | Implementación de cada adapter | Pin/scan/test de API; no requiere decidirlas antes de diseñar contratos |
| Límites/retención/keys/isolation y replay seguro del negocio | Aplicación consumidora | Config explícita y validación de invariantes; no inventar defaults financieros |
| Red/DNS/certificados/secret source deployment | Operación de la aplicación | Bind/TLS/host y controles definidos por despliegue; no rol del router |
| Manifest de versiones y publicación por módulo | Release tooling | Consumer con tags reales, no solo replace local; concurrency no se publica desde el workflow anterior |
| Revisión de equivalencias públicas heredadas | Migración de cada módulo | Tabla de métodos/tipos y fixtures de import; no todo puede ser type alias |

Una Config inválida o una dependencia no registrada falla antes de abrir recursos;
no se usa la palabra "pendiente" para posponer una elección necesaria en runtime.
Los valores que legítimamente pertenecen a la aplicación se marcan requeridos
por los contratos, en vez de quedar abiertos dentro de la biblioteca.

## Evidencia específica del prototipo concurrency

Revisión de `concurrency.go`, `batch.go`, tests, benchmark y consumer guard:

| Observación | Implicación |
|---|---|
| From no valida ctx y lo usa desde goroutine | nil ctx puede panic asíncrono; la firma v1 retorna error uniformemente |
| Map.Config.Buffer solo controla output | No afirmar admisión o memoria total acotada; input es propiedad del caller |
| Result solo contiene Value/Err | El prototipo no tiene IDs/keys/Future ni índice de jobs aceptados |
| invoke ejecuta síncrono en worker y recupera panic | Buena base: mantiene límite real al timeout; extraer PanicError tipado y causa consistente |
| Cancel/timeout compiten con output select | Streaming puede entregar o descartar último resultado; pool Future tiene otra regla de transición terminal |
| Limiter actual es contador unitario con release idempotente | Mantener seguridad, agregar pesos/FIFO y tests de admisión atómica/cancelación en espera |
| FanIn crea un forwarder por stream | Bound relativo a cantidad de inputs, no un máximo global independiente; documentar presupuesto del caller |
| Batch pesa items, no bytes, y no retorna error runtime | Cambiar Config/weight/StreamResult antes de publicar; no agregar opciones ambiguas a firma vieja |
| TestLimiterCancelsWaitingAcquire cancela sin barrera de waiter encolado | Puede cubrir solo pre-cancelación; falta prueba determinista de cancelación tras bloqueo real |
| TestMapBoundsExecutionAndOutput verifica workers y cap(out) | No prueba admisión Submit ni memoria/keys ni invariantes todavía inexistentes |
| Benchmark solo tareas triviales | No sirve como SLA ni evaluación de backpressure, skew, leaks o latencia de I/O |
| Consumer guard solo usa Map | Debe compilar/ejecutar contratos nuevos Pool/Future/Batch/Limiter sin raíz antes de v1 |

Verificación realizada en esta revisión: `go test -race -count=20 -timeout=60s
./...` con workspace y proxy desactivados; repetido con GOMAXPROCS=1 y 4, ambos
correctos. `go vet ./...` correcto. Estas pruebas son sobre la API **experimental
existente**, no sobre código futuro. Se revisaron tipos de los seis fragmentos
Go declarativos de las especificaciones mediante go/parser/go/types, agregando
cuerpos AST vacíos solo en memoria para verificar declaraciones genéricas; sin
lógica, ejecución de consumidores nuevos ni prueba de comportamiento nuevo.

Se verificó por hash que 362 archivos de implementación/build/config/scripts
presentes antes de la revisión no cambiaron; los cambios de esta iteración son
documentales. Inventario cubierto 69/69, índice de contratos con 110 entradas
concretas (incluye paquetes/proveedores/testkits, no 110 implementaciones ni
necesariamente 110 go.mod). Referencias locales y formato diff revisados.


## Reconciliación con respuestas del arquitecto

Se retiran core obligatorio, constructor universal e IsRetryable. Se confirma
DefaultConfig con decoder seeded, cliente solo I/O, observación por wrappers y
ErrCommitOutcomeUnknown. Se corrige la compatibilidad que pretendía conservar
garantías diferentes: task_executor se congela/depreca y se elimina en la release
de migración, sin bridge. La ejecución de esa deprecación es trabajo de código
posterior; esta actualización solo modifica documentos.

No se modificó concurrency-api.md. Las referencias históricas a bridges/harness
en sus notas de migración no autorizan nuevos puentes semánticos ni dependencias
externas en tests: A10/D28 concretan cómo se ejecuta la migración. El legado se
conserva congelado hasta retirarlo, no como bridge nuevo.


## Cierre de dependencias documentales

El usuario confirmó que las respuestas suministradas bastan para validar y tomar
decisiones. Se concretaron las firmas de client en public-contracts y la matriz
de once categorías/evidencias E1–E7 en decisions. No se atribuyen esos detalles
a un archivo externo ni se requiere recibirlo. Quedan trabajo de implementación,
pruebas y autorización de release, no preguntas pendientes por answers.md.

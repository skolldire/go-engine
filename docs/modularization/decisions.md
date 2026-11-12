# Decisiones normativas de modularización

Fuente: decisiones D01–D24 existentes y respuestas del arquitecto suministradas
por el usuario en esta conversación. No se atribuyen a un RFC de Notion ni a
ADR 0002. No se inventan números D25/D31: no se recibió su texto numerado.
Las aclaraciones A01–A14 registran las catorce respuestas sin renumerarlas como ADR.

## Precedencia y resolución de conflictos

1. Decisiones del arquitecto D25+ y D01–D24 (las aclaraciones nuevas corrigen las anteriores cuando lo indican explícitamente).
2. public-contracts.md y concurrency-api.md.
3. modularization.md.
4. Documentos de semántica.
5. Inventario e índice, exclusivamente descriptivos.

El RFC de Notion y ADR 0002 no son normativos. D32: ante un conflicto no elige
quien implementa; se corrige el documento afectado antes de implementar ese
contrato. El usuario autorizó concretar detalles técnicos a partir de las
respuestas recibidas sin depender de un archivo answers.md. Esas concreciones
se registran aquí; no se atribuyen literalmente a un documento externo.

## D01–D24

| ID | Decisión |
|---|---|
| D01 | Configs locales sin map[string]any para clientes base; application root puede tener secciones raw |
| D02 | DefaultConfig explícito, decoder seeded; cero no se transforma en default cuando cero tiene significado |
| D03 | Context nil inválido; métodos con error lo retornan; utilidades puras no reciben ctx artificial |
| D04 | New puro salvo pool/limiter/rate limiter que administran sincronización local; Open para I/O, Run para loops/listeners |
| D05 | Sin hot reload en v1; cambios se aplican recreando recursos mediante aplicación |
| D06 | Parser profile estricto y stdlib; entorno y compatibilidad fuera del parser |
| D07 | Neutral API no expone SDK. Native adapters sí exponen tipos nativos donde aporta funcionalidad actual |
| D08 | Single SQL Tx se pasa explícitamente; no Tx escondida en context ni transacciones anidadas implícitas |
| D09 | Reintentos apagados en base; classifier requerido al habilitar retry; stop por defecto ante error desconocido |
| D10 | Fallos de cancelación/admisión no cuentan como fallo de dependencia del breaker |
| D11 | Pool tiene Future por aceptado; no callbacks ni cola de resultados obligatoria; ledger vivo únicamente |
| D12 | Peso lógico ponderado en admisión; output/heap arbitrario no están bajo un hard cap ofrecido por la biblioteca |
| D13 | Keyed pool local, round-robin por key lista, failure policy explícita; distributed ordering fuera de alcance |
| D14 | Stats pull, métricas externas, sin SDK/hook obligatorio dentro del scheduler |
| D15 | Idempotency/outbox/inbox/audit DBTX atómico; store fuera de Tx y lease worker usan API distinta |
| D16 | Reloj DB para leases; expiración de retención no permite borrar ejecuciones activas |
| D17 | TLS activado por defaults de transporte; modo insecure/desactivado solo explícito, sin inferir seguridad por profile |
| D18 | Hosts desconocidos se rechazan con 421, JWT inválido 401/permiso 403; no CORS como control de host |
| D19 | Migrations explícitas, ni auto-migrate ni cleanup destructivo en constructor |
| D20 | PostgreSQL y MySQL se prueban por separado; versión mínima establecida por matriz real de integración antes de publicar |
| D21 | Una sola política dueña del retry budget; wrappers/SDK no multiplican intentos ocultamente |
| D22 | concurrency estable implica v1.0.0 propio; versión del resto no cambia por ello |
| D23 | Mantener Go 1.27.4 declarado durante esta extracción; no elevarlo como efecto secundario |
| D24 | No hay nuevos paquetes vacíos ni APIs sin comportamiento para aparentar avance |

## Decisiones posteriores con identificador comunicado

| ID | Decisión |
|---|---|
| D26 | No existe módulo core ni lifecycle compartido. client es stdlib-only y solo lo importa quien decora. concurrency/utilities/config no importan client. El ensamblador ordena el cierre. |
| D27 | YAML se lee con go.yaml.in/yaml/v3 dentro de config/yaml. Mientras config/yaml no exista, consumidor usa config/json. |
| D28 | Stdlib-only incluye tests sin dependencias externas. Leaks con perfil estándar goroutineleak, no goleak. Integración pesada en modulo/integration con go.mod propio; fakes solo para adapters, en testkit/adaptador. |
| D29 | No etiquetar módulos independientes sin manifiesto de versiones. Responsable propone versión; usuario la aprueba. concurrency/v1.0.0 puede coexistir con 0.x. |
| D30 | Commit incierto devuelve ErrCommitOutcomeUnknown; su decisión es reconcile, nunca retry ciego. |
| D32 | Corregir documentos en conflicto antes de implementar; no permitir elección unilateral del implementador. |

## Aclaraciones recibidas

| Respuesta | Contrato confirmado |
|---|---|
| A01 | Precedencia anterior; Notion/ADR 0002 informativos. |
| A02 | No core; firmas de client concretadas en public-contracts a partir de Operation tipada, contexto/causas preservados e Invocation de etiquetas fijas. |
| A03 | Primero dependencias, tareas separadas. Disponible = en rama principal con consumidor externo pasando; para etiquetar dependencias deben tener tag resoluble con go get. concurrency no tiene dependencias. |
| A04 | Retirado constructor universal. New puro, Open ctx de startup, Run/Serve ctx de vida; pool/limiters excepción declarada. |
| A05 | DefaultConfig y decoder seeded. Ausencia conserva default; cero/false explícitos respetados. Literal Go se valida tal cual, sin punteros ni metadata. |
| A06 | Permisos por once categorías concretados abajo conforme a los bordes recibidos. Resilience compone sus herramientas; store importa capacidad+atomic+driver; exporter OTel+transporte; provider engine+herramienta+config. |
| A07 | client solo envuelve I/O, nunca función pura, tarea del pool o sentencia dentro de Tx. Operation tipada, contexto/causas preservados, Invocation solo etiquetas fijas. Retry apagado; classifier obligatorio del caller al habilitarlo. |
| A08 | Retirado IsRetryable. Adaptador reporta hechos, caller decide stop/retry/reconcile. Commit incierto según D30. |
| A09 | Observabilidad por operación con wrappers. slog solo donde se liste y para eventos internos. Componente no registra error que devuelve; errores de negocio no se elevan a error. |
| A10 | Si cambian garantías, ruptura explícita sin puente. task_executor se congela, depreca y retira en release de migración. |
| A11 | concurrency-api.md sin cambios; C1–C8 y gates §9 determinan aceptación. Adoptar client no cambia concurrency. |
| A12 | Consumidores minimal/composed; replaces solo módulo y deps declaradas. YAML por D27; mientras no exista usar config/json. |
| A13 | D28 para tests/harness/testkit; ningún fake exportado obligatorio para herramientas puras. |
| A14 | D29 para versión/aprobación. Siete evidencias operativas concretadas abajo, incluyendo tag real y gorelease. |

## Concreciones técnicas autorizadas por el usuario

Las respuestas de esta conversación son fuente suficiente. La ausencia de
answers.md no bloquea diseño, validación o implementación. Las siguientes
especificaciones son concreciones registradas del proyecto; respetan las
respuestas y no se presentan como transcripción de un RFC no recibido.
No cambian concurrency-api.md ni asignan números D25/D31 arbitrarios.

### Once categorías de dependencias permitidas

Cada módulo tiene categoría primaria y allowlist concreta de paths/versiones.
Las familias no autorizan importar todos sus miembros. Un import permitido no
elimina la obligación de justificar sus requisitos transitivos.

| Categoría | Ejemplos | Permisos y límites |
|---|---|---|
| 1. Fundaciones stdlib | utilities, config base, concurrency, client, logging, retry, ratelimit | Solo stdlib, también tests. concurrency/utilities/config no importan client. Ningún SDK ni engine. |
| 2. Fuente de configuración | config/yaml, config/viper, config/ssm | config + parser/fuente seleccionado; SSM solo su cliente. Sin clientes DB, engine o config de todas las herramientas. JSON/env son paquetes stdlib de config. |
| 3. Herramienta/adapter específico | validation, auth/jwt, circuitbreaker, router/chi, clientes AWS/HTTP/brokers/noSQL/RPC | Stdlib + SDK/library de su capacidad y contratos mínimos expresamente necesarios. client solo si decora I/O; no en parser/breaker puro. Sin engine/presets ni SDK de otro servicio. |
| 4. Contratos durables | idempotency, outbox, inbox, saga, audit | Stdlib + interfaces propias; sin drivers ni engine. Política se inyecta; no SDK de observabilidad. |
| 5. SQL neutral | database/atomic | Solo stdlib: database/sql/driver/context. Ningún driver, ORM, client middleware de sentencias o engine. |
| 6. Store/conector SQL por backend | database/postgres/mysql, capacidad/postgres/mysql, database/gorm/* | Driver/dialector seleccionado + atomic + contrato de capacidad cuando corresponda; store no importa otro backend. GORM solo en adapter GORM. |
| 7. Composición de políticas | resilience, client/resilience | client y herramientas concretas que compone (retry/breaker/concurrency/ratelimit según contrato), sin SDK o engine. No depende de todos los módulos por conveniencia. |
| 8. Instrumentación/puentes | client/logging, client/otel, integration/cloud/otel, telemetry/otel, logging/logrus | Contrato decorado + slog/library o SDK de señal seleccionado; no exporter/engine obligatorio. telemetry/otel no importa transportes. |
| 9. Exportadores | telemetry/otlphttp, otlpgrpc, prometheus, otellog | OTel + exporter/transporte requerido; sin transportar ambos OTLP implícitamente ni engine. |
| 10. Providers | engine/providers/* | engine + herramienta concreta + config para decode, contratos mínimos de dependencias declaradas. Nunca dependencia inversa herramienta→provider. |
| 11. Ensamblado/agregadores | engine, presets/*, integration/aws y ejemplos de composición | Engine mínimo solo contratos declarados; presets/fachadas agregan implementaciones seleccionadas explícitamente. Footprint mayor opt-in; no tools→agregador. |

health/router estándar usan contratos stdlib dentro de categoría 1; un adapter
con library concreta usa categoría 3. Testkits solo para adapters, en módulo
separado con sus tipos/SDK necesarios. Harness integration posee go.mod propio;
no amplía la allowlist del módulo mínimo. Cada consumer minimal/composed tiene
su propia lista, no hereda la unión de todas estas categorías.

### Siete evidencias para declarar v1

Esta es la lista operativa adoptada para el proyecto, consistente con los dos
requisitos expresos del arquitecto (tag real y gorelease) y los gates existentes:

| Evidencia | Contenido verificable |
|---|---|
| E1. Contrato y documentación | API/config/defaults/errores/lifecycle/limitaciones y migración cerrados; sin contradicciones normativas; concurrency cumple alcance C1–C8 y gates §9. |
| E2. Calidad funcional | Resultados unitarios, race, vet/lint, invariantes, fuzz/benchmarks pertinentes y leaks con goroutineleak; comandos/Go/plataforma registrados. |
| E3. Integración aplicable | Harness aislado y dependencias reales por backend; limpieza y recuperación. Para herramienta pura sin backend, prueba de composición aplicable y justificación de no requerir integración remota. Ninguna prueba skipped cuenta como pasada. |
| E4. Independencia local | Consumers minimal y composed con GOWORK=off, allowlists, replaces declarados y ejecución offline reproducible cuando aplique; no inicializan framework. |
| E5. Compatibilidad de release | Informe gorelease y revisión de métodos/tipos/config. Contra tag anterior cuando exista; primera release registra ausencia de base, sin inventarla ni silenciar diagnósticos. Cambios de garantías son ruptura sin puente. |
| E6. Preparación y autorización de versión | Manifiesto de versiones, dependencies con tags resolubles, versión propuesta por responsable, aprobación del usuario y commit/candidato exactos identificados. No crear tag para satisfacer un test sin esa autorización. |
| E7. Consumidor contra tag real | Descargar desde tag publicado sin replace local ni workspace, compilar/ejecutar casos públicos y guardar versión/sumas/resultado. Fallo implica incidente de release y corrección según versionado, no mover un tag inmutable. |

E1–E6 habilitan proponer/publicar el candidato con autorización; E7 ocurre tras
la publicación. Hasta E7 la verificación de publicación está incompleta: no se
inventa un tag para cerrar el checklist ni se llama completa una corrida con
replace. Estas siete evidencias no sustituyen ni relajan C1–C8/gates concurrency.

### Firmas de client

Las declaraciones concretas se fijan en public-contracts. client es un helper
stdlib para I/O tipado, sin Config, scheduler, retry ni lifecycle implícitos.
Invocation = Component y Operation estáticos; Do y Chain preservan T y las
causas de error. Se distinguen middleware de I/O y Task del pool: no comparten
obligatoriamente una interfaz ni una dependencia. Los SDK reportan hechos;
la política de retry/reconcile pertenece al caller.

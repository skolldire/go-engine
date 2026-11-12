# Contratos públicos y configuración por módulo

Estado: especificación para implementación; no declara APIs disponibles hoy.
Prevalecen las [decisiones del arquitecto](decisions.md); este documento y
[concurrency-api](concurrency-api.md) ocupan el siguiente nivel. Notion y ADR 0002
no son normativos. Antes de implementar un contrato en conflicto se corrige aquí
(D32); no se elige una alternativa por cuenta propia. Las firmas Go
incluidas son declaraciones de API (sin cuerpos), no código de implementación.
En tablas, `R`=requerido; `D:x`=valor de DefaultConfig; `O`=opcional sin efecto
si se omite. Un campo O de duración/int tiene cero literal permitido, salvo
validación explícita. Los campos no listados son unknown keys y se rechazan.
Los contratos se agrupan por familia sin reunir sus go.mod.

## Convenciones que aplican a todas las familias

- Módulo con Config ofrece `DefaultConfig() Config`, `Config.Validate() error` y
  constructor que valida/copia Config. Defaults se aplican al obtener DefaultConfig
  o al sembrar el decoder; no sustituyen cero explícito en un literal. Config es
  inmutable tras construcción. Funciones de utilidad no inventan Config vacío.
- Campos Go son PascalCase de los nombres snake_case listados (TLS, DSN, URI,
  URL, ID, JWKS son acrónimos). Tipos de tiempo = time.Duration; tamaños/pesos =
  int64; capacidades = int salvo SDK que exija otro tipo. Enums de config son
  strings con constantes y validación de conjunto, no strings arbitrarios.
- Contextos de operaciones siempre explícitos, no-nil. Open usa ctx para startup,
  no como lifetime global. Run/Serve/Dispatch usan su ctx para el loop. Cierre de
  recursos con contexto acotado por el dueño; no código que llame os.Exit/Fatal.
- ConfigError{Field,Reason} envuelve ErrInvalidConfig local al módulo; no import
  de error universal. Errores conservan causas con Unwrap/Is/As. Sin secretos en
  error, String, JSON diagnóstico, Stats o logs automáticos. Config de conexiones
  no expone serialización diagnóstica insegura.
- Dependencies contiene objetos/funciones, no nombres de secciones YAML. Referencias
  `*_ref` solo existen en ensamblado, no en Config neutral. Dependencias prestadas
  no se cierran ni reconfiguran salvo transferencia explícita documentada.
- No instrumentation/retry/auth automático detrás de un booleano en base clients.
  Decorators se construyen aparte; cada módulo tiene sus propios presupuestos.
- Objetos con recursos ofrecen Close/Shutdown idempotente. Wrappers de tipos
  nativos conservan interfaz de SDK solo en su módulo; neutral API usa stdlib.
- Cada firma que devuelve un recurso especifica ownership de bodies/rows/handles.
  No se materializan streams ilimitados para adaptar una firma a []byte.

## Fundaciones

<!-- contract: utilities -->
### utilities

Paquetes públicos `profile`, `strings`, `collections`, `pointers`, `defaults`.
Sin Config/Dependencies/Close. `profile.ParseScope(scope string) (Profile,error)`:
trim/lowercase y último token separado por `-`; solo local/test/stage/prod. Vacío
retorna local; token no reconocido ErrUnknownProfile; no lee SCOPE. Profile tiene
String/IsLocal/IsTest/IsStage/IsProd sin globals. Helpers conservados se enumeran
en el inventario de origen; descartar wrappers redundantes solo en ruptura
documentada. Map Keys/Values no ordenan; Merge devuelve nuevo map, último gana;
Ptr/ValueOrZero no hacen I/O. No helper abre archivos ni ejecuta comandos.

<!-- contract: config -->
### config

```go
// package config; imports: context

type Config struct {
    MaxSourceBytes int64
    MaxDepth int
    MaxExpansionDepth int
    RejectUnknown bool
}
type Document map[string]any
// Source devuelve datos propios de la fuente; el loader los copia.
type Source interface { Load(context.Context) (Document,error) }
type Resolver interface { Resolve(context.Context,string) (string,error) }
type Loader struct { /* privado */ }
type Snapshot struct { /* privado */ }
type Dependencies struct { Sources []Source; Resolver Resolver }
func New(cfg Config, deps Dependencies) (*Loader,error)
func (l *Loader) Load(ctx context.Context) (*Snapshot,error)
func (s *Snapshot) Revision() string
func (s *Snapshot) Decode(section string, target any) error
func (s *Snapshot) Has(section string) bool
```

MaxSourceBytes D:1 MiB, MaxDepth D:64, MaxExpansionDepth D:8, todos >0;
RejectUnknown D:true y false inválido en API nueva (legacy adapter puede tener
su modo explícito). Sources se mezclan en orden; cada fuente aplica byte limit
antes de devolver Document. Snapshot es copia inmutable; Revision es un ID aleatorio por Load exitoso,
no hash de contraseñas ni igualdad semántica del contenido. No usarlo para
probar que secretos no cambiaron ni para un CAS de configuración remota.
Decode requiere pointer no-nil, sección presente y tipo conocido; missing section
ErrSectionMissing, unknown keys ConfigError. No getter expone map mutable crudo.
Resolver recibe referencias con esquema registrado; no ejecuta shell. La fuente
entorno se snapshottea una vez por Load, no vuelve a consultar env por campo.

<!-- contract: config/json -->
<!-- contract: config/yaml -->
<!-- contract: config/viper -->
<!-- contract: config/env -->
<!-- contract: config/ssm -->
### Fuentes config/json, config/yaml, config/viper, config/env, config/ssm

Cada fuente produce `config.Source` con `New(cfg Config,deps Dependencies)
(Source,error)`. YAML usa exclusivamente go.yaml.in/yaml/v3 dentro del módulo opcional
config/yaml (D27). Mientras no exista se usa config/json en consumers.
JSON/env forman paquetes del módulo config (stdlib); **no son
go.mod adicionales**. YAML/Viper/SSM sí son módulos opcionales. El decoder base
no necesita Viper. Dependencies FS=fs.FS o reader proporcionado y Env=func(string)
(string,bool); nunca cwd/env global implícitos. Interfaces FS/SSM inyectadas hacen
reproducibles los tests.

| Fuente | Config pública exacta | Contrato/errores |
|---|---|---|
| json | path:string R, max_bytes:int64 D:1 MiB, required:bool D:true | JSON estricto sin claves duplicadas; required=false permite ausencia, no sintaxis inválida |
| yaml | path:string R, max_bytes:int64 D:1 MiB, max_aliases:int D:0, required:bool D:true | Duplicados rechazados; aliases desactivados por default; tags ejecutables prohibidos |
| viper | files:[]string R no vacía, max_bytes D:1 MiB, required D:true | Instancia local; traduce legacy lookup/merge explícito, no global singleton |
| env | bindings:map[path]var R, required:[]path O | Solo bindings declarados; ausente/empty distintos; no volcado de todo el entorno |
| ssm | paths:[]string R, decrypt:bool D:true, recursive:bool D:false, timeout D:5s, max_bytes D:1 MiB | Cliente SSM prestado + decoder del formato registrados en deps; paginación acotada; colisiones de keys error |

Secret references se resuelven con `Resolver` inyectado; no existe resolver AWS
dentro del módulo base. La revisión anterior sugería config/ssm obligatorio para
secrets: queda corregido a integración opcional. V1 sin reload/watcher.

<!-- contract: logging -->
<!-- contract: logging/logrus -->
<!-- contract: validation -->
### logging, logging/logrus, validation

| Módulo | Entrada/superficie pública | Config y validación |
|---|---|---|
| logging | New(Config,Dependencies) (*slog.Logger,error); deps Output io.Writer R, atributos estáticos/Redactor opcional | level enum debug/info/warn/error D:info; format json/text D:json; add_source bool D:false; redact_keys []string D:password,token,authorization,secret; max_value_bytes int D:4096 >0 |
| logging/logrus | NewHandler(*logrus.Logger,Config) (slog.Handler,error); AdaptLegacy(*slog.Logger) LegacyService | Config sin backend params: add_source D:false; logger nativo requerido y prestado; nunca Fatal/os.Exit por bridge |
| validation | New(Config,Dependencies) (*Validator,error); Struct(ctx,value) error; deps reglas nombradas opcionales | field_names json/go D:json, required_struct bool D:true; registro duplicado/regla inválida error de construcción; sin singleton global |

Redactor protege claves conocidas también dentro de mapas/structs hasta MaxDepth
interno 16 y redacta estructuras más profundas completas. Writer/Redactor lentos
son responsabilidad de logging/llamador; no se ejecutan bajo locks del scheduler.
Structured validation retorna FieldError{Path,Rule} sin valor secreto y Unwrap
original opcional; no convierte automáticamente errores a HTTP.

<!-- contract: client -->
<!-- contract: client/logging -->
<!-- contract: client/otel -->
<!-- contract: client/resilience -->
### client y decorators separados

D26: `client` solo usa stdlib y no existe `core/call` ni `core/lifecycle`.
Solo importa client quien usa sus decorators. No lo importan concurrency,
utilities ni config. Operation[T] conserva T, contexto y causa de error;
Invocation solo etiquetas estáticas de baja cardinalidad. Se envuelven llamadas
I/O, no operaciones puras, tareas del pool o sentencias dentro de una Tx.
Retry apagado por defecto; si se habilita requiere classifier del caller.

Concreción técnica autorizada por el usuario, coherente con las respuestas:

```go
// package client; imports: context

type Invocation struct {
    Component string
    Operation string
}
type Operation[T any] func(context.Context) (T,error)
type Handler[T any] func(context.Context,Invocation,Operation[T]) (T,error)
type Middleware[T any] func(Handler[T]) Handler[T]

func Chain[T any](next Handler[T], middleware ...Middleware[T]) (Handler[T],error)
func Do[T any](ctx context.Context, info Invocation, operation Operation[T],
    middleware ...Middleware[T]) (T,error)
```

No Config ni lifecycle: funciones puras de composición. Chain aplica outermost
primero; sin middleware devuelve next. next nil, middleware nil o que produzca
handler nil devuelve ErrInvalidCall local antes de invocar operación. Do valida
ctx/op no-nil y Component/Operation no vacíos de máximo 128 bytes cada uno;
invoca chain sobre un handler base que retorna exactamente operation(ctx).
Invocation se copia, sin metadata mutable ni payload. No normaliza nombres ni
introduce contexto background; sin middleware transmite el mismo ctx y error
original. Un wrapper que derive deadline conserva cancelación/valores del ctx
y causa de error; no debe sustituir un error de dominio por otro sin preservarlo.
No se capturan panics aquí ni se generan goroutines/retries/logs automáticamente.

Los nombres deben ser fijos/baja cardinalidad por contrato del caller; no IDs,
rutas resueltas, tenants o secretos. Cambiar Component respecto del campo Client
de la Invocation heredada se documenta como migración de tipos, no alias falso.
No se incorpora client a concurrency/utilities/config para probar esta cadena.

client/logging construye wrapper con *slog.Logger prestado y Config{Level string
D:debug, IncludeDuration bool D:true}; client/otel con tracer/meter nativos
prestados y Config{InstrumentationName string R}; client/resilience con policies
concretas y Config de composición descrito abajo. Cada wrapper SDK en módulo
propio; no un import de todos dentro del go.mod de client. El logger directo
solo registra eventos internos donde el contrato lo lista. Un componente no
registra un error que también devuelve; la observabilidad por operación pertenece
al wrapper, y errores de negocio no se elevan a nivel error.

<!-- contract: retry -->
<!-- contract: circuitbreaker -->
<!-- contract: resilience -->
<!-- contract: ratelimit -->
### retry, circuitbreaker, resilience, ratelimit

```go
// package retry; imports: context, time

type Decision string // stop, retry, reconcile
type Config struct {
    MaxRetries int
    InitialBackoff, MaxBackoff, TotalTimeout time.Duration
    Multiplier float64
    Jitter string // full, none
}
type Classifier func(error) Decision
func Do[T any](ctx context.Context,cfg Config,classify Classifier,
    operation func(context.Context)(T,error)) (T,error)
```

Retry MaxRetries D:0 >=0; InitialBackoff D:100ms >0, MaxBackoff D:10s >=initial,
Multiplier D:2 >=1 y finito, Jitter D:full, TotalTimeout D:0 >=0. Classifier
requerido cuando MaxRetries>0; reconcile detiene con ErrReconciliationRequired
que envuelve causa. Operación recibe contexto por intento; cancel antes de
iniciar no invoca fn. Esperas cancelables. Caps calculados sin overflow; RNG
no criptográfico, seguro en concurrencia. No retry después de deadline ni de
panic recuperado: panic no se captura aquí, se captura en boundary del pool/app.

Breaker `New(Config,Dependencies) (*Breaker,error)`,
`Execute(ctx, func(context.Context)(T,error)) (T,error)` es función genérica libre
`Execute[T](ctx,*Breaker,fn)` (Go no admite método con T adicional);
`State() State`, `Stats() Counts`. Dependencies.Classify func(error) bool R
(¿cuenta como fallo remoto?), sin logger. Config Name R, RequestThreshold D:5 >0,
FailureRate D:0.5 en (0,1], Interval D:60s >0, OpenTimeout D:30s >0,
HalfOpenMax D:1 >0. Estado propio closed/open/half_open; no tipo gobreaker en API
neutral. ErrOpen/ErrProbeLimit no fallan remotamente ni se reintentan por default.

Resilience `Wrap[T](fn client.Operation[T],cfg Config,deps Dependencies)
(client.Operation[T],error)`. Config Order enum `retry_then_breaker` R cuando
ambos existen, TotalTimeout D:0, AttemptTimeout D:0, AdmissionTimeout D:0 todos
>=0. Dependencies contiene RetryConfig opcional + classifier, Breaker opcional,
Limiter opcional y Weight >0 cuando exista, sin SDK. Admite orden único inicial:
retry exterior, breaker después, limiter por intento, attempt timeout al ejecutar.
Cambiar el orden del breaker respecto del legado es ruptura explícita; no
crear bridge que prometa las mismas garantías. Métricas
no entran en la librería base. Sin todos los decorators es fn con total deadline.

Rate limiter: `New(Config) (*Limiter,error)`, `Wait(ctx,n int) error`,
`Allow(n int) bool`. Config RatePerSecond float64 R finito >0, Burst int R>0.
Token bucket local con reloj monotónico; n en [1,Burst], espera cancelable; no
redis ni coordinador distribuido. No crea goroutine por llamada. Su deuda/cola
se limita a llamadas del caller, igual que semaphore; el caller controla el
número de goroutines esperando. No reordena una keyed operation para mejorar rate.

<!-- contract: concurrency -->
### concurrency

Contrato y configuración completos en [concurrency-api](concurrency-api.md).
Pool/stream/weighted limiter son tipos diferentes; no un Config de todos los
módulos. ID local no es clave idempotente durable. Sus errores de admisión no
cuentan como fallo remoto del breaker.

## SQL, stores y consistencia

<!-- contract: database/atomic -->
### database/atomic

```go
// package atomic; imports: context, database/sql, database/sql/driver, time

type DBTX interface {
    ExecContext(context.Context,string,...any) (sql.Result,error)
    PrepareContext(context.Context,string) (*sql.Stmt,error)
    QueryContext(context.Context,string,...any) (*sql.Rows,error)
    QueryRowContext(context.Context,string,...any) *sql.Row
}
type Config struct {
    MaxOpen, MaxIdle int
    MaxLifetime, MaxIdleTime, OperationTimeout time.Duration
}
type Pool struct { /* privado */ }
func Open(ctx context.Context,cfg Config,connector driver.Connector) (*Pool,error)
func Borrow(db *sql.DB) (*Pool,error)
func (p *Pool) DBTX() DBTX
func (p *Pool) Tx(ctx context.Context,opts *sql.TxOptions,
    fn func(context.Context,*sql.Tx) error) error
func (p *Pool) Stats() sql.DBStats
func (p *Pool) Close() error
```

MaxOpen R>0, MaxIdle D:0 en [0,MaxOpen], MaxLifetime/MaxIdleTime D:0 >=0,
OperationTimeout R>0. Open hace PingContext con deadline startup; error cierra
DB abierto. Borrow no reconfigura ni cierra db: no tiene ownership. Tx usa
*sql.Tx explícita, rollback error/panic (repropaga panic), commit solo en éxito.
No raw Begin SQL, nested transactions ni retry implícito. D30: un commit de
resultado incierto devuelve ErrCommitOutcomeUnknown local a database/atomic,
envolviendo la causa de driver con Unwrap. El caller decide reconcile; jamás
retry ciego. No se traduce cualquier error SQL a incertidumbre, solo cuando
no se puede determinar si el commit ocurrió. Queries devuelven rows
cuyo Close corresponde al caller; no cancelar un contexto derivado al devolver
Rows antes de que termine su consumo: el wrapper debe asociar cancelación a
Rows.Close o delegar ese presupuesto al caller. La v1 usa contexto del caller
en DBTX crudo; OperationTimeout se aplica a Tx/Open y helpers de consumo, no
promete interceptar un *sql.DB nativo que se entrega al usuario.

<!-- contract: database/postgres -->
<!-- contract: database/mysql -->
<!-- contract: database/gorm -->
### database/postgres, database/mysql, database/gorm

Cada driver `Open(ctx,Config) (*atomic.Pool,error)`,
`Connector(Config) (driver.Connector,error)` sin I/O y
`Capabilities() Capabilities`. Config común *local* de backend: DSN O XOR
Host/Port/Database/Username/Password, Pool atomic.Config R, ConnectTimeout D:5s>0.
Password puede ser vacío si auth elegida lo permite, nunca logueado. DSN completo
excluye campos estructurados/TLS contradictorios. Host DNS/IP incluyendo IPv6,
port [1,65535]. Schema/charset/timezone restringidos por dialecto.

Postgres: Port D:5432, TLSMode D:verify-full (disable/require/verify-ca/verify-full),
CAFile/CertFile/KeyFile O, Schema O; client cert/key juntos. MySQL: Port D:3306,
TLSMode D:verify-full (disable/require/verify-full), CAFile/CertFile/KeyFile O,
Charset D:utf8mb4, Timezone D:UTC. Credentials provider runtime inyectado en deps
si no password; no config env lookup automático. No config `Type` en atomic:
Type lo resuelve registry de la aplicación. No failover/reconnect infinito
implementado por wrappers fuera del driver.

GORM: `Open(ctx,Config,dialector gorm.Dialector) (*Client,error)`; Config
Pool atomic.Config R, TablePrefix O, LogLevel D:warn; OperationTimeout es Pool.OperationTimeout, sin campo duplicado;
sin AutoMigrate. `WithContext`, `Create`, `First`, `Find`, `Update`, `Delete`,
`Count`, `Exec`, `Transaction`, `Ping`, `Close`, `DB` preservan superficie nativa.
Migrate solo comando explícito, no constructor. Dialectors PostgreSQL/MySQL en
módulos `database/gorm/postgres`/`database/gorm/mysql` opt-in, Config de su backend;
no hace falta que usuario de SQLC importe GORM. Tipos nativos no se unifican con
atomic.DBTX mediante casts inseguros.

<!-- contract: idempotency -->
<!-- contract: idempotency/postgres -->
<!-- contract: idempotency/mysql -->
### idempotency y stores por dialecto

```go
// package idempotency; imports: context, time

type Config struct { Retention time.Duration; MaxResponseBytes int64 }
type Request struct { Scope, Key, Fingerprint string }
type Record struct {
    Request Request
    State string // in_progress, completed, uncertain
    Version uint64
    Response []byte
    ExpiresAt time.Time
}
type Claim struct { Record Record; Token string; Acquired bool }
type Store interface {
    Claim(context.Context,Request) (Claim,error)
    Complete(context.Context,string,[]byte) error // token, respuesta
    MarkUncertain(context.Context,string) error
    Lookup(context.Context,Request) (Record,error)
}
```

Scope/Key/Fingerprint R no vacíos, máximo 256 bytes cada uno; fingerprint
algoritmo/versión explícitos en aplicación. Retention R>0, MaxResponseBytes R>0. Scope/Key se comparan byte a byte (collation binaria en
MySQL), sin case-folding/trim implícito; índice único debe preservar esa semántica.
Store SQL transaccional se construye `New(tx atomic.DBTX,cfg Config)(Store,error)`
con Table string D:idempotency_keys añadido al Config del adapter
y no Begin/Commit. Claim unique(scope,key), compara fingerprint antes de replay;
conflicto ErrConflict; in_progress devuelve ErrInProgress, no Execute nuevo.
Completed devuelve copia respuesta; false Acquired no autoriza repetir.
Token opaque firmado por ownership/version en store, no generado por caller.
Complete verifica token, filas afectadas y tamaño. ErrStaleOwner/ErrNotFound.
Store corto transaccional no tiene LeaseDuration ni recovery de claim abandonado:
rollback elimina el claim. Trabajo externo/largo usa saga/outbox y dedup downstream,
no reusa este contrato como distributed lock. Uncertain preserva registro para
reconciliación; no caduca como autorización para ejecutar sin criterio de negocio.
Cleanup explícito por adapter borra completed expirados en batch con deadline;
no in_progress/uncertain. Schema names SQL se validan como identificadores, no params.

<!-- contract: outbox -->
<!-- contract: inbox -->
<!-- contract: outbox/postgres -->
<!-- contract: outbox/mysql -->
<!-- contract: inbox/postgres -->
<!-- contract: inbox/mysql -->
### outbox, inbox y adapters

```go
// package outbox; imports: context, time

type Config struct {
    BatchSize, MaxAttempts int
    LeaseDuration, LeaseMargin, PublishTimeout, AckTimeout, PollInterval time.Duration
    MaxEventBytes int64
}
type Event struct {
    ID, Topic, Key, ContentType string
    Sequence uint64
    Body []byte
    Headers map[string]string
}
type Writer interface { Append(context.Context,Event) error }
type Leased struct { Event Event; Token string; Attempt int; ExpiresAt time.Time }
type Store interface {
    Claim(context.Context,int) ([]Leased,error)
    Ack(context.Context,string) error // token
    Fail(context.Context,string,error,time.Time) error // token, error, next_at
    LeaseDuration() time.Duration
    Park(context.Context,string,error) error
}
type Publisher interface { Publish(context.Context,Event) error }
type Dependencies struct {
    Store Store
    Publisher Publisher
    NextAttempt func(attempt int,err error) (next time.Time,terminal bool)
}
func Dispatch(ctx context.Context,cfg Config,deps Dependencies) error
```

Writer es transaccional: adapter.NewWriter(tx atomic.DBTX,cfg WriterConfig)
(Writer,error), WriterConfig MaxEventBytes R>0, Table D:outbox. Append usa ID
unique y payload conflict explícito, no ejecución remota. Worker Store es otro
objeto: adapter.NewStore(pool *atomic.Pool,cfg StoreConfig)(Store,error), pool
prestado. StoreConfig LeaseDuration R>0 y Table D:outbox. Claim usa su transacción
corta, clock DB y tokens fence; no publica bajo lock SQL. BatchSize R>0,
MaxAttempts R>0, PublishTimeout R>0 y <LeaseDuration, PollInterval D:1s>0.
MaxEventBytes R>0; Body+headers tienen límite, Header count máximo 64/keys 128 bytes.
Dispatch secuencial en v1 (batch fetch, publish uno a uno), context-cancelable;
no worker setting oculto. Claim lease cubre el tiempo del batch completo:
BatchSize*(PublishTimeout+AckTimeout) + LeaseMargin < LeaseDuration, con cálculo seguro sin
overflow. AckTimeout R>0 y LeaseMargin R>0 son parte de Config; Store.LeaseDuration debe coincidir
con Config o falla Dispatch antes de reclamar. No lease renewal en v1. ExpiresAt es tiempo DB para diagnóstico; el token
fence se verifica con reloj DB, no con confianza en reloj local del worker. La
desigualdad y el margen son presupuesto, no prueba de que una red particionada
respete el lease: redelivery y efectos duplicados siguen siendo posibles. Los
contextos de Ack/Fail están acotados por AckTimeout y el remanente del lease.
Al cancelar Dispatch tras publish, no confirmar si se perdió ownership; queda
redelivery posible. No cleanup side effect con contexto global ilimitado.
MaxAttempts es budget durable, no retry interno por publicación; el callback
NextAttempt es obligatorio, produce next_at UTC o terminal. Error persistiendo
Ack/Fail se devuelve sin borrar evento. Si se agota MaxAttempts se aparca terminal
independientemente de NextAttempt; Store.Park aplica la transición y verifica
ownership. Un claim vencido no permite un Ack/Park posterior del dueño viejo.

No orden por agregado prometido por outbox genérico v1: Key/Sequence preservados,
pero paralelo entre dispatchers puede reordenar. Si negocio lo exige, un adapter
Ordering=per_key debe impedir reclamar secuencias posteriores mientras la anterior
no sea terminal; **fuera de v1**, no usarlo para ordered ledger hasta implementarlo.
DLQ = estado terminal consultable, no envío opcional que borra fila sin ack.
Errores ErrStaleOwner/ErrLeaseExpired/ErrPublishUnknown exigen recuperación y
redelivery, no afirmación exactly-once. Máximos de retención por Table cleanup
se configuran en tarea administrativa, nunca en constructor.

Inbox API `New(tx atomic.DBTX,Config)(Store,error)` en adapter;
base `Store.Accept(ctx,ConsumerID,EventID,Fingerprint) (fresh bool,err error)`.
Config Retention R>0, MaxKeyBytes D:256; constraints unique(consumer,event).
Accept se confirma **con** update de negocio en la misma tx; rollback deshace
ambos. Duplicado fingerprint diferente ErrConflict. Cleanup explícito y retención
mayor al horizonte máximo de redelivery declarado por aplicación. Inbox no usa
lease ni ack del broker por sí solo; ack después de commit corresponde al handler.

<!-- contract: saga -->
<!-- contract: saga/postgres -->
<!-- contract: saga/mysql -->
### saga y adapters

API base: `New(cfg Config,deps Dependencies)(*Coordinator,error)` con
Dependencies{Store Store; Steps []Step; Classify func(error) Decision},
Decision local enum stop/retry/reconcile,
`Start(ctx,id,definitionVersion string,input []byte)(Snapshot,error)`,
`Resume(ctx,id string)(Snapshot,error)`, `Inspect(ctx,id)(Snapshot,error)`.
Step{Name string, Execute func(context.Context,StepInput)([]byte,error),
Compensate func(context.Context,StepInput) error}. StepInput conserva SagaID,
StepID, IdempotencyKey, Payload y Compensation bool. Compensate requerido para
paso con efectos reversibles, y ausencia explícita indica punto no compensable;
no inventar rollback automático para un paso irreversible.

Config StepTimeout/LeaseDuration R>0 con StepTimeout<LeaseDuration,
MaxAttempts/MaxCompensationAttempts R>0, MaxPayloadBytes R>0,
CheckpointTimeout R>0, InitialBackoff D:100ms>0, MaxBackoff D:10s>=initial,
Jitter fijo full; Dependencies.Classify R; Decision es enum local para no forzar retry go.mod,
CompensationTimeout R>0; max(StepTimeout,CompensationTimeout)+CheckpointTimeout
<LeaseDuration validado sin overflow. Secuencia de pasos y definitionVersion
inmutables por instancia; cambio incompatible requiere versión nueva, no reusar
un ID existente. Estados pending/running/compensating/completed/compensated/
manual_intervention/uncertain; no completed después de fallar una compensación.

Store `Create(ctx,Instance) error; Load(ctx,id)(Snapshot,error);
Acquire(ctx,id)(Lease,error); Checkpoint(ctx,token,expectedVersion,Transition)
(Snapshot,error); Release(ctx,token) error`. Adapter NewStore recibe pool prestado
y Config Table D:sagas, lease config del coordinator; transacciones internas
cortas de checkpoint, nunca I/O remoto bajo Tx. Version/owner fence obligatorio;
lease DB time; retries persistidos con next_at. V1 ejecuta un paso por Resume;
no loop de retries sleeping con lease abierto indefinidamente. Retry persiste
NextAt calculado con full jitter en base a MaxAttempts, backoff y classifier;
stop termina o inicia compensación, reconcile marca uncertain, error desconocido
se trata como stop. El callback de clasificación no produce NextAt arbitrario. Compensar exitosos
en reversa, cada transición durable; unknown remote outcome => uncertain hasta
reconcile explícito administrativo, no replay automático.

<!-- contract: audit -->
<!-- contract: audit/postgres -->
<!-- contract: audit/mysql -->
### audit y adapters

`Writer.Append(ctx,Record) error`, `Reader.Query(ctx,Filter,Cursor)(Page,error)`.
Record{ID,Actor,Action,Resource,Tenant,Outcome,CorrelationID string;
OccurredAt time.Time; Metadata map[string]string}. Adapter `NewWriter(tx,Config)`
presta el mismo Tx de negocio; reader recibe pool. Config MaxRecordBytes R>0,
Retention R>0, Table D:audit_log; Filter exige tenant/scope explícito,
PageSize R[1,1000], Cursor opaque estable por (recorded_at,ID). RecordedAt lo
asigna DB; OccurredAt lo aporta caller UTC, no confiar en él como orden total.

Append rechaza ID duplicado con contenido diferente; record inmutable, sin API
Update/Delete de negocio. Borrado por retención administrativo independiente
con acceso explícito. ErrRecordTooLarge/ErrConflict; sin nombres de campos
secretos en detalles. DDL/privilegios hacen append-only; interfaz Go no prueba
tamper-resistance. Auditoría obligatoria falla la tx, observación best-effort
se entrega por otro canal explícito, no un bool que silencie errores financieros.

## HTTP, autenticación y salud

<!-- contract: health -->
<!-- contract: router -->
<!-- contract: router/chi -->
<!-- contract: router/errors -->
<!-- contract: router/operations -->
<!-- contract: auth/jwt -->
<!-- contract: auth/jwt/http -->
### health, router, operations, errors, JWT

| Módulo | Superficie pública estable objetivo | Config y valores |
|---|---|---|
| health | New(Config)(*Registry,error); Register(name string,Checker) error; Live() bool; Ready(ctx)(Report,error); Check(ctx)(Report,error) | Timeout D:5s>0, MaxParallel D:4>0, MaxChecks D:128>0; Checker.Check(ctx) error; duplicado ErrDuplicateCheck; no driver |
| router | New(Config)(*Mux,error); Handle(method,host,pattern string,http.Handler) error; Use(...Middleware) error antes de Freeze; Freeze() error; ServeHTTP | AllowedHosts []string R, TrustedProxies []CIDR O; CORS opcional {Origins,Methods,Headers []string; Credentials bool D:false; MaxAge D:0}; UnknownHostStatus fijo 421; host wildcard etiqueta única; sin default public wildcard |
| router/chi | New(Config)(*Adapter,error); Native() *chi.Mux; Handler() http.Handler | Config del router; tipos chi solo aquí; routes host en el neutral no se traducen a catch-all |
| router/errors | NewResponder(Config)(*Responder,error); Write(ctx,w,error) error | IncludeRequestID D:true, MaxDetailsBytes D:4096>0; deps Classify func(error) Problem, slog logger O; Problem{Status,Code,Title,Details}; nunca cause privada al cliente |
| router/operations | New(Config,Dependencies)(http.Handler,error) | Ping D:true, Health D:true, Pprof D:false; MaxProfileDuration D:30s>0; deps health checker si Health, Metrics http.Handler O; paths /ping,/live,/ready,/deps,/debug/pprof/*,/metrics; duplicados rechazados |
| auth/jwt | NewVerifier(Config,Dependencies)(*Verifier,error); Verify(ctx,token)(Claims,error); RequireGroup(Claims,group) error | Issuer/Audience/JWKSURL R; Algorithms D:[RS256] allowlist explícita; CacheTTL D:1h>0; RefreshTimeout D:5s>0; ClockSkew D:30s>=0; MaxTokenBytes D:16KiB; MaxJWKSBytes D:1MiB; StaleGrace D:0; deps http.Client prestado R |
| auth/jwt/http | Middleware(verifier,Config)(func(http.Handler)http.Handler,error); ClaimsFromContext(ctx)(Claims,bool) | SkipPaths []string O (matching exacto/subruta declarado), RequiredGroups []string O, Header D:Authorization, Scheme fijo Bearer; sin logger/engine |

Registry health registro antes de freeze por ensamblado o protegido concurrente;
Report inmutable {Live,Ready bool; Checks []CheckResult}. Cada check contexto
acotado; cancelar no mata checker no cooperativo. MaxParallel limita código
real vivo: no reemplazar checker atascado con nuevos goroutines en cada poll;
reutilizar slots y responder timeout/degraded. Sin cache de salud ilimitada.

Mux es handler; servidor opcional `Serve(ctx,ServerConfig,handler) error` con
Listen R, ReadHeaderTimeout D:5s>0, ReadTimeout D:0>=0, WriteTimeout D:30s>=0,
IdleTimeout D:120s>0, ShutdownTimeout D:15s>0, MaxHeaderBytes D:1MiB>0;
TLS cert/key o listener TLS suministrado, no provisiona certificados. HandlerTimeout
O=0; timeout por middleware explícito. Serve abre listener solo al invocarse,
propaga fallos y drena; no usa signal.Notify implícito. Operaciones usa otro
ServerConfig con WriteTimeout superior a MaxProfileDuration, validar al ensamblar.
Construcción de routers nunca abre red. Hosts aceptan ASCII/A-labels, lowercase
y puerto separado; rechazar unicode crudo, normalizar un punto final DNS y
rechazar host inválido. Wildcard solo cubre una etiqueta. CORS origins=* junto
con credentials=true es configuración inválida. JWT header/cookie/body nunca
se usa para seleccionar un host permitido.  Use/Handle después de Freeze ErrFrozen;
ServeHTTP requiere Freeze (si no, respuesta 503), sin auto-freeze racy al primer hit.

JWT Claims conserva Subject/Issuer/Audience/Expiry/Groups y claims custom copy;
restringir tamaño; refresh coalesced acotado, desconocido kid error tras un refresh,
stale desactivado por default. 401 sin token/invalid, 403 sin grupo, causas internas
no al cliente. auth/jwt/http depende solo verifier y stdlib; no router/chi/AWS.

<!-- contract: httpclient -->
<!-- contract: httpclient/resty -->
### Clientes HTTP

`httpclient.New(Config,Dependencies)(*Client,error)`,
`Do(ctx,*http.Request)(*http.Response,error)`, `CloseIdleConnections()`.
Config BaseURL R http(s), RequestTimeout D:10s>0, ConnectTimeout D:5s>0,
ResponseHeaderTimeout D:5s>0, IdleTimeout D:90s>0, MaxIdleConns D:100>0,
MaxIdleConnsPerHost D:10>0, MaxConnsPerHost R>0, MaxResponseBytes R>0,
FollowRedirects D:false, MaxRedirects D:0>=0, ProxyURL O,
TLS {RootCAFile,CertFile,KeyFile O; InsecureSkipVerify D:false}.
Dependencies.RoundTripper O prestado; si se proporciona, config de transport
contradictoria error en vez de mutarlo. Redirects autorizados mantienen política
host/credenciales; nunca copiar Authorization a otro origen sin regla explícita.
Response.Body es streaming limitado, Close y consumo del caller; cancel timeout
vivo hasta Close/EOF, no cancel defer al retornar headers. No retry por default.

Resty `New(Config,Dependencies)(Service,error)` preserva R(ctx) *resty.Request y
Get/Post/Put/Patch/Delete con respuesta incluso ante status no-2xx. Config =
BaseURL R, Timeout D:10s, transport budgets anteriores; native resty.Client
prestado por deps O. Retry y breaker solo wrappers/compat Config; v1 nativa no
tiene EnableLogging ni WithResilience. Resp non2xx es transporte exitoso con
respuesta; helper puede envolver StatusError{Status,Response} conservando body
ownership. Replay POST/PATCH necesita idempotency policy expresa; bodies no
replayable y respuestas grandes no se bufferizan silenciosamente para retry.

## Stores no SQL

<!-- contract: database/redis -->
<!-- contract: database/mongodb -->
<!-- contract: database/memcached -->
### Redis, MongoDB, Memcached

`Open(ctx,Config,Dependencies)(*Client,error)`, Ping(ctx), Close() error para redis/memcached, Disconnect(ctx) error para mongodb; Native() devuelve
SDK explícito. Dependencies acepta native client prestado como alternativa a
Open; no se muta su pool. Las operaciones existentes se conservan con contexto,
fuera de los toggles de logging/resilience de la capa heredada.

| Módulo | Config (campos no declarados en otros lugares no aparecen) | Métodos/validación |
|---|---|---|
| redis | Host R; Port D:6379; DB D:0>=0; Username/Password O; Prefix O; PoolSize R>0; DialTimeout/ReadTimeout/WriteTimeout D:5s>0; OperationTimeout D:5s>0; TLS{Enabled D:true,ServerName/CAFile O} | Get/Set/SetNX/Del/Exists/Expire/TTL/Incr/IncrBy/HGet/HSet/HGetAll/LPush/RPop/LRange/ZAdd/ZAddMulti/ZScore/ZRem/ZRange/SAdd/SMembers/SIsMember/SRem/SCard/SAddWithExpire/Pipeline/TxPipeline según interfaz real actual; miss propio envuelve redis.Nil |
| mongodb | URI R; Database R; MaxPoolSize R>0; MinPoolSize D:0<=max; ConnectTimeout D:5s>0; OperationTimeout D:10s>0; TLS config nativa validada por URI | Database/Collection/Ping/Disconnect; sesiones/transacciones Mongo futuras explícitas, no simularlas como SQL |
| memcached | Servers []host:port R no vacía; MaxIdleConns R>0; Timeout D:5s>0; Prefix O; MaxItemBytes R>0 | Get/Set/Delete/Add/Replace/Increment/Decrement/GetMulti/FlushAll; miss/cas conflict explícitos; native SDK puede no tener I/O cancelable |

No prometer cancelación fuerte donde SDK no la ofrece (especialmente Memcached).
El adapter verifica ctx antes de llamar y usa deadline nativo; no lanzar
unbounded goroutines para simular Context. Documentar bound real del I/O por
backend. TTL de Set es parámetro de operación, 0 tiene semántica nativa explícita;
no default de expiración que elimine datos por sorpresa en API nueva.

## Mensajería y RPC

<!-- contract: messaging/kafka -->
<!-- contract: messaging/rabbitmq -->
<!-- contract: rpc/grpc -->
### Kafka, RabbitMQ y gRPC

Kafka `OpenProducer(ctx,Config)(Producer,error)`,
`OpenConsumer(ctx,Config)(Consumer,error)` y combinador opcional Client;
Publish(ctx,...Message) error, Consume(ctx,Handler) error, Close(ctx) error.
Message preserva Topic/Key/Value/Headers/Partition/Offset/Time. Handler recibe ctx.
Config Brokers []string R, Topic R, GroupID R solo consumer, DLQTopic O,
FetchMaxBytes R>0, BatchMaxBytes R>0, MaxWait D:1s>0, RequiredAcks D:all,
Async D:false, CommitInterval D:0, TLS{Enabled D:true,CAFile/CertFile/KeyFile/ServerName O}, SASL opcional
{Mechanism enum plain/scram256/scram512 R, Username/Password R},
MaxRetries D:0, Backoff Config opcional con classifier solo handler retry habilitado.
No CommitMessages sobre offsets posteriores de una partición con un fallo no
resuelto; stop/retry o park en DLQ confirmado antes de avanzar. Rebalance suspende
admisión, drena lo permitido y no confirma trabajo fuera de ownership. V1 ack
secuencial por partición, sin paralelizar dentro de una partición a escondidas.
Async=true expone error delivery callback/canal **acotado** propiedad caller y
no confirma durable success al devolver Publish. Close flush con deadline.

Rabbit `Open(ctx,Config)(*Client,error)`, Publish(ctx,Message) error,
Consume(ctx,queue,Handler) error, DeclareQueue/DeclareExchange/BindQueue,
Close(ctx) error. Handler(ctx,Delivery) error en nueva API (no callback sin ctx).
Config URL R con esquema amqps; amqp solo con AllowInsecure D:false cambiado
explícitamente a true, TLS{CAFile/CertFile/KeyFile/ServerName O},
Prefetch R>0, Consumers R>0, ConfirmPublish D:true, AutoAck D:false,
ConnectTimeout D:5s, PublishTimeout D:5s, ReconnectBackoff opcional,
MaxDeliveryAttempts R>0, DLQExchange O, RequeueOnTransient D:false.
Ack/nack tras outcome; no ack automático en panic/failure. Confirm publish
requiere ack broker, cancellation antes de confirm puede ser outcome unknown;
reconnect no repite writes sin idempotency. Named queues/exchanges y bindings
son datos del app, no se crean topologías extra ocultas. Native Delivery escape
hatch conserva funciones específicas en módulo Rabbit, no contrato neutral.

`rpc/grpc` paquetes client y server en un solo módulo (mismo SDK),
client.Open(ctx,ClientConfig,Dependencies)(*Client,error), Invoke(ctx,...),
Connection() *grpc.ClientConn, Close(); Server.New(ServerConfig,Dependencies)
(*Server,error), Register(fn func(*grpc.Server)) error antes de Serve,
Serve(ctx) error, Shutdown(ctx) error. ClientConfig Target R, ConnectTimeout
D:5s>0, CallTimeout D:5s>0, WaitForReady D:false, MaxRecvBytes/MaxSendBytes R>0,
TLS{Mode D:verify,ServerName/CAFile/CertFile/KeyFile O}; insecure explícito.
ServerConfig Listen R, ShutdownTimeout D:15s>0, MaxConcurrentStreams R>0,
MaxRecvBytes/MaxSendBytes R>0, TLS análogo (server cert/key R si TLS).
Dependencies SDK options/interceptors O, sin globals. Construcción no escucha;
Close/Shutdown no hacen os.Exit; timeout de graceful stop fuerza stop y reporta
ErrForcedShutdown sin afirmar que desaparecieron goroutines de usuario.

## AWS y fachadas

<!-- contract: aws/config -->
<!-- contract: aws/s3 -->
<!-- contract: aws/sqs -->
<!-- contract: aws/sns -->
<!-- contract: aws/ses -->
<!-- contract: aws/ssm -->
<!-- contract: aws/cognito -->
<!-- contract: aws/dynamodb -->
<!-- contract: aws/lambda -->
### Config AWS y clientes por servicio

aws/config `Load(ctx,Config,Dependencies)(aws.Config,error)`:
Region R, Profile O, Endpoint O (http solo explicit AllowHTTP D:false),
ConnectTimeout D:5s, OperationTimeout D:10s, SDKMaxAttempts D:1>=1,
CredentialsSource enum default_chain/profile/provider D:default_chain.
Dependencies.Credentials aws.CredentialsProvider O requerido para provider,
HTTPClient SDK interface O prestado. Env/shared files/roles solo a través de la
cadena AWS **explícitamente elegida aquí**, no lectura general de config dentro
de clientes. Sin AccessKey/SecretKey en log ni defaults de credenciales estáticas.

Cada cliente `New(cfg Config,awsCfg aws.Config)(Service,error)` no abre red;
no Close si solo usa transporte prestado, health solo en probes explícitos.
Config base de cada servicio OperationTimeout D:10s>0; NativeClient O en deps
alternativo a awsCfg para pruebas/composición. SDK retries=1 por default o política
SDK opt-in explícita con budget; ninguna capa WithResilience importada por cliente.
Las interfaces de operaciones específicas del SDK permanecen públicas solo en
su módulo. No todos retornan el mismo any.

| Servicio | Config específica además de timeout | Métodos públicos conservados / nueva superficie |
|---|---|---|
| s3 | Bucket R, Endpoint O, MaxObjectBytes R>0; multipart concurrency R si se habilita | Put/Get/Delete/Head/List/Copy, PresignedGET/PUT; body stream caller-close, expiración por llamada validada |
| sqs | Endpoint O, QueueURL O (R por operación si ausente), WaitTime D:0 [0,20s], MaxMessages D:1 [1,10] | Send/SendJSON/Receive/Delete/CreateQueue/DeleteQueue/List/GetURL; receipt handle preservado; no delete antes de negocio commit |
| sns | Endpoint O, MaxMessageBytes R>0 | Topic/subscription/publish/JSON, SMS/bulk/attributes/opt-out y platform endpoints; no reducir SNS a Publisher solo |
| ses | Endpoint O, MaxMessageBytes R>0 | Email/raw/bulk/attachments, quota/stats, verify/delete/list identities; resultados parciales tipados por destinatario |
| ssm | Endpoint O, MaxParameterBytes R>0, DecryptDefault D:false | Get/GetMany/ByPath/Put/Secure/Delete/History/Tags/Exists; fuente config aparte puede usar decrypt=true explícito |
| cognito | UserPoolID/ClientID/JWKSURL R, ClientSecret O, MaxTokenBytes D:16KiB, TokenClockSkew D:30s | Register/confirm/auth/refresh/logout/password/MFA/groups y validación token; validar issuer/userpool/client audience sin duplicar JWKS inseguro |
| dynamodb | Endpoint O, TablePrefix O, MaxBatchItems R>0 validado contra operación SDK | Typed CRUD/query/scan/batch/TransactWrite y key helpers; no límite universal falso para todas las operaciones |
| lambda | Endpoint O, MaxPayloadBytes R>0, InvocationType D:RequestResponse enum SDK permitido | Invoke(ctx,Request)(Response,error); distinguir error de función, de transporte y outcome async; FunctionName R por request |

Las firmas exactas de las operaciones existentes se trasladan desde los Service
interfaces y métodos listados en [inventario](inventory.md), salvo toggles de
logging/resilience retirados mediante ruptura explícita cuando cambien garantías. Los DTO específicos AWS permanecen
nativos; no se requieren nuevos métodos públicos para reflejar cada campo de
Config. Lambda cliente dedicado es nuevo: Request{FunctionName,Qualifier string;
Payload []byte; InvocationType string}; Response{StatusCode int; Payload []byte;
FunctionError,RequestID string}. Invoke async éxito no implica éxito del negocio.

<!-- contract: integration/cloud -->
<!-- contract: integration/cloud/logging -->
<!-- contract: integration/cloud/otel -->
<!-- contract: integration/aws -->
<!-- contract: integration/aws/s3 -->
<!-- contract: integration/aws/sqs -->
<!-- contract: integration/aws/sns -->
<!-- contract: integration/aws/ses -->
<!-- contract: integration/aws/ssm -->
<!-- contract: integration/aws/lambda -->
<!-- contract: integration/aws/events -->
### Fachada opt-in

integration/cloud conserva Client.Do(ctx,*Request)(*Response,error), Middleware,
Request/Response, JSON helpers, Error{Code,Message,Retriable,Cause,StatusCode,
Metadata}. Config RequestTimeout D:10s>0, MaxBodyBytes R>0;
New(Config,Handler) (Client,error). Retriable heredado no activa retry solo.
Metadata/header count máximo 64, tamaño bajo MaxBodyBytes; copiar mapas de entrada
al retenerlos. No crecimiento de cache/history de requests. Request de fachada agrega
ID/Key/CorrelationID string opcionales para conservar identidad entrante; no
los deriva de body ni reutiliza RequestID como clave durable automáticamente.

integration/aws/<servicio> `New(Config,native Service)(cloud.Client,error)`;
Config de fachada anterior, Endpoint no duplicado (ya está en cliente), solo
operaciones soportadas por ese servicio. El agregado integration/aws recibe
`map[string]cloud.Client` y Config{MaxBodyBytes R>0,RequestTimeout D:10s}; se
registran nombres únicos, operación desconocida ErrUnsupportedOperation; no
construye todos los SDK desde un aws.Config implícito. Si cambia las garantías del constructor antiguo, se documenta una ruptura
explícita; no se crea bridge de compatibilidad para ocultarla.

integration/aws/events `NormalizeSQS/NormalizeSNS/NormalizeAPIGateway(event)
([]cloud.Request,error)` con Config MaxEventBytes R>0, MaxRecords R>0;
normalizadores construidos con New(Config). No red/Close; preservan event IDs,
headers, key/trace metadata y errores por registro sin volver a ejecutar handler.
Logging/OTel de fachada tienen Config análoga a client/logging y client/otel;
wrappers no mutan Request/Response prestados ni loguean payload/credenciales.

## Observabilidad

<!-- contract: telemetry/otel -->
<!-- contract: telemetry/otlphttp -->
<!-- contract: telemetry/otlpgrpc -->
<!-- contract: telemetry/prometheus -->
<!-- contract: telemetry/otellog -->
### Providers, exporters y logs

`telemetry/otel.New(Config,Dependencies)(*Provider,error)` con
Tracer(name)/Meter(name) tipos OTel, Shutdown(ctx) error; los consumidores que
no eligen OTel usan solo funciones/hook interfaces locales. Config ServiceName R,
ServiceVersion O, Environment O, SamplingRatio D:1 en [0,1], MaxAttributes D:128>0,
MaxAttributeBytes D:4096>0. Dependencies TraceProcessor y MetricReader opcionales;
si no existen, señal no exportada (no crear exporter global). Registrar globals
solo `InstallGlobal(provider) (restore func(),error)` opt-in y con ownership
único del ensamblador; no default global de New.

OTLP HTTP y gRPC cada uno `NewTraceExporter(ctx,Config)` y
`NewMetricExporter(ctx,Config)` retornan interfaces SDK nativas que se pasan al
provider. Config Endpoint R (URL http(s) HTTP; authority host:port gRPC),
Headers map O<=64, Timeout D:5s>0, TLS{Insecure D:false,CAFile O},
Compression none/gzip D:gzip; MaxQueueSize D:2048>0, BatchSize D:512>0 <=queue,
FlushInterval D:5s>0 se aplican en processor config de ensamblado y no se duplican
en exporter: **no campos queue/batch en Config del exporter**. Provider config
incluye TraceQueueSize/TraceBatchSize/TraceFlushInterval con esos defaults cuando
se solicita processor administrado, XOR TraceProcessor prestado en deps.

Prometheus `New(Config)(*Exporter,error)`, Handler() http.Handler,
Reader() sdkmetric.Reader, Shutdown(ctx) error. Config Namespace O,
MaxConcurrentScrapes D:4>0, ScrapeTimeout D:10s>0, MaxMetricStreams R>0;
Registry local, sin default global de Prometheus, sin listen. Cardinalidad real
se limita en OTel Views/reader; MaxMetricStreams configura ese límite y se
rechaza si combinación de exporter no puede cumplirlo, no un campo decorativo.

Otellog `NewHandler(Config,Dependencies)(slog.Handler,error)` y
Shutdown(ctx) error; Dependencies LogExporter nativo R, fallback slog.Handler O
prestado. Config QueueSize D:2048>0, BatchSize D:512<=queue,
FlushInterval D:5s>0, MaxRecordBytes D:16KiB>0, Overflow drop_newest/error
D:drop_newest para **diagnóstico** con contador de drops. Slog Handler.Handle
retorna error si Overflow=error; caller decide reporte, no recursión de logging.
No usar logs exporter como audit Writer ni afirmar entrega durable. Señales
propagan trace/span IDs con contexto y redactan antes de exportar.

## Ensamblado, providers, presets y pruebas

<!-- contract: engine -->
<!-- contract: engine/providers/* -->
<!-- contract: presets/* -->
<!-- contract: testkit/* -->
### Ensamblado y factories

`engine.New(Config,[]Provider)(*Engine,error)`, `Start(ctx) error`,
`Shutdown(ctx) error`, `Component(name)(any,bool)` y helper genérico libre
`Get[T](engine,name)(T,error)`. Config Name R, StartupTimeout D:30s>0,
ShutdownTimeout D:30s>0; no Config de router/logger/health/SDK. Provider es
`Name() string; Requires() []string; Start(ctx,Resources)(any,error);
Stop(ctx) error`. Resources `Lookup(name)(any,bool)` solo para deps declaradas,
no service locator global. Config decodificada/capturada por el provider factory,
no un RawConfig de todas las herramientas dentro de Deps. Dependencias explícitas
permiten DAG topológico; ciclos/double-name/missing antes de I/O. Stop de solo
started en reversa, no doble cierre de shared pool; errores Join preservados.

Cada `engine/providers/<herramienta>` exporta
`New(name string,cfg ToolConfig,deps ToolDependencies)(engine.Provider,error)`;
Config no introduce campos adicionales al tool, excepto nombre/referencias en
configuración del **ensamblador**. Todas las referencias se resuelven por DAG
antes de Start; los providers de drivers/fuentes/telemetría son módulos opt-in.
El patrón cubre cada provider del inventario, no un módulo universal con imports
de todos. Presets reciben una lista explícita de providers/options y su Config
es solo la composición de esas entradas; `full` es agregado opt-in, no registra
stores nuevos por encontrar su YAML. Falta bloque + provider requerido => error,
no construir silenciosamente un Config cero de un cliente real.

Testkit por adapter exporta builders de fixtures/mocks y no Config runtime.
Sin registrar globals ni abrir red al importar. Interfaces nativas de SDK en su
propio testkit; mock genérico común solo stdlib. Examples no son API pública ni
módulos requeridos. Benchmarks y pprof se tratan como código de test y handler
operativo respectivamente, nunca como dependencia obligatoria del motor.

## Reglas de compatibilidad por familia

- Existing APIs que exponen SDK conservan sus capacidades en native adapters;
  neutral interfaces no adquieren esos tipos para evitar ruptura accidental.
- Alias solo si identidad y semántica coinciden; métodos de logging toggles,
  globals, executor desacoplado y defaults cero→tres que cambien garantías
  requieren ruptura explícita, sin puente. task_executor queda congelado y
  obsoleto hasta retirarse en la release de migración.
- Toda firma en tablas se convierte a una declaración compilable con doc comment
  y consumer fixture al implementar el módulo; no inventar overloads. Referencias
  a firmas históricas se resuelven contra el inventario y Service de origen.
- Config structs completos se fijan en estos contratos: agregar campo SDK no
  enumerado requiere ampliar diseño y pruebas, no permitir map de opciones opaco.
- Defaults de seguridad cambiados respecto del código actual (TLS, explicit pool
  bounds, timeouts y unknown keys) se publican como migración incompatible y se
  prueban en ambos modos; no afirmar compatibilidad binaria/funcional automática.

## Tipos de protocolo y operaciones administrativas

Las siguientes formas completan los DTO mencionados por las familias; structs
son copias de datos, no referencias mutables a maps internos.

| Tipo | Campos públicos/contrato |
|---|---|
| ConfigError | Field, Reason string; Unwrap/Is local ErrInvalidConfig, sin valor de usuario |
| Capabilities SQL | Dialect string, ServerVersion string, Savepoints, SkipLocked bool; obtenidos del backend sin asumir equivalencia |
| Lease | Token string, Version uint64, ExpiresAt time.Time de DB; opaque ownership, no generarlo fuera del adapter |
| Saga Instance | ID, DefinitionVersion string; Input []byte; estado inicial pending; schema version de formato persistido separada |
| Saga Snapshot | Instance Instance, State string, Version uint64, Steps []StepSnapshot, NextAt time.Time, LastErrorCode string |
| StepSnapshot | StepID, State string, Attempts/CompensationAttempts int, Output []byte; contenido acotado por MaxPayloadBytes |
| Saga Transition | ExpectedState, NewState, StepID string; Output []byte; NextAt time.Time; ErrorCode string; validación de state machine antes del CAS |
| Audit Filter | Tenant R, Actor/Action/Resource/Outcome O; From/To UTC opcionales; PageSize requerido; no SQL libre |
| Audit Page | Records []Record, NextCursor string; snapshot de consulta paginada, no todo el historial |
| HTTP Problem | Status int en [400,599], Code/Title string públicos, Details map[string]string redactados; causa privada fuera del payload |
| JWT Claims | Subject/Issuer string, Audience/Groups []string, Expiry time.Time, Custom map[string]any acotado; copia inmutable |

Cada adapter SQL durable ofrece explícitamente `Migrations() []Migration`, con
Migration{Version uint64,Name,UpSQL string}; no Down automático destructivo en
v1. `Migrate(ctx,db,versions)` es comando del adapter/CLI, no de las tools ni del
startup. Mantiene registro de versión/checksum y lock exclusivo por dialecto;
rechaza checksum cambiado en migración aplicada. Soporte de rollback DDL depende
del backend; no afirmar atomicidad de DDL idéntica en PostgreSQL y MySQL.
No renovar schema mientras hay workers sin estrategia de compatibilidad expand/
contract. Table names validados contra esquema permitido y nunca interpolar
entrada de requests.

Admin cleanup por adapter:
`Cleanup(ctx,cfg CleanupConfig) (deleted int64,err error)` con BatchSize R>0,
Before UTC R, MaxBatches R>0; no goroutine/ticker propio. Solo estados terminales
permitidos, leases/uncertain excluidos. Before no puede violar Retention del store;
error si intenta hacerlo. Permisos/ownership administrativos separados de Writer.

Ergonomía de constructores no introduce overloads: tipo nativo prestado se pasa
por Dependencies y es mutuamente excluyente con credenciales/connector de apertura.
Cada adapter real deberá generar su declaración Service exacta desde el método
set de origen enumerado en inventory; esta revisión fija que capacidades nativas
se conservan, no permite eliminarlas al moverlas. Funciones sin Config (helpers,
DTO, normalización pura con límites fijados por constructor) no leen globals.


## Requisitos de implementación y evidencia posteriores

Disponible significa merge en main con consumidor externo pasando; las
dependencias se realizan antes como tareas separadas. Para etiquetar, cada
dependencia necesita versión resoluble mediante go get. No crear core como
prerrequisito ni reimplementar una dependencia faltante dentro de una herramienta.

Los permisos de once categorías están fijados en decisions.md por concreción
técnica autorizada sobre las respuestas recibidas.
Se confirman los bordes store→capacidad+atomic+driver, exporter→OTel+transporte,
provider→engine+herramienta+config y resilience→herramientas que compone. No se
usa una allowlist universal stdlib+SDK+core.

Stdlib-only incluye tests sin dependencias externas; fugas con goroutineleak
(D28), no goleak. Harness pesado en modulo/integration, go.mod propio, fuera
del grafo mínimo; fakes solo de adaptadores en testkit/adaptador. Dos consumers:
minimal y composed, replaces únicamente al módulo y sus dependencias declaradas.
No manifest de versiones => no tag independiente. Responsable propone versión
y usuario aprueba. Las siete evidencias E1–E7 están fijadas en decisions.md, incluyendo tag real
y gorelease; no queda una dependencia documental de answers.md.

# API pública objetivo de concurrency y condición de estabilidad

Estado: especificación normativa de la API v1, **todavía no implementada**.
Sustituye alternativas abiertas del [contrato de concurrencia](concurrency.md).
La API experimental puede cambiar para cumplirla; no se declara estable por
haber pasado los tests actuales. No se requiere completar sagas/SQL/telemetría
para estabilizar este módulo independiente.

## 1. Alcance decidido

Dos superficies en un módulo stdlib-only: primitivas de streaming y pool
administrado. Sin retry interno, DB, broker, logger, reloj público sustituible,
callbacks de usuario en el lock, rate limiter ni estado durable. La aplicación
compone retry alrededor de la función; cada worker permanece ocupado durante
esa función, incluido su backoff. Un limiter compartido limita I/O por intento.
Rate limiting por tiempo tendrá módulo separado `ratelimit`, no será llamado
Limiter de concurrencia.

La v1 incluye FIFO de admisión, unordered o serialización por clave, Future por
job, límite de jobs/weights, batches y cancel/drain. **No incluye** prioridades,
resizing/recarga, ejecución multi-key, broadcast, retry diferido, historial de
resultados, orden global entre procesos ni salida ordenada de Map. Son extensiones
separadas, no huecos que impidan terminar v1. Si se añaden posteriormente deben
preservar las garantías de v1. Map ordenado necesita otra API con ventana acotada.

## 2. Pool, tipos y configuración exacta

Firmas objetivo; imports omitidos en este fragmento solo por presentación.
Enums son strings para decodificación, con constantes para cada valor indicado.

```go
// package concurrency
// imports: context, time

type Admission string        // "block", "reject"
type Ordering string         // "unordered", "per_key"
type KeyFailurePolicy string // "continue", "cancel_pending"
type ShutdownMode string     // "drain", "cancel"
type State string            // "succeeded", "failed", "canceled"

type PoolConfig struct {
    Workers          int
    QueueCapacity    int
    Admission        Admission
    AdmissionTimeout time.Duration
    TaskTimeout      time.Duration
    Ordering         Ordering
    MaxActiveKeys    int
    PerKeyCapacity   int
    KeyFailure       KeyFailurePolicy
    MaxJobWeight     int64
    MaxInFlightWeight int64
}

func DefaultPoolConfig() PoolConfig
func (c PoolConfig) Validate() error

type Job[I any] struct {
    ID     string
    Key    string
    Weight int64
    Value  I
}

type Result[O any] struct {
    ID         string
    Key        string
    Sequence   uint64
    Value      O
    Err        error
    State      State
    AcceptedAt time.Time
    StartedAt  time.Time
    FinishedAt time.Time
}

type Task[I, O any] func(context.Context, I) (O, error)
type Pool[I, O any] struct { /* campos privados */ }
type Future[O any] struct { /* campos privados */ }

type Stats struct {
    Accepted, Succeeded, Failed, Canceled, Rejected uint64
    Queued, Running, ActiveKeys int
    InFlightWeight int64
    Closed bool
}

func NewPool[I, O any](cfg PoolConfig, task Task[I, O]) (*Pool[I, O], error)
func (p *Pool[I, O]) Submit(ctx context.Context, job Job[I]) (*Future[O], error)
func (p *Pool[I, O]) Shutdown(ctx context.Context, mode ShutdownMode) error
func (p *Pool[I, O]) Done() <-chan struct{}
func (p *Pool[I, O]) Stats() Stats
func (f *Future[O]) Done() <-chan struct{}
func (f *Future[O]) Wait(ctx context.Context) (Result[O], error)
func (f *Future[O]) Cancel(cause error)
```

| Campo | Regla v1 | DefaultPoolConfig |
|---|---|---|
| workers | Obligatorio > 0, no se redimensiona | 1 |
| queue_capacity | >= 0; 0 = handoff solo con worker libre, sin backlog | 0 |
| admission | `block` o `reject`; vacío inválido | block |
| admission_timeout | >= 0; 0 = solo deadline/cancel del caller | 0 |
| task_timeout | >= 0; empieza al iniciar ejecución; 0 = contexto caller | 0 |
| ordering | `unordered` o `per_key`; vacío inválido | unordered |
| max_active_keys | > 0 con per_key; 0 obligatorio con unordered | 0 |
| per_key_capacity | > 0 con per_key; cuenta pendientes; 0 con unordered | 0 |
| key_failure | Requerido explícito en per_key; vacío obligatorio en unordered | vacío |
| max_job_weight | > 0, unidad lógica del llamador; ninguna medición del heap | 1 |
| max_in_flight_weight | >= max_job_weight; suma queued + running | 1 |

DefaultPoolConfig permite un único job de peso 1 en vuelo. Para mayor
concurrencia, el caller aumenta también el presupuesto de weight. El decoder
siembra ese default antes de aplicar campos presentes; un literal PoolConfig
incompleto se rechaza, no se rellena silenciosamente. Job.Weight siempre > 0;
si solo se quiere contar jobs se usa 1. Para bytes el caller debe calcularlos y
mantener esa estimación válida mientras el pool es dueño del input. La v1 no
promete limitar memoria de outputs o de funciones de usuario.

Los nombres YAML son los campos anteriores en snake_case (`key_failure`,
`max_in_flight_weight`). No hay `result_capacity` en PoolConfig: Future evita
que una cola global de resultados bloquee a los workers. Shutdown mode y su
plazo son parámetros de la aplicación, no estado mutable escondido en el pool.
Job/Task/keys/extractores son valores Go, no código ejecutable en YAML.

## 3. Admisión y propiedad

- NewPool valida, copia config e inicia exactamente Workers goroutines y un
  coordinador; cancelación de queued se observa en el coordinador mediante
  espera multiplexada sobre canales/contextos acotados por jobs vivos, sin watcher
  goroutine por job. Los contextos del caller pueden crear sus propios timers,
  fuera del scheduler. task nil es ErrInvalidConfig. No necesita un método Start que
  permitiría Submit antes de arrancar. Esta excepción explícita a constructores
  puros se justifica por su única función de administrar workers.
- Submit es concurrent-safe. Devuelve Future **solo si** se aceptó el job; de
  otro modo nil y error. El punto de aceptación reserva atómicamente ID activo,
  espacio, weight y clave, y asigna Sequence monotónica local. No se reserva
  parcialmente una key mientras se espera otro recurso.
- ID obligatorio UTF-8, máximo 128 bytes; Key máximo 256 bytes y obligatoria en
  per_key, vacía en unordered. Un mismo ID vivo devuelve ErrDuplicateID de
  inmediato. Al completar se elimina del índice: no hay dedup tras finalizar.
  Future conserva ID/Sequence; usar claves durables en otro módulo cuando aplique.
- block espera capacidad hasta el menor deadline de ctx/admission_timeout;
  reject devuelve ErrSaturated inmediatamente. Job.Weight mayor que el presupuesto
  individual devuelve ErrJobTooLarge, sin esperar. En reject todos los límites
  de capacidad producen ErrSaturated; en block todos aplican backpressure.
- La capacidad de espera del scheduler es finita. Submit **no** crea goroutines
  por llamada ni mantiene una lista interna ilimitada de callers bloqueados.
  El caller conserva el job mientras espera; el pool no garantiza limitar el
  número de goroutines que la aplicación decida lanzar para invocarlo.
- El contexto de Submit rige admisión **y vida del job aceptado**. Cancelación
  posterior cancela queued/ejecución cooperativamente. admission_timeout solo
  rige la espera previa; su timer nunca cancela una tarea ya aceptada.
- Si aceptación y cancelación coinciden, el resultado de Submit determina si
  existe responsabilidad: nil error + Future = aceptado con estado terminal,
  incluso si se cancela de inmediato; error + nil Future = no ejecutado.
  Un job aceptado cuyo contexto ya está cancelado al despachar no se ejecuta.
- No se copia Value. Desde aceptación hasta Future.Done el caller no lo muta.
  El pool elimina referencias al input al terminar. Los resultados pertenecen
  al Future del caller; retener millones de Futures está fuera del presupuesto
  del pool. No hay ledger histórico ni GC/TTL implícito de resultados.

## 4. Resultados, errores y cancelación

Future.Done cierra exactamente una vez. Wait admite múltiples lectores y es
repetible: devuelve copia inmutable del mismo Result (datos referenciados deben
tratarse como inmutables). El error externo de Wait solo significa que **la
espera** fue cancelada; no cancela la tarea. Si retorna nil error, el outcome de
la tarea está en Result.Err/State. Cancel(nil) usa context.Canceled; Cancel con
causa conserva esa causa; no altera un outcome terminal ya confirmado.

La finalización es una transición atómica. Si cancellation gana antes de la
finalización, un retorno nil del task produce resultado canceled, no éxito.
Si la función retorna error propio después de esa señal, Result.Err conserva
ambas causas con errors.Join; State=canceled. Un error de función sin señal
produce failed. Un panic síncrono produce failed y PanicError aunque hubiera
cancelación concurrente; la causa de contexto puede adjuntarse. Si éxito ya fue
confirmado, Cancel posterior no lo convierte en fallo. Estas reglas se prueban
con barreras, sin depender de sleeps.

La librería no infiere si un error de negocio es transient/permanent. Es
responsabilidad de classifier/retry inyectado. Una ejecución lógica puede tener
varios intentos; el Future expone su outcome lógico, no inventa attempt IDs.
Instrumentation por intento vive en el decorator, preservando el ID del job.

Errores públicos del pool: ErrInvalidConfig, ErrClosed, ErrSaturated,
ErrAdmissionTimeout, ErrDuplicateID, ErrJobTooLarge, ErrKeyAborted y ErrPanic.
ConfigError{Field,Reason} envuelve ErrInvalidConfig sin valores sensibles;
PanicError{Value any,Stack []byte} tiene Is(ErrPanic), stack acotado a 64 KiB y
no se loguea automáticamente. Deadline de caller preserva context.DeadlineExceeded;
timeout propio de admisión es ErrAdmissionTimeout. TaskTimeout conserva
context.DeadlineExceeded. Todas las causas siguen alcanzables por errors.Is/As.

Cancelar queued libera reserva y finaliza su Future; cancelar running conserva
worker/weight/key hasta que la función retorne. StartedAt es cero para canceladas antes de ejecutar. Sin goroutine por task que se
abandone al timeout. Panics en goroutines creadas por el usuario, runtime fatal
errors o una función que llama runtime.Goexit quedan fuera del contrato; el task
no debe llamar Goexit. Código no cooperativo requiere aislamiento por proceso.

## 5. Scheduler por clave

FIFO de **aceptación** por key; no de llegada a goroutines Submit concurrentes.
Una única función lógica activa por key, incluyendo todos los retries del
wrapper. Distintas keys progresan mediante round-robin de keys listas, tomando
un job por turno. Un job cuyo weight está reservado no compite de nuevo por
memoria al despachar. MaxActiveKeys cuenta keys con queued o running; se borran
inactivas sin cache. PerKeyCapacity y QueueCapacity cuentan solo pendientes.

KeyFailure es explícito: continue permite siguiente job tras un fallo;
cancel_pending finaliza con ErrKeyAborted los que estaban pendientes en esa key
al confirmar el fallo. No deja una key pausada para siempre ni introduce Resume.
La key se admite otra vez tras esa transición; no es un bloqueo de cuenta de
negocio. Cancelación explícita de un job no dispara key_failure; panic y failed
sí. Ni unordered ni per_key ofrecen FIFO entre producers que aún no fueron
aceptados; la política de block no promete ausencia de starvation del caller
externo, mientras el round-robin sí evita que una key admitida acapare workers.

No usar hash shards como sustituto silencioso de semántica por key. No hay
locks multi-key ni sincronización entre réplicas. La corrección financiera sigue
en la transacción y las restricciones de negocio.

## 6. Shutdown sin pérdida de resultados ni bloqueo de outputs

Estados: accepting → draining/canceling → closed. Shutdown cierra admisión
atómicamente, despierta Submit bloqueados con ErrClosed y es concurrent-safe.
Drain ejecuta los jobs aceptados respetando su contexto original; el contexto
de Shutdown solo limita cuánto espera esa llamada, no sustituye los contextos
de jobs ni los extiende. Cancel cancela queued y solicita cancelación a running.
Una llamada Cancel puede escalar un drain; un drain nunca revierte canceling.

Si ctx de Shutdown vence devuelve ctx.Err con stats consultables. El pool
permanece sin admisión y continúa el modo solicitado; se puede llamar otra vez
para esperar o escalar a cancel. Done solo cierra cuando terminan todos los
workers y todos los Futures aceptados son terminales. No hay falso Done cuando
una función no cooperativa sigue viva. No existe Results global cuyo abandono
bloquee shutdown. Después de cerrado, Shutdown retorna nil salvo ctx inválido.
Stats es snapshot coherente bajo lock, no vista mutable de maps internos.

## 7. Streaming y limiter: superficie final

Se sustituye la Config ambigua del prototipo por tipos específicos. Las APIs
experimentales no están publicadas, por lo que se admite esta ruptura antes de v1.

```go
// package concurrency; imports: context, time

type StreamConfig struct { Workers, Buffer int; TaskTimeout time.Duration }
type StreamResult[T any] struct { Value T; Err error }
type BatchConfig struct {
    Size int
    MaxWait time.Duration
    Buffer int
    MaxBatchWeight int64
}
type LimiterConfig struct { Capacity int64 }
type Limiter struct { /* privado */ }

func Map[I,O any](ctx context.Context, in <-chan I, cfg StreamConfig,
    task Task[I,O]) (<-chan StreamResult[O], error)
func From[T any](ctx context.Context, values ...T) (<-chan T, error)
func FanIn[T any](ctx context.Context, buffer int, in ...<-chan T) (<-chan T, error)
func FanOut[T any](ctx context.Context, in <-chan T, count, buffer int) ([]<-chan T, error)
func Batch[T any](ctx context.Context, in <-chan T, cfg BatchConfig,
    weight func(T) int64) (<-chan StreamResult[[]T], error)
func NewLimiter(cfg LimiterConfig) (*Limiter, error)
func (l *Limiter) Acquire(ctx context.Context, weight int64) (release func(), err error)
```

Contexto nil devuelve ErrInvalidConfig en **todas** las APIs, incluido From;
inputs nil inválidos excepto miembros nil de FanIn, que se ignoran. Valores
negativos inválidos; Workers/count > 0; Buffer=0 unbuffered. Slice From no se
copia y no se modifica hasta cierre. Fuentes infinitas las proporciona el caller
con contexto y canal; no API generadora adicional en v1.

Map completion-order, sin tracking; worker fijo y resultados Buffer acotado.
FanOut distribuye, no broadcast; FanIn un forwarder por input no-nil (el caller
controla cuántos streams construye). Cancel puede descartar elementos/resultados;
el consumidor que abandona cancela el contexto compartido. Streaming no garantiza
un resultado por cada input al cancelar. Nunca se usa como aceptación durable.

Batch: Size > 0, MaxWait >= 0 (0 desactiva timer), MaxBatchWeight > 0, weight
no-nil y pesos positivos. Flush al alcanzar size o antes de exceder weight,
además de tiempo; cierre normal emite remanente. Item individual excedido o peso
inválido emite un StreamResult con ErrJobTooLarge/ErrInvalidConfig y termina esa
etapa; no continúa descartando inputs silenciosamente. El consumidor cancela
pipeline al recibir error terminal. Cancel descarta batch parcial. El tiempo
acota cuándo se ofrece, no entrega bajo backpressure; slices no comparten backing
storage. La función weight debe ser pura/no-panic; no intenta medir el heap real.

Limiter acepta 0 < weight <= Capacity, release idempotente y cancelación de espera.
Reserva ponderada atómica y cola FIFO de waiters; un waiter grande puede bloquear
los pequeños hasta salir o cancelarse (head-of-line documentado). Sin goroutine
por waiter; el caller es dueño de cuántas llamadas concurrentes lanza. Un nil
receiver/zero-value no inicializado produce ErrInvalidConfig. Release no devuelve
error y sigue válido después de cancelar ctx. No es requests/segundo.

## 8. Plan concreto de finalización, sin implementar en esta revisión

| Paso | Cambio sobre prototipo | Evidencia para cerrar |
|---|---|---|
| C1 | Congelar tipos/config/errores aquí definidos y ejemplos de consumidor | Fixtures de API y configs inválidos/válidos; ninguna alternativa sin decidir |
| C2 | Scheduler con reserva atómica count/weight/key e IDs vivos | Invariantes bajo Submit concurrente y race |
| C3 | Future local sin ledger de resultados y cancelación por job | Un terminal por aceptado; Wait repetible; abandono no bloquea pool |
| C4 | Key scheduling y key_failure | FIFO por key, round-robin, hot key, eliminación y política de fallo |
| C5 | Drain/cancel/escalación y contexto | Shutdown concurrente; job no cooperativo mantiene slot y Done abierto |
| C6 | Adaptar streaming, errores uniformes y limiter ponderado | Conservación normal; cancelación; nil/weights/timers/límites |
| C7 | Documentar replay y wrappers externos sin imports | Composición retry+limiter stdlib; consumers sin root/SDK |
| C8 | Release candidate y publicación v1 independiente del motor | Gates siguientes; tag concurrency/v1.0.0 solo al alcanzarlos |

Para v1 real se separa versionado de concurrency del release coordinado del
motor 0.x: el workflow actual que copia una versión a todos los tags debe
adaptarse a un manifiesto de versiones por módulo. No cambiar todos los módulos
a v1 para estabilizar uno ni llamar "estable" una v0.x. El gate actual standalone
solo cubre Map experimental; debe cubrir Pool/Future/keyed/shutdown/Batch/Limiter
por las APIs nuevas, y un consumidor sin workspace ni replace a la raíz.

## 9. Gates objetivos de v1

- Race, go vet/lint y tests repetidos con GOMAXPROCS 1 y >1, deadlines de suite;
  controlar interleavings con canales/barreras, no sleeps de precisión.
- Modelo de estados e invariantes: running<=workers, queued<=queue_capacity,
  weight<=budget, pending/key y active_keys acotados; Accepted=Succeeded+Failed+
  Canceled+Queued+Running en snapshots coherentes.
- Cancel/Submit/Shutdown concurrentes, ID duplicado vivo, saturación count/weight/
  key, Future abandonado, cancelación de Wait sin cancelar task, errores y panics.
- Shutdown de tarea no cooperativa probado con gate liberable, nunca una goroutine
  inmortal de test. Fugas verificadas tras cerrar todos los recursos. No añadir
  dependencias runtime para probar leaks; herramientas de test en harness separado
  si el contrato del go.mod exige cero dependencias incluso de test.
- Serialización y retries por key, equidad entre keys admitidas, key_failure,
  cancelación pendiente/activa, snapshots y peso liberado exactamente una vez.
- Property/fuzz para secuencias de estado y configs (sin usar aleatoriedad como
  único test); ejemplos ejecutables que compilen desde un módulo externo.
- Benchmarks sin race para throughput/allocs; race se prueba aparte. Medir cargas
  vacías, I/O cooperativo, hot keys, saturación, outputs lentos y memoria retenida.
  Publicar plataforma/carga; no prometer un SLA hardware-independiente inventado.
- Documentación de todos los exports, configuración/zero values, limitaciones,
  migración task_executor → nueva API y revisión de firmas Go (sin métodos con
  parámetros de tipo adicionales, ni genéricos incompatibles con Go declarado).

**Conclusión de revisión:** es viable completar concurrency como módulo v1
independiente. El prototipo sirve de base para streaming, pero no basta agregar
unas opciones: hacen falta scheduler/Future/lifecycle y validar sus invariantes.
No se declara listo hasta que C1–C8 y los gates tengan evidencia real.

Referencias: [context](https://pkg.go.dev/context),
[pipelines](https://go.dev/blog/pipelines) y
[versionado de módulos](https://go.dev/doc/modules/release-workflow).
Los límites adicionales de este documento son contratos elegidos por el proyecto.

## 10. Migración explícita desde las APIs anteriores

| API anterior | Destino decidido | Diferencia que el consumidor debe aceptar |
|---|---|---|
| task_executor.WorkerPool(map[string]Tasker) | Submit Job por entrada y conservar Futures | ID propio, aceptación explícita; el caller decide si colecciona un map y asume su memoria |
| task_executor.Task[I,O] / Tasker(any) | Task[I,O] + Job[I] | Tipado sin any; no se importa logger de raíz |
| SetPriority/WithPrioritySupport | Bridge heredado, fuera de v1 del pool | No fingir prioridad con orden FIFO; el consumidor que depende de ella no migra aún |
| WithTaskTimeout | PoolConfig.TaskTimeout | Cooperativo, slot retenido hasta retornar; no retorno temprano con ejecución desacoplada |
| WithResultTimeout | Future.Wait(ctx) o Shutdown(ctx,mode) | Timeout de espera no cancela job; Cancel es explícito |
| BatchWorkPool | Batch streaming + Submit o Map | No materializa todo el trabajo; se decide ownership, IDs y final partial batch |
| WithLogger/WithMetricsCollector/WithResultCallback | Wrapper alrededor de Task/Future + Stats pull | Sin callback scheduler, sin bloqueo bajo locks ni SDK obligatorio |
| Prototipo Config/Result | StreamConfig/StreamResult en Map; PoolConfig/Result en Pool | Renombre incompatible permitido por estado experimental, antes de publicar |
| Prototipo From sin error | From con error de argumento | Context nil y fallo de construcción son uniformes |
| Prototipo Batch(size,maxWait,buffer) | BatchConfig + weight y StreamResult de batch | Tamaño por peso y error de item excedido dejan de ser silenciosos |
| Prototipo NewLimiter(int)/Acquire(ctx) | LimiterConfig + Acquire(ctx,weight) | Reserva ponderada/FIFO; release sigue idempotente |

No usar aliases para ocultar diferencias de semántica. La API heredada sigue
siendo explícitamente heredada hasta una release de migración; no se agrega
una dependencia inversa tool → root para conservarla.

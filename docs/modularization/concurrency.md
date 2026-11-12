# Contrato de concurrencia para producción

Estado: fundamentos del diseño. La especificación exacta y las decisiones v1
están en [concurrency-api](concurrency-api.md) y prevalecen ante ejemplos o
alternativas generales de este documento. El prototipo aún no las implementa.
No confundir un límite de goroutines con una garantía de operaciones durables.

## Responsabilidad y límites

`concurrency` planifica trabajo local acotado. `retry` decide qué repetir;
`idempotency` y la base de datos deciden si una operación ya se aplicó; brokers y
outbox conservan trabajo durable. El motor solo los ensambla. No hay driver,
SDK de métricas, logger ni clasificación financiera dentro del scheduler.

| Aspecto | Contrato a diseñar e implementar | Dueño de la política |
|---|---|---|
| Cancelación | Contexto propagado en admisión, cola, ejecución, espera de retry, publicación de resultados y shutdown | Scheduler + operación cooperativa |
| Backpressure | Capacidad finita, `block` con deadline o `reject` inmediato; errores explícitos de saturación | Config del pool |
| Errores | Resultado por tarea, clasificación explícita, causa preservada, reporte por hooks | Operación/adaptador + retry opcional |
| Orden | ID estable, secuencia y opción de serialización por clave | Aplicación define ID/clave; scheduler impone orden local |
| Recursos | Workers, cola, outputs, bytes, keys, conexiones y llamadas externas acotados | Cada recurso limita lo que posee; ensamblado valida presupuesto conjunto |
| Consistencia | Restricciones SQL, idempotencia y transacción compartida; recuperación durable | Stores + aplicación, no el pool |

## Admisión, colas y memoria

La nueva configuración diferencia `queue_capacity` de `result_capacity`; el
Buffer del prototipo solo limita resultados. Toda admisión devuelve aceptada o
error y conserva el ID para rastrear el destino. `block` respeta el menor de
contexto y admission_timeout; `reject` no encola y devuelve saturación. Un caller
puede reintentar admisión con presupuesto explícito, sin crear otra cola ilimitada.

No se habilita descarte silencioso, sobrescritura o drop-oldest para trabajo
durable/financiero. Una capacidad de muestreo best-effort sería otro contrato
opt-in, nunca un modo oculto del mismo pool. No persistir implica que un proceso
puede perder tareas aceptadas; si la admisión necesita sobrevivir al proceso,
se confirma después de persistir en broker/outbox, no después de enviar a un chan.

Cada cola tiene dueño, capacidad, política de overflow y canal de reporte. Se
acotan tareas pendientes globales y por clave, claves activas, resultados,
retries diferidos, tamaño de lotes y buffers de cada stage. Un retry scheduler
no puede crear un timer/goroutine por cada tarea sin un límite adicional.

Limitar cantidad de items no limita bytes: para payloads variables se usa peso
calculado por el llamador, con máximo por item y presupuesto total. Definir la
transferencia de ownership del payload y no duplicarlo por stage sin contabilizar.
Los slices/lotes y resultados siguen consumiendo memoria hasta que su dueño
los libera. El scheduler no promete limitar el heap de código de usuario.

Presupuesto conceptual por pool: bytes en cola + bytes de workers activos +
bytes en outputs + lotes en formación + backlog de retries + estructuras de
claves. El ensamblador suma además transport buffers, pools SQL y export queues.
Reservar todos los recursos en un orden definido para evitar deadlock por
adquisiciones cruzadas; no ejecutar operaciones de usuario mientras se posee
el lock interno del scheduler.

## Identidad, resultados y clasificación

El job envelope opcional conserva ID, ordering key, intento, fecha de admisión y
metadata de correlación acotada; el valor permanece tipado. Reintentar conserva
el ID de operación y agrega un attempt ID. No meter credenciales o estado mutable
en context; usarlo para cancelación y metadata de petición según convención.

Toda tarea aceptada tiene un estado terminal observable en el API de pool con
tracking: completada, fallida o cancelada. Esto requiere un ledger local acotado
con consumo/retención definidos o un sink durable explícito; no guardar para
siempre un map de resultados. Las primitivas de streaming de bajo nivel pueden
abandonar resultados al cancelar, pero deben documentarlo y no ofrecerse como
tracking durable. La v1 del pool usa Stats pull/Future y no callbacks; observación externa tiene
su propio buffer y política.

Categorías de diseño a nivel de composición: cancelado/deadline, saturación,
validación, conflicto, transitorio, permanente, pánico y resultado desconocido.
El Pool solo clasifica estado local (succeeded/failed/canceled) y sus errores;
no infiere categorías de negocio ni de replay. Se preservan
`errors.Is/As` y causa. Cada adaptador reconoce sus códigos específicos; las
reglas viven junto al recurso, no en una tabla que importe todos los SDK.

Por defecto no hay retries. Se activa un decorator que acepta classifier,
presupuesto, exponential backoff y jitter. Cancelación, validación y pánicos no
se repiten automáticamente. Un error transitorio no basta para repetir un
write: se exige que sea seguro o esté protegido por idempotencia. Un resultado
ambiguo pasa a conciliación, no a una repetición ciega.

Los hooks reportan intento, outcome, latencia y espera sin bloquear indefinidamente
el scheduler. IDs van a logs/traces, no a labels métricas de cardinalidad ilimitada.
Una falla del observer se distingue de la falla del negocio.

## Orden y serialización por clave

Modos separados: unordered/completion-order; ordered-results para presentación;
y per-key para orden de ejecución. Reordenar resultados no impide que dos writes
se ejecuten simultáneamente. Ordered-results necesita ventana acotada y aplica
backpressure ante un elemento lento; nunca acumula resultados ilimitados.

En per-key solo una tarea por clave puede ejecutar; las distintas claves pueden
avanzar en paralelo hasta el límite global. El orden es el de admisión aceptada,
no el de creación de goroutines concurrentes. La aplicación define una clave
estable de negocio, por ejemplo account ID. Retry de una tarea mantiene su lugar
si posteriores operaciones no pueden adelantarla. Una tarea fallida permanente
necesita una política explícita de continuar o bloquear esa clave; ninguna se
oculta detrás de un default de reintentos.

Acotar max_active_keys, per_key_capacity y queue_capacity; remover keys inactivas;
definir equidad para evitar que una hot key bloquee a todas. No evadir el orden
al reencolar desde un decorator. Las tareas multi-key no se resuelven con dos
colas locales independientes: necesitan orden de adquisición y validación en DB.

El orden por clave del pool es **por proceso**. Entre réplicas se necesita
particionamiento por clave, exclusión en DB/optimistic version o coordinación
durable con fencing según el caso. Una key hash no es un lock distribuido.

## Cancelación y shutdown

Cancelar una tarea en cola impide iniciarla. Cancelar una activa entrega la señal
al contexto; debe conservar su slot hasta terminar. No lanzar tareas de reemplazo
que oculten que el código anterior sigue ejecutándose. La cancelación no revierte
un commit ni prueba que un efecto remoto no ocurrió. Un task timeout puede estar
anidado dentro del deadline global, nunca extenderlo.

Shutdown cierra admisión primero. Modos explícitos: drain de aceptadas con un
contexto de shutdown separado y acotado, o cancel-pending/cancel-active. En drain,
no cancelar inmediatamente el contexto que necesitan las tareas aceptadas.
Al vencer el plazo, reportar pendientes y activas; no afirmar que una goroutine
no cooperativa fue detenida. Los trabajos durables se recuperan desde el store.
Los outputs solo se cierran por su productor; no cerrar canales de un tercero.

## Conexiones, límites externos y finanzas

Workers y conexiones SQL son límites distintos. Reusar un pool por recurso,
respetar MaxOpen, tiempo de adquisición y reservar capacidad para salud/control
cuando el diseño lo requiera. Una operación puede necesitar más de una conexión:
no asumir workers <= MaxOpen demuestra ausencia de bloqueo.

Limitar llamadas externas con semáforo/bulkhead compartido por dependencia y
rate limiter opcional separado de la concurrencia. No reintentar en scheduler,
client y SDK simultáneamente sin un presupuesto conjunto; los intentos se
multiplican. Describir dónde queda el breaker y el límite durante un backoff.

Para operaciones financieras, el servicio de dominio conserva invariantes como
versiones, estados, límites y atomicidad; usa representación monetaria exacta y
restricciones apropiadas a su negocio. La librería aporta transacciones,
idempotencia, outbox/inbox y auditoría; no implementa un ledger universal ni
promete exactly-once distribuido. Cancelar un HTTP request tras commit no elimina
el registro: un replay con la misma clave recupera el resultado durable.

## Brechas verificadas del prototipo

| Prototipo actual | Cambio de diseño antes de declararlo producción |
|---|---|
| `Map` recibe un canal del caller | No controla la admisión ni capacidad de ese input; Pool/Submit/Future definidos en la especificación v1 |
| Config incluye Workers/Buffer/TaskTimeout | Faltan capacidades de cola, bytes, keys, admission, shutdown y integración externa del presupuesto de retries |
| Result conserva Value/Err | IDs/clave/attempt requieren envelope tipado opcional; no inferirlos del índice |
| Map emite en completion-order | Faltan orden por clave y ventana acotada de resultados ordenados |
| Función síncrona por worker | Mantener: no desvincular la tarea al vencer su contexto |
| Cancelación puede descartar outputs | Mantener solo en primitivas low-level; tracking local requiere Future; tracking durable requiere otro store |
| Limiter acota simultáneas | Mantener; no presentarlo como rate limiter o límite global entre réplicas |
| Batch limita tamaño por items | Agregar política/peso de bytes cuando haya payloads variables |

## Aceptación futura

Pruebas de límites reales, saturación block/reject, cancelación antes/durante
admisión y ejecución, deadlocks/leaks, IDs tras retry, hot-key fairness,
serialización y orden de retry, límites de bytes/keys/outputs, shutdown acotado,
non-cooperative tasks y pipeline abandonado. Benchmarks deben incluir carga lenta,
I/O simulado, skew de keys, memoria y buffers; una tarea trivial no demuestra
capacidad de producción. Correlacionar después con pruebas PostgreSQL/MySQL y
redelivery del broker, sin sustituirlas por tests del pool.

Base técnica: [pipelines y cancelación en Go](https://go.dev/blog/pipelines) y
[cancelación de consultas SQL](https://go.dev/doc/database/cancel-operations).
Las políticas adicionales de admisión, tracking y orden son decisiones de este
diseño, no garantías heredadas de esas fuentes.

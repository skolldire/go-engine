# Composición, lifecycle y plan de migración

Estado: estrategia previa a implementación. Firmas/configuración exactas en
[public-contracts](public-contracts.md) y [concurrency-api](concurrency-api.md).
Las cadenas y contratos son diseño,
no nombres de funciones que ya se puedan invocar en la nueva API.

## Configuración y dependencias son entradas distintas

Cada herramienta recibe su Config tipado; dependencias como un pool compartido,
un publisher, un logger o un clock se pasan explícitamente. La configuración
refiere instancias por nombre solo en la raíz de composición. El ensamblador
resuelve referencias, valida que no haya ciclos, construye y entrega objetos
concretos; las herramientas no consultan un registro global durante cada llamada.

Puede haber un constructor directo para una sola herramienta y una factory
opcional por tipo para aplicaciones configurables. La factory conoce únicamente
las implementaciones que el programa registró. La aplicación mínima de PostgreSQL
no importa MySQL, y la de configuración sola no importa ninguna base de datos.
Un registry de backends en el ensamblador no forma parte de `database/atomic`.

## Decorator como mecanismo principal

Usar funciones/pequeñas interfaces compatibles con contexto para envolver una
operación o un cliente específico. No forzar todos los transportes a retornar
`any`, ni intentar implementar SQL, broker y HTTP mediante una única interfaz
universal. Adaptadores de conveniencia pueden conservar `Client.Do` de la
fachada existente para quien elija usarla.

| Composición | Responsabilidad | Dependencias que el base no debe adquirir |
|---|---|---|
| Operación + retry | Repetición clasificada y presupuesto | No incorpora SDK de observabilidad |
| Operación + breaker | Admission y estado por dependencia | No incorpora scheduler ni logger global |
| Operación + limiter | Límite compartido de simultáneas | No es rate limiter ni pool SQL |
| Cliente + logging | Evento lógico/correlación/redacción | No exportadores de trazas |
| Cliente + OTel | Spans/métricas y propagación | Solo si el wrapper está importado |
| Handler + JWT/host routing | Controles HTTP y host dispatch | No AWS Cognito obligatorio |
| Store + transacción | Participación en la unidad de trabajo | No crear otra conexión/transacción sin avisar |
| Provider engine + herramienta | Decode/config/lifecycle | La herramienta no importa engine |

Los wrappers especializados que importen un SDK tienen módulo de integración
propio. Un subpaquete dentro del mismo go.mod no ofrece el mismo aislamiento de
requisitos. Un wrapper genérico que solo reciba funciones puede ser stdlib-only.

## Orden de decorators: observable y validado

Para una llamada externa, cadena conceptual de afuera hacia adentro:

```text
contexto/presupuesto total
  → observación de la operación lógica
    → serialización local por clave (si corresponde)
      → retry con clasificación y backoff
        → breaker por dependencia/attempt
          → límite compartido de llamadas activas
            → timeout por intento
              → operación nativa
```

Este orden hace que la serialización por clave dure toda la operación lógica y
sus retries, mientras el slot de llamada externa se libera durante el backoff.
Los workers pueden seguir ocupados esperando retry; un scheduler que libere
workers para deferred retries necesita backlog acotado y conservar el orden.
El timeout por intento aquí empieza después de adquirir el límite; la espera
está acotada por el presupuesto total y/o un acquisition timeout separado.

El breaker exterior observa un intento y puede incluir esperas de admisión del
limiter. El classifier del breaker no cuenta cancelación/admisión local como
fallo de la dependencia. Si se requiere medir exclusivamente el I/O, se cambia
el orden con un contrato probado o una API de permiso del breaker; no acumular
probes half-open esperando otros recursos sin presupuesto.

Circuit-open no dispara retry genérico por defecto; se reporta y se deja a la
política de scheduling durable decidir cuándo volver. Half-open tiene límite
de probes y cada retry debe pedir admisión de nuevo. No compartir una única
instancia breaker entre dependencias distintas sin declararlo.

Logging/tracing pueden tener dos capas explícitas: logical call y cada attempt.
No multiplicar registros por copiar wrappers internos. Dejar retries SDK/client
inactivos cuando retry externo posee el presupuesto, o calcular un máximo
conjunto y exponerlo. Interacciones de tiempo, locks, prioridades y memoria son
parte de la composición, no un detalle que un builder pueda ordenar arbitrariamente.

No aplicar esta cadena ciegamente a escrituras SQL. La unidad de trabajo y la
idempotencia dominan el replay. Tampoco se usa el decorator genérico para simular
una saga o esconder una transacción distribuida.

## Flujo durable de una operación sensible

1. Resolver identidad, alcance y fingerprint. La clave de idempotencia sigue
   estable entre intentos; request ID y attempt ID pueden variar.
2. Abrir una transacción SQL del backend elegido y reclamar/validar la clave
   atómicamente. En duplicado completado, recuperar respuesta; en payload
   diferente, conflicto; en progreso, aplicar política explícita.
3. Ejecutar reglas de dominio y escrituras con el DBTX de **esa transacción**.
   Proteger invariantes mediante constraints, versiones o locks apropiados.
4. Insertar outbox y auditoría obligatoria, y completar el registro idempotente,
   dentro de la misma transacción. No abrir una conexión por capacidad.
5. Commit. En fallo inequívoco, rollback; en resultado ambiguo, resolver por
   clave durable y reconciliación. No afirmar fracaso si el commit pudo ocurrir.
6. Un dispatcher publica outbox después del commit y maneja redelivery. El
   consumidor usa inbox + su propia transacción de negocio antes de confirmar.

Este flujo se refiere a cambios que caben en una transacción corta de una base
de datos. Si la ejecución debe abarcar servicios externos o larga duración,
no mantener locks abiertos: usar claim/lease durable y saga/outbox con fencing,
compensaciones y reconciliación. Es un contrato diferente, no la misma función
con un timeout más largo. Serialización local por account ID no reemplaza la
protección SQL entre réplicas ni resuelve transferencias que afectan dos cuentas.

La unidad de trabajo recibe la transacción explícitamente, no como valor oculto
en context. La primera implementación rechaza anidamiento implícito: el llamador
pasa el DBTX existente a stores/repositorios. Savepoints serían API por dialecto,
no un supuesto común. Query builders y sqlc de la app trabajan sobre ese DBTX.
Migraciones de esquema son comandos explícitos; no ocurren al construir un cliente.

## Ciclo de vida y ownership

- Construir/validar un objeto puro no arranca listeners ni goroutines.
- Abrir recursos usa contexto de startup, timeouts y health check explícito.
  No conservar ese contexto para toda la vida del componente si el caller lo
  cancela al terminar el startup.
- Instancias compartidas se construyen una vez, tienen un dueño y se cierran
  una vez. Un wrapper cierra recursos propios; no cierra el cliente recibido
  a menos que la transferencia de ownership sea explícita.
- Ante startup parcial fallido, cerrar en orden inverso lo abierto y combinar
  error inicial y errores de cleanup sin perder causas.
- Shutdown: desactivar readiness y admisión; detener nuevos consumidores;
  drenar solicitudes/workers con contexto acotado separado; cerrar dispatchers;
  flush/close de productores; cerrar pools; flush de telemetría al final.
  Mantener el servidor operativo mientras aporta visibilidad al drain, y
  cerrarlo al final de esa fase. Ajustar orden según dependencias declaradas.
- Un drain timeout reporta pendientes/activas; no asegura que código arbitrario
  fue terminado. Tareas durables sobreviven en su store/broker.
- Las utilidades puras no implementan lifecycle. Los módulos evitan `Fatal` y
  `os.Exit`; devuelven errores al dueño de la aplicación.

## Ruta de migración por fases

| Fase | Trabajo concreto | Condición de cierre |
|---|---|---|
| 0. Diseño | Inventario completo, configuraciones, contratos, políticas y compatibilidad; revisar el prototipo | Matriz de origen/destino sin huecos; decisiones públicas cerradas antes de implementar |
| 1. Fundaciones | Utilities/profile, config/fuentes/decoder y logging base; aislar globals heredados | Consumidores solos sin engine/drivers/OTel; fixtures de configuración equivalentes |
| 2. Ejecución | Contratos client, retry, breaker, concurrency completo e integraciones opcionales | Race/leaks/admission/ordering/budgets probados; sin dependencia circular hacia raíz |
| 3. Persistencia | database/atomic, PostgreSQL, MySQL y GORM opt-in | Tests reales de ambos backends: cancelación, rollback, aislamiento, pool y commit incierto |
| 4. Consistencia | Idempotency/outbox/inbox/audit; luego saga y stores por dialecto | Crash/restart/claims/fencing/redelivery/compensación y transacción compartida probados |
| 5. Transporte y operación | Router/hosts/JWT, operaciones, HTTP clients, OTel/exporters; extracción de AWS y brokers por servicio | Cada transporte funciona aislado y en combinaciones seleccionadas; parity de métodos públicos |
| 6. Ensamblado y consumidores | Engine mínimo, providers/presets, ejemplos y rollout | Aplicación sin engine y con engine; ambas sin SDK no elegidos; migration guide completo |

No comenzar una fase simplemente porque sus directorios ya fueron nombrados.
Las fases pueden dividirse en PRs por módulo, pero cada PR contiene Config,
contratos, compatibilidad, consumer guard y pruebas relevantes, no solo un move.

## Compatibilidad y publicación

Antes de cambiar paths, inventariar símbolos públicos y su semántica, mapearlos
a nuevas firmas y preparar una tabla old → new con ejemplos. Aliases solo donde
los tipos sean realmente equivalentes. Wrappers donde haga falta adaptar firma;
una ruptura explícita donde cambien garantías, como el timeout del executor.

**Riesgo Go específico:** introducir módulos nuevos debajo de paths que ya
contienen paquetes de la raíz puede causar ambiguous imports para consumidores
que todavía resuelven una versión vieja de esa raíz. No confiar en `go.work` o
replaces para ocultarlo. Usar paths nuevos no superpuestos o coordinar una
versión/raíz de compatibilidad que retire los paquetes antiguos; probar el caso
con versiones publicables de consumidores antes de fijar los nombres definitivos.
`pkg/utilities/retry_backoff` y el destino nuevo `retry` ilustran la diferencia.

El puente heredado puede depender de herramientas; las herramientas no del
puente. No crear require de ida y vuelta raíz → tool → raíz. Mantener la API
vieja no obliga a que los consumidores nuevos instalen el puente.

Conservar releases coordinados para módulos 0.x y tags `directorio/vX.Y.Z`;
concurrency v1 usará versión propia declarada en manifiesto por módulo. Definir
versión de ruptura y mínimos soportados de PostgreSQL/MySQL antes de publicar;
no aumentar floors ni actualizar dependencias por accidente al mover paquetes.
Catálogo de módulos único para checks/release con publicables explícitos,
incluidos nuevos módulos y excluidos ejemplos/harness.

La rama actual ya tiene un prototipo concurrency y cambios de CI de una
iteración anterior. No son autorización de publicación: se evalúan como
experimento contra el contrato de fase 0. Este documento no elimina ni publica
esos cambios. La implementación posterior deberá revisarlos, reemplazarlos o
retirarlos coherentemente.

## Verificaciones de aceptación

| Prueba | Evidencia requerida |
|---|---|
| Inventario | Cada paquete no-test actual tiene destino; funcionalidades nuevas separadas de extracción |
| Dependencias | go.mod y lista de módulos permitidos; ningún tool importa raíz/engine/preset; también revisar test deps |
| Consumidor aislado | `GOWORK=off` y sin replaces de módulos no seleccionados; red desactivada cuando la prueba pueda ejecutarse offline |
| Config | Precedencia, defaults/zero/null, unknown keys, secretos ausentes, referencias inválidas, snapshots y redacción |
| Composición | Orden de decorators, deadlines, retry budget, breaker probes y lifecycle de dependencias compartidas |
| Concurrencia | Límites reales de goroutines/bytes/colas/keys, block/reject, identidad, orden y shutdown |
| SQL PostgreSQL/MySQL | Transacciones con mismo handle, rollback, constraints, cancelación, fallos de red, aislamiento y locks por dialecto |
| Consistencia | Claimed/completed/ambiguous outcomes, duplicate payload, crash windows, fencing, redelivery/inbox y saga resume |
| HTTP y auth | Hosts/subdominios, proxies, CORS separado, JWT/JWKS sin regresiones, listeners separados y pprof opt-in |
| Observabilidad | SDK opcional, trace propagation, cardinalidad, redacción, buffers/exporter shutdown y no-op |
| Compatibilidad | Tabla de símbolos/API, ejemplos antiguos, consumers con versiones mezcladas y tags resolubles |
| Calidad | Tests con race, leaks/lifecycle, benchmarks representativos y lint; no tests que solo reproduzcan el wiring |

Una revisión documental ordinaria no requiere suites runtime. Esta revisión
ejecuta solo los tests existentes del prototipo para contrastar sus capacidades,
sin atribuir esos resultados a la API nueva todavía no implementada.
Al implementar, cada módulo debe demostrar aislamiento real y equivalencia o
ruptura documentada. Una suite verde del monorepo no sustituye el test externo.

## Fuentes técnicas consultadas

Las políticas y el catálogo son decisiones de este diseño. Sus límites se
contrastan con documentación primaria:

- [Go: transacciones](https://go.dev/doc/database/execute-transactions): las
  operaciones de una unidad atómica se ejecutan en su transacción, sin mezclar
  llamadas al DB fuera de ella.
- [Go: cancelar consultas](https://go.dev/doc/database/cancel-operations): usar
  contextos y deadlines para solicitar cancelación al driver.
- [PostgreSQL: aislamiento](https://www.postgresql.org/docs/current/transaction-iso.html):
  distintos niveles tienen garantías y posibles conflictos diferentes.
- [PostgreSQL: SELECT/locking](https://www.postgresql.org/docs/current/sql-select.html)
  y [MySQL: locking reads](https://dev.mysql.com/doc/refman/8.0/en/innodb-locking-reads.html):
  diseñar claims por dialecto; SKIP LOCKED sirve para ciertos patrones de cola,
  no como consulta general de saldos consistentes.
- [OpenTelemetry: señales](https://opentelemetry.io/docs/concepts/signals/):
  separar logs, traces y métricas, con integraciones/exportación explícitas.

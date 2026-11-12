# Prompt de implementación alineado con las respuestas del arquitecto

Reemplazar {MODULO}, {RUTA} y {ORIGEN}. Este prompt prescribe el procedimiento;
no sustituye contratos ni inventa APIs ausentes.

```text
Actúa como responsable del diseño, implementación y validación de {MODULO},
ruta github.com/skolldire/go-engine/{RUTA}, origen actual {ORIGEN}.

Objetivo: un módulo independiente, configurable y componible con evidencia
reproducible. Respeta los cambios existentes y no publiques tags.

0. Fuentes y precondiciones

Lee docs/modularization/decisions.md y respeta la precedencia:
decisiones D25+/D01–D24 → public-contracts y concurrency-api → modularization.md
→ semántica → inventario/índice descriptivos. Notion y ADR 0002 no son normativos.
D32: ante conflicto corrige el documento; no elijas una interpretación propia.

Lee los contratos y la especificación del módulo, inventario, review y ADRs
relevantes. Las respuestas del arquitecto comunicadas por el usuario son fuente suficiente;
no exijas answers.md. Firmas client, once categorías y evidencias E1–E7 ya están
concretadas en public-contracts/decisions. Resuelve detalles técnicos consistentes
y documéntalos; consulta solo una decisión de negocio o contradicción real que
no pueda resolverse con las decisiones vigentes.

No existe core ni lifecycle compartido. client es stdlib-only y opt-in para
quien decora I/O; concurrency/utilities/config no lo importan. El ensamblador
ordena el cierre. No conviertas core en precondición ni lo reimplementes.

Dependencias antes como tareas separadas. Disponible significa en main con
consumidor externo pasando. Para etiquetar, cada dependencia debe tener tag
resoluble con go get. Aplica permisos de la categoría correspondiente; no una
allowlist universal. concurrency no necesita dependencias.

1. Inventario y alcance

Inspecciona implementación, pruebas, provider y consumidores, incluidos presets
y example. Documenta funcionalidades y métodos actuales, origen exacto, destino,
conservar/adaptar/desarrollar/retirar y prueba requerida. Identifica capacidades
faltantes, bugs/riesgos, globals, BaseClient/logger/engine coupling y fuera de
alcance. No consideres existente una capacidad por una mención en documentación.

2. Contrato antes de código

En docs/modules/{MODULO}.md documenta:
- Firmas, tipos, interfaces, errores y preservación/traducción de causas SDK.
- Config local tipado, requeridos, defaults, validación, ausencia/cero/false.
- DefaultConfig y decoder sembrado; literal Go se valida tal cual; sin
  punteros/metadata para distinguir defaults. New no sobrescribe ceros explícitos.
- Constructor específico del contrato: New puro; Open(ctx) I/O de startup;
  Run/Serve(ctx) loops de lifetime. Pool/limiters excepción declarada.
- Dependencias permitidas por categoría, objetos prestados y quién cierra qué.
- Cancelación/deadlines, límites, orden e invariantes aplicables.
- Composición: client envuelve solo I/O; nunca tareas pool ni sentencias dentro
  de Tx. Operation conserva T/contexto/errores, Invocation solo etiquetas fijas.
- Retry desactivado; al habilitar, classifier obligatorio del caller con
  stop/retry/reconcile. No IsRetryable. Commit incierto:
  ErrCommitOutcomeUnknown → reconcile, nunca retry ciego.
- Observabilidad por operación solo con wrappers; slog directo únicamente
  donde el contrato lo liste para eventos internos. No loguear un error que
  el componente devuelve. No elevar errores de negocio a error ni filtrar secretos.
- Compatibilidad: si cambian garantías, ruptura explícita y sin puente.
  task_executor congelado/deprecated y retirado en release de migración.
- Criterios de aceptación, migración y limitaciones.

Para concurrency no alteres concurrency-api.md: implementar Pool/Job/Future,
admisión, pesos, keyed ordering, drain/cancel, streaming y Limiter. Completar
C1–C8 y gates §9; adoptar client no cambia esta API ni su stdlib-only.

3. Implementación

Incrementos pequeños verificables. Sin globals/service locator/env lookup dentro
de herramientas; las fuentes explícitas de configuración y credenciales siguen
sus contratos propios. Sin recursos/retries/workers ocultos. Mantén config y
contratos con su dueño. No SDK/middleware extra fuera de la allowlist.

Propio go.mod, go.work y checks de arquitectura/dependencias. No crear tags,
ni siquiera como atajo de validación. Documenta el prefijo esperado. Cambio de
garantías requiere migración explícita, no alias/bridge. Fakes solo para adapters
en testkit/<adaptador>; no fake universal ni exports de test innecesarios.

4. Pruebas unitarias

Prueban comportamiento e invariantes: API existente/nueva, Config/defaults/zero,
errores/causas, cancelación antes/durante, deadlines, ownership/close idempotente,
saturación/límites/order, partial failures y regresiones.

Composición con dos wrappers y orden cuando el módulo realmente decora I/O;
no forzar client en concurrency o utilidades para satisfacer una prueba.
Sincronización con barreras/synctest donde aplique, sin sleeps de precisión.
Stdlib-only también en tests; fugas con goroutineleak estándar (D28), no goleak.
Fuzzing pertinente de parsers/estados y benchmarks representativos con b.Loop
cuando corresponda. No red/servicios reales en unit tests.

Ejecuta tests con race donde aplique, vet, lint y checks de arquitectura/deps.
Para concurrency repetir con GOMAXPROCS=1 y >1. Coverage detecta huecos, no prueba
corrección por sí sola. No afirmar pruebas de APIs futuras sin implementarlas.

5. Integración: responsabilidad del implementador

Harness pesado en {RUTA}/integration, go.mod propio, fuera del grafo stdlib-only;
build tag integration cuando corresponda. Servicios aislados, cada backend,
fallos reales/cancelación/rollback/recovery y combinaciones soportadas. Limpieza
siempre. Entregar comandos, versiones, entorno y resultados; omitida/no ejecutada
no equivale a aprobada. Si no aplica a una tool pura, justificar, no inventar DB.

6. Revisión independiente

Preparar examples/standalone/{MODULO}/minimal y composed con go.mod propios,
GOWORK=off. minimal importa solo API pública y dependencias necesarias declaradas;
composed agrega únicamente integraciones admitidas. Sin inicializar engine/app.

Replaces solo al módulo y sus dependencias declaradas, documentados. Validar
struct directo y carga config/json; cuando config/yaml esté disponible, YAML con
go.yaml.in/yaml/v3 dentro de ese módulo. No parser YAML estándar inexistente.
Con módulos sin Config/WithMiddleware, usar su contrato real, no inventar métodos.

Comparar go list -deps/go mod graph con allowlist propia de cada consumidor;
ningún SDK/engine no elegido. Script reproducible y corrida offline cuando
sea viable tras preparar dependencias, usando GOPROXY=off y -mod=readonly.

El implementador ejecuta la integración real; nosotros verificamos aislamiento
con estos consumidores. Una prueba no sustituye a la otra.

7. Entrega y release

Entregar docs/modules/{MODULO}.md: inventario/migración, contrato/config, código,
guía, unit tests, integración con evidencias, minimal/composed, dependencias,
limitaciones/pendientes y versión propuesta. Mantener documentación normativa
coherente; registrar decisiones sin convertir ADR 0002 en norma.

D29: sin manifiesto de versiones no etiquetar módulos independientes. Responsable
propone versión y usuario aprueba. concurrency/v1.0.0 puede coexistir con 0.x.
V1 requiere E1–E7 de decisions.md: contrato, calidad funcional, integración
aplicable, independencia local, compatibilidad/gorelease, manifiesto y aprobación
de versión, y consumer contra tag real. Son la concreción técnica autorizada
de las respuestas, sin depender de un archivo answers.md.
Separar aceptación del código/RC de verificación posterior del tag real; no
publicar un tag para completar un checklist sin autorización del usuario.

No declarar terminado por compilar o pasar monorepo: demostrar contrato,
integración aplicable e independencia. Cuando cambien garantías, documentar
ruptura; no prometer un provider compatible que preserve garantías diferentes.
```

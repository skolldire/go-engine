# Configuración, utilidades y construcción

Estado: contrato de diseño. Los nombres y el YAML son ilustrativos, no una API
implementada ni un archivo compatible con el motor actual. Para firmas, campos
y defaults exactos prevalecen [public-contracts](public-contracts.md) y
[concurrency-api](concurrency-api.md).

## Separación de responsabilidades

`config` conoce fuentes, mezcla, snapshots, secciones, resolución y decoding;
no conoce drivers, routers, brokers o SDKs. Cada herramienta posee una estructura
`Config` local y puede construirse directamente con ella, sin importar `config`.
La aplicación define el documento raíz con solo las herramientas que utiliza.
El ensamblador opcional puede recibir secciones raw, pero no conserva la
obligación de que router/logging/health formen parte del motor.

| Capa | Entrada → salida | Efectos permitidos |
|---|---|---|
| Fuente | Archivo, entorno o fuente remota → datos | I/O solo aquí; contexto y límites cuando aplique |
| Mezcla | Fuentes ordenadas → árbol de valores | Determinista, registra origen sin secretos |
| Resolver | Referencias de entorno/secretos → valores resueltos | Backend de secretos inyectado; sin AWS obligatorio |
| Decoder | Sección → Config propiedad de la herramienta | Duraciones, listas y tipos estrictos; no abre conexiones |
| Validador | Config → copia normalizada o error | Defaults y reglas del módulo; no modifica la entrada |
| Constructor | Config validada + dependencias → componente | Solo recursos declarados; conexión mediante Open(ctx) explícito |
| Ensamblador | Configs + registros → aplicación | Construcción ordenada, rollback de startup y shutdown |

Los constructores puros no arrancan goroutines ni listeners. La excepción
explícita es NewPool, que inicia workers locales al construirse, según su contrato. Los componentes
que necesitan I/O usan `Open(ctx, cfg, deps)` o una fase `Start(ctx)` explícita.
Se elige una convención consistente por clase; no todos los objetos deben
implementar un lifecycle artificial. Una función pura de utilities no necesita
Config, ni Close, ni logger.

## Fuentes, precedencia y validación

- Precedencia propuesta, de menor a mayor: defaults de módulo → archivo base →
  archivo del perfil seleccionado → overrides de entorno declarados → overrides
  explícitos de la aplicación. No existe búsqueda silenciosa de archivos fuera
  de las rutas configuradas. La precedencia se prueba con fixtures.
- Los mapas se mezclan por clave; las listas se reemplazan completas, no por
  índice. Las instancias se representan como mapas con nombres únicos. `null`
  desactiva un bloque opcional solamente donde el esquema lo permite.
- El esquema distingue ausencia, cero y desactivación. `max_retries: 0` significa
  ningún reintento; no se transforma silenciosamente en tres. `enabled: false`
  no se pierde por defaults. Límites obligatorios deben ser positivos.
- Las duraciones se escriben con unidad; se rechazan números ambiguos. Hay
  validación estricta de claves desconocidas y referencias entre instancias.
- Los `${VAR}` requeridos sin valor son error al iniciar; `${VAR:-default}`
  puede tener fallback explícito. Valores vacíos y ausentes se distinguen.
  Secretos no resueltos no deben llegar como texto literal a un cliente.
- Resolver referencias después de la mezcla y antes de la validación tipada;
  detectar ciclos y límites de expansión. No evaluar expresiones ni comandos.
- El error identifica sección/campo y origen, sin imprimir contraseñas, DSN,
  tokens o headers privados. Una vista diagnóstica se redacta; nunca serializar
  sin más el Config de todas las herramientas.
- Límites de archivo, profundidad y tamaño de valores evitan entrada ilimitada.
  Los valores se copian a un snapshot inmutable y tienen una revisión rastreable (ID por carga exitosa,
  sin hash público del contenido secreto).
- El contrato común de validación es una convención de métodos/resultados, no
  una dependencia obligatoria de todas las herramientas en un framework validador.

## Utilities y app-profile

`utilities` contiene paquetes pequeños `profile`, `strings`, `collections`,
`pointers` y `defaults`, sin logger/validator/router/config/SDK. Los helpers que
solo renombran una función estándar pueden retirarse de la API nueva, con
forwarding o migración documentada; no crear abstracciones por obligación.

`profile` parsea un scope recibido explícitamente y devuelve un perfil
normalizado. El lector de entorno vive en `config`/ensamblado. La compatibilidad
con `GetScopeValue`/`IsProdProfile` actuales mantiene el fallback a local y el
uso de `SCOPE` solo en el puente heredado; el parser nuevo rechaza tokens desconocidos con ErrUnknownProfile según
el contrato normativo. El perfil permite elegir archivos y tags,
pero no habilita/deshabilita seguridad de red implícitamente.

## Selección de PostgreSQL/MySQL sin importar ambos

Cada backend define su Config específico. Un documento puede describir host,
puerto, nombre, usuario, contraseña o referencia a secreto, TLS y límites de
pool. Host puede ser DNS, IPv4 o IPv6. DSN y campos estructurados son formas
alternativas: rechazar combinaciones ambiguas y escapar mediante el driver.
No concatenar credenciales manualmente.

La aplicación importa y registra solo los adaptadores que admite. Si registra
PostgreSQL pero el documento pide MySQL, falla con "backend no registrado";
no hay fallback. El ejecutable capaz de cambiar entre ambos sí incorpora ambos
por elección explícita, mientras que las aplicaciones de un único backend no.
Registro local a la instancia, sin init global, plugins dinámicos ni service
locator dentro de cada herramienta. Un pool compartido se identifica por nombre;
no crear un pool por cada store de idempotencia/outbox/audit.

Separar parámetros comunes de conexión y parámetros de dialecto (schema,
search_path, charset, timezone, etc.). TLS tiene esquema propio por backend y
no se reduce a un booleano universal. Rechazar capacidades no soportadas al
iniciar. Pérdidas de conexión en commit producen resultados ambiguos que deben
conciliarse; un cambio de backend no hace equivalentes los dialectos.

## HTTP entrante, HTTP saliente y dominios

El cliente REST actual se migra a `httpclient/resty`: no es el futuro router.
`httpclient` configura base URL, transport, pool, timeouts, proxy y límites del
body. Sus retries dependen de seguridad de replay e idempotencia.

El router entrante configura bind address, hosts permitidos, rutas de host,
subdominios, CORS y proxies de confianza por separado. Aceptar un host no crea
DNS, certificados, ingress ni balanceadores. La aplicación suministra esos
recursos o una integración de despliegue explícita. CORS no reemplaza el
allowlist de hosts. Los headers forwarded solo se usan desde proxies confiables.

El matching normaliza host/puerto y aplica precedencia determinista: host exacto
→ wildcard de subdominio explícito → rechazo; sin ruta pública catch-all por
defecto. Wildcards solo de etiqueta completa, no comparación por sufijo que
acepte `evil-example.com`. Conflictos se detectan al construir el router. Un
wildcard no implica aislamiento de tenants: la aplicación valida el tenant y
su autorización. Un host desconocido se rechaza con 421 según el contrato HTTP, sin detalles internos.

## Ejemplo de documento de aplicación futura

Los valores son ilustrativos y no defaults ni límites financieros recomendados.
En esta muestra el ensamblador ha registrado PostgreSQL, un router y el exporter
OTLP/HTTP. Una aplicación que no los usa omite completamente sus bloques.

```yaml
schema_version: 1
application:
  name: payments
  profile: prod

components:
  database:
    main:
      type: postgres
      host: "${DB_HOST}"
      port: 5432
      database: "${DB_NAME}"
      username: "${DB_USER}"
      password: "${DB_PASSWORD}"
      tls:
        mode: verify-full
        ca_file: "${DB_CA_FILE}"
      pool:
        max_open: 20
        max_idle: 5
        max_lifetime: 10m
        operation_timeout: 3s

  concurrency:
    payments:
      workers: 8
      queue_capacity: 64
      admission: block
      admission_timeout: 250ms
      max_job_weight: 65536
      max_in_flight_weight: 1048576
      ordering: per_key
      max_active_keys: 1024
      per_key_capacity: 8
      key_failure: cancel_pending
      task_timeout: 2s
      # Shutdown(ctx, "drain") lo decide la aplicación con su deadline.

  retry:
    transient_io:
      max_retries: 3
      initial_backoff: 100ms
      max_backoff: 2s
      jitter: full
      total_timeout: 5s

  idempotency:
    transfers:
      database_ref: main
      store: postgres
      retention: 48h
      max_response_bytes: 16384

  http_server:
    application:
      listen: ":8080"
      allowed_hosts: [api.example.com, "*.api.example.com"]
      hosts:
        - host: api.example.com
          routes_ref: public_api
        - host: "*.api.example.com"
          routes_ref: tenant_api
      trusted_proxies: ["10.0.0.0/8"]
      read_header_timeout: 5s
      shutdown_timeout: 15s
    operations:
      listen: "127.0.0.1:9090"
      ping: true
      health: true
      pprof: false
      metrics_ref: prometheus_internal

  telemetry:
    service_name: payments
    traces:
      exporter: otlphttp
      endpoint: "${OTEL_ENDPOINT}"
      # El ensamblador entrega estos campos al Provider, no al exporter:
      trace_queue_size: 2048
      trace_batch_size: 512
      trace_flush_interval: 5s
    metrics:
      exporter: prometheus
      instance: prometheus_internal
```

`routes_ref`, extractores de clave, handlers, clasificadores de errores y pasos
de una saga hacen referencia a funciones registradas por la aplicación. La
configuración no contiene Go ejecutable ni descubre código dinámicamente.
Cada referencia anterior debe resolverse antes de abrir recursos. Un exportador
Prometheus solo existe si la aplicación lo incorporó y registró.

## Recarga y rotación

Primera versión: snapshot al inicio; recarga no implícita. La observación de
archivos/dynamic config sería una integración futura explícita, no una capacidad
que se supone presente porque una documentación histórica la menciona.

Cada campo declara si es inmutable, recargable o requiere reemplazo del recurso.
Nivel de log puede actualizarse de forma atómica; driver, credenciales/TLS,
orden de decorators y topología del pool requieren protocolo de reemplazo,
health check, drain y rollback. No reducir un límite de workers rompiendo tareas
activas. Rotación de secretos no provoca migraciones ni reconexiones silenciosas.
En una futura recarga, una revisión inválida conservaría el último snapshot
válido y se reportaría sin datos sensibles. En v1 no existe API Reload y una
nueva carga no modifica recursos existentes.

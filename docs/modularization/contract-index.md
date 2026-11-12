# Índice completo de contratos

Cada fila tiene contrato, configuración y lifecycle en
[public-contracts](public-contracts.md); para concurrency se remite a
[su API exacta](concurrency-api.md). No son módulos ya implementados. Los
providers, presets y testkits se rigen por contratos de familia explícitos,
no por módulos que importan todos los demás. JSON/env son paquetes, no go.mod.
El catálogo no obliga a desarrollar módulos vacíos por adelantado.

| Path objetivo | Contrato aplicable | Config propia |
|---|---|---|
| `audit` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `audit/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `audit/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `auth/jwt` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `auth/jwt/http` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/cognito` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/config` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/dynamodb` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/lambda` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/s3` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/ses` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/sns` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/sqs` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `aws/ssm` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `circuitbreaker` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `client` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `client/logging` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `client/otel` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `client/resilience` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `concurrency` | [Pool/stream/limiter](concurrency-api.md) | PoolConfig, StreamConfig, BatchConfig, LimiterConfig |
| `config` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `config/env` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `config/json` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `config/ssm` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `config/viper` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `config/yaml` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/atomic` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/gorm` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/gorm/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/gorm/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/memcached` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/mongodb` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `database/redis` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `engine` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `engine/providers/aws/cognito` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/aws/dynamodb` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/aws/s3` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/aws/ses` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/aws/sns` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/aws/sqs` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/aws/ssm` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/awsbase` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/database/atomic` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/database/gorm` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/database/memcached` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/database/mongodb` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/database/redis` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/httpclient/resty` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/messaging/kafka` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/messaging/rabbitmq` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/rpc/grpcclient` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/rpc/grpcserver` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `engine/providers/telemetry/otel` | [Factory de provider](public-contracts.md#engine-mínimo-y-factories) | Config tipado de herramienta + referencias solo en ensamblado |
| `health` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `httpclient` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `httpclient/resty` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `idempotency` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `idempotency/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `idempotency/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `inbox` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `inbox/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `inbox/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/events` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/lambda` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/s3` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/ses` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/sns` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/sqs` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/aws/ssm` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/cloud` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/cloud/logging` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `integration/cloud/otel` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `logging` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `logging/logrus` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `messaging/kafka` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `messaging/rabbitmq` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `outbox` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `outbox/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `outbox/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `presets/aws` | [Composición explícita](public-contracts.md#engine-mínimo-y-factories) | Lista explícita de providers; no activa por YAML desconocido |
| `presets/full` | [Composición explícita](public-contracts.md#engine-mínimo-y-factories) | Lista explícita de providers; no activa por YAML desconocido |
| `presets/http` | [Composición explícita](public-contracts.md#engine-mínimo-y-factories) | Lista explícita de providers; no activa por YAML desconocido |
| `ratelimit` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `resilience` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `retry` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `router` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `router/chi` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `router/errors` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `router/operations` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `rpc/grpc` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `saga` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `saga/mysql` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `saga/postgres` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `telemetry/otel` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `telemetry/otellog` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `telemetry/otlpgrpc` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `telemetry/otlphttp` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `telemetry/prometheus` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |
| `testkit/aws/dynamodb` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `testkit/aws/s3` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `testkit/aws/sqs` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `testkit/context` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `testkit/database/redis` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `testkit/httpclient/resty` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `testkit/logging` | [Fixtures de prueba](public-contracts.md#engine-mínimo-y-factories) | Sin Config runtime ni startup implícito |
| `utilities` | [Contrato de familia](public-contracts.md) | Sin Config: funciones puras |
| `validation` | [Contrato de familia](public-contracts.md) | Config local, defaults/required y validación definidos |

Cobertura: 110 entradas concretas. No incluye futuros SDK/backends
no solicitados. Los contratos de familias definen reglas comunes sin forzar
dependencias en sus consumidores.

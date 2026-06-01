# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

### Added
- `make lint-arch` Makefile target: scans `pkg/` for `gorm.io/gorm` imports and exits 1 on violation. Enforces the architectural boundary that GORM stays inside the `database/sql` sub-module.
- README section `## SQL and hexagonal architecture`: anti-pattern vs correct pattern with full code examples (domain port → use case → adapter → `main.go` wiring).
- `GET /health` unified endpoint returning `{status:"healthy"/"unhealthy", checks[], latency_ms}` — designed for ECS Fargate health checks.
- `AppBuilder.RegisterHealthChecker(name, checker)` — registers checkers fluently; auto-mounts `GET /health` on the router when both the health service and router are available.
- `health.HealthResponse` and `health.CheckResult` public types for the new endpoint.
- Constants `HealthStatusHealthy`, `HealthStatusUnhealthy`, `CheckStatusOK`, `CheckStatusError` in `pkg/health`.
- `CHANGELOG.md` (this file).
- `.github/CONTRIBUTING.md` contribution guide.

### Changed
- `ServiceRegistry.Health` type changed from `health.Service` to `*health.HealthService` to allow direct `Register` calls from the builder without type assertions.
- `AppBuilder.WithHealth` now calls `mountHealthIfReady()` internally; health routes mount regardless of whether `WithRouter` is called before or after.
- README rewritten as a persistent API reference: builder method table, Engine getter tables, module map, and package conventions.
- `.github/pull_request_template.md` simplified and translated to English.

---

## [v0.14.0] — 2026-05-28

### Changed
- **Multi-module refactor**: all AWS clients (Cognito, SQS, SNS, SES, S3, SSM, DynamoDB, AWS facade, inbound adapters) moved to the `aws/` sub-module (`github.com/skolldire/go-engine/aws`).
- Messaging clients (Kafka, RabbitMQ, gRPC client/server) moved to the `messaging/` sub-module (`github.com/skolldire/go-engine/messaging`).
- Database clients (Redis, MongoDB, Memcached) moved to dedicated sub-modules under `database/`.
- CI updated to build and test each sub-module independently.
- Root `go.mod` updated with `replace` directives for local sub-module resolution.

> Note: `v0.13.0` and `v0.14.0` point to the same commit — both tags mark this refactor.

---

## [v0.12.0] — 2026-05-27

### Added
- Kafka client with `Producer` and `Consumer` interfaces (`pkg/integration/kafka`).
- `AppBuilder` wires Kafka client from the `kafka:` section of the config YAML.
- `Engine.GetKafkaProducer()` and `Engine.GetKafkaConsumer()` getters.
- Kafka checker for health monitoring (`kafka.Checker`).

---

## [v0.11.0] — 2026-05-27

### Added
- `pkg/utilities/logger/logrusadapter`: Logrus adapter implementing the `logger.Service` interface with ECS log format.
- `pkg/app/default_logger.go`: internal default logger used during builder initialization before user config is loaded.
- `pkg/utilities/logger/writer.go` and ECS field mapping.

### Changed
- Viper service improved: better error messages, cleaner config loading path.
- Logger `SetLogLevel` now returns an error instead of panicking on invalid level.

---

## [v0.10.0] — 2026-05-26

### Added
- `pkg/telemetry/otel`: OpenTelemetry provider with OTLP/gRPC exporter for metrics and traces.
- `AppBuilder.WithOTEL(cfg)` — initializes the OTEL provider and registers its `Shutdown` in the router's graceful shutdown sequence.
- `Engine.GetOTELProvider()` getter.
- `pkg/health`: complete health check package — `HealthService` (concurrent checker execution), `HTTPHandler` (`/live`, `/ready`, `/deps`), and checkers `SQLChecker`, `RedisChecker`, `HTTPChecker`.
- `AppBuilder.WithHealth(cfg)` — creates and registers the `HealthService`.
- `Engine.GetHealthService()` getter.
- `CLAUDE.md` added to document codebase conventions for AI-assisted development.

---

## [v0.9.0] — 2026-01-21

### Changed
- Dependency updates across all modules.

---

## [v0.8.0] — 2026-01-15

### Added
- Full MFA implementation: TOTP and SMS challenges, session management, software token association and verification.
- Cognito service split into dedicated files: `authentication.go`, `mfa.go`, `password.go`, `session.go`, `token.go`.
- `password.go`: change password, forgot password, confirm forgot password flows.
- `session.go`: session management helpers.
- `token.go`: token refresh and revocation.

---

## [v0.7.0] — 2026-01-09

### Added
- Cognito MFA methods: `AssociateSoftwareToken`, `VerifySoftwareToken`, `SetUserMFAPreference`, `GetUserMFAStatus`, `RespondToMFAChallenge`.
- MFA test coverage (`mfa_test.go`, `session_test.go`).

---

## [v0.6.0] — 2026-01-06

### Changed
- Dependency upgrades.

---

## [v0.5.0] — 2026-01-06

### Added
- `pkg/clients/cognito`: full Cognito authentication client — user registration, confirmation, authentication, JWT validation, token refresh, sign-out.
- `Engine.GetCognito()` getter.
- JWKS validation with public key caching (`jwks.go`).

---

## [v0.4.0] — 2026-01-05

### Fixed
- Additional linter violations and test corrections following the v0.1.0 refactor.

> Note: `v0.3.0` and `v0.4.0` point to the same commit.

---

## [v0.3.0] — 2026-01-05

### Fixed
- Linter violations and test corrections across the codebase.

---

## [v0.2.0] — 2026-01-05

### Fixed
- CI workflow configuration errors introduced in v0.1.0.

---

## [v0.1.0] — 2026-01-05

### Added
- `AppBuilder` fluent builder pattern replacing the previous ad-hoc initialization.
- `ServiceRegistry`: centralized registry for multi-instance clients (maps keyed by name).
- `ConfigRegistry`: configuration maps per Clean Architecture layer (repositories, use cases, handlers, batches).
- Multi-instance support for SQS, SNS, DynamoDB, Redis via `*_clients` YAML arrays.
- `pkg/clients/rabbitmq`: RabbitMQ client via amqp091-go.
- `pkg/clients/grpc`: gRPC client.
- `pkg/server/grpc`: gRPC server.
- `pkg/clients/ssm`: AWS SSM Parameter Store client.
- `Engine.GetCustomClient(name)` / `AppBuilder.WithCustomClient(name, client)` for arbitrary client injection.
- Full test suite for `pkg/app` (builder, registry, initializer, router).
- `pkg/app/build` package for request/response construction helpers.
- CI: `version.yml` workflow for automatic tagging.

### Changed
- `pkg/clients/rest` split into `simple` and `advanced` sub-packages.
- S3 and SES client tests significantly expanded.

### Removed
- SQL/GORM auto-initialization removed from the engine. Use `WithCustomClient` to inject GORM connections.

---

## [v0.0.15] — 2025-09-12

### Added
- `pkg/clients/s3`: AWS S3 client (upload, download, delete, presigned URLs).
- `pkg/clients/ses`: AWS SES client (send email, send templated email).
- REST client split into `advanced` (full options) and `simple` (minimal config) variants.

---

## [v0.0.14] — 2025-08-22

### Added
- `pkg/utilities/telemetry`: OpenTelemetry metrics integration (counters, histograms, gauges).
- DynamoDB service mock (`pkg/database/dynamo/mock`).

---

## [v0.0.13] — 2025-04-07

### Fixed
- Redis `SAdd` TTL handling corrected.

---

## [v0.0.12] — 2025-04-04

### Changed
- Redis client configuration fields updated.

---

## [v0.0.10] — 2025-04-03

### Added
- `pkg/clients/grpc`: initial gRPC client with multi-service support.

---

## [v0.0.9] — 2025-04-01

### Added
- Redis `SAdd` operation with TTL support.

---

## [v0.0.8] — 2025-03-31

### Added
- `pkg/utilities/circuit_breaker`: gobreaker wrapper.
- `pkg/utilities/retry_backoff`: exponential backoff retry.
- `pkg/utilities/task_executor`: bounded concurrency worker pool.
- `pkg/utilities/resilience`: combined resilience primitives.

---

## [v0.0.7] — 2025-03-31

### Changed
- Router configuration fields updated (timeouts, CORS, trusted proxies).

---

## [v0.0.1] — 2025-03-27

### Added
- Initial framework: `AppBuilder`, chi-based HTTP router, Viper config, Logrus logger.
- AWS clients: SQS, SNS, DynamoDB (single instance).
- Redis client (single instance).
- `pkg/utilities/logger`: Logrus wrapper with ECS format.
- `pkg/utilities/validation`: go-playground/validator global instance.
- `pkg/utilities/error_handler`: centralized error types.

---

[Unreleased]: https://github.com/skolldire/go-engine/compare/v0.14.0...HEAD
[v0.14.0]: https://github.com/skolldire/go-engine/compare/v0.12.0...v0.14.0
[v0.12.0]: https://github.com/skolldire/go-engine/compare/v0.11.0...v0.12.0
[v0.11.0]: https://github.com/skolldire/go-engine/compare/v0.10.0...v0.11.0
[v0.10.0]: https://github.com/skolldire/go-engine/compare/v0.9.0...v0.10.0
[v0.9.0]: https://github.com/skolldire/go-engine/compare/v0.8.0...v0.9.0
[v0.8.0]: https://github.com/skolldire/go-engine/compare/v0.7.0...v0.8.0
[v0.7.0]: https://github.com/skolldire/go-engine/compare/v0.6.0...v0.7.0
[v0.6.0]: https://github.com/skolldire/go-engine/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/skolldire/go-engine/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/skolldire/go-engine/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/skolldire/go-engine/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/skolldire/go-engine/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/skolldire/go-engine/compare/v0.0.15...v0.1.0
[v0.0.15]: https://github.com/skolldire/go-engine/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/skolldire/go-engine/compare/v0.0.13...v0.0.14
[v0.0.13]: https://github.com/skolldire/go-engine/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/skolldire/go-engine/compare/v0.0.10...v0.0.12
[v0.0.10]: https://github.com/skolldire/go-engine/compare/v0.0.9...v0.0.10
[v0.0.9]: https://github.com/skolldire/go-engine/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/skolldire/go-engine/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/skolldire/go-engine/compare/v0.0.1...v0.0.7
[v0.0.1]: https://github.com/skolldire/go-engine/releases/tag/v0.0.1

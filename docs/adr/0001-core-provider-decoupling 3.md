# ADR 0001 — Decoupling the core from provider imports

- Status: Implemented in `v0.30.0` (2026-08-17)
- Date: 2026-08-05

## Context

`pkg/app` (the `Engine` and `AppBuilder`) currently imports every provider
package directly: AWS (SQS/SNS/SES/S3/SSM/DynamoDB/Cognito), Redis, MongoDB,
Memcached, Kafka, RabbitMQ, gRPC and OTEL. `pkg/config/viper.Config` likewise
imports every provider's concrete `Config` type. As a result, importing
`pkg/app` transitively pulls in all provider SDKs, defeating the opt-in
dependency model the README still advertises.

Phase 3 of the stabilisation plan calls for a "core without provider imports":
a small core (`engine`, lifecycle, config, health, logging contracts) that does
not import any adapter, with adapters composed by the application via typed
registrations.

## Decision

Perform the correctness-and-security stabilisation (phases 0–2) and the
mechanical API normalisation (phase 3: English names, `any`, docs) first, and
**defer the core/provider decoupling to its own dedicated milestone** rather
than attempting it inside the current stabilisation pass.

## Rationale

The decoupling is not a mechanical change; it is a redesign of the public
surface that consumers depend on:

- The `Engine` exposes typed getters (`GetSQSClientByName`, `GetRedisClient`,
  `GetKafkaProducer`, …) and typed fields. Removing provider imports means
  replacing these with a type-erased registry plus generic accessors — a
  breaking change to every consumer call site.
- `viper.Config` embeds every provider `Config`. Decoupling requires a
  per-adapter config-registration mechanism so the core no longer references
  those types.
- Done partially, the build stays broken across a large surface for a long time,
  which is incompatible with the "green from a clean checkout" exit criteria the
  rest of the plan holds itself to.

Landing the high-value, low-risk items first delivers correct timeouts,
lifecycle, security and a normalised API immediately, while keeping every phase
independently shippable and verifiable.

## Consequences

- The milestone landed in `v0.30.0`. `pkg/app` and `pkg/config/viper` were
  removed rather than kept as shims; `pkg/engine` is the core, adapters register
  through `engine.Provider`, and retrieval is generic (`engine.Get[T]`, or the
  `From` helper each provider exposes).
- The repository is now nine modules, one per adapter family, so the boundary is
  enforced by the module graph and not only by convention: the core cannot
  import an adapter without a `require` line appearing in its `go.mod`, which is
  what `make lint-arch` reads.

## Follow-up

Delivered as phase C (C1–C9) of `docs/plan-auditoria-2026-08.md`. The migration
path is `docs/migration-engine.md`.

Superseded note: previously tracked as the remaining work of phase 3 in
`docs/plan-mejoras-go-engine.md`
(items 1–2). This ADR will be updated to "Superseded" when that milestone lands.

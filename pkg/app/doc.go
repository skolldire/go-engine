// Package app assembles the Engine from configuration via the fluent AppBuilder.
//
// Deprecated: use [github.com/skolldire/go-engine/pkg/engine] instead.
//
// This package is the coupled assembly the 2026-08 audit identified as the
// blocker for publishing the library: importing it resolves ~110 external
// modules (547 packages), because Engine and the configuration struct name the
// concrete types of all fifteen adapters. A service that only wants an HTTP
// router pays for the entire AWS SDK, the MongoDB driver, Kafka, RabbitMQ and
// a QR-code library.
//
// pkg/engine inverts that dependency: the core knows no adapter, adapters
// implement engine.Provider, and a pure HTTP service resolves 19 modules
// (41 packages). See docs/migration-engine.md for the mapping between the two
// APIs, and docs/plan-auditoria-2026-08.md for the reasoning.
//
// This package continues to work and is still tested; it is not scheduled for
// removal before v0.30.0.
package app

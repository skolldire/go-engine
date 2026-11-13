# Logical observation with real tools

Uses client/logging, client, logging, resilience, retry, circuitbreaker, concurrency and ratelimit. Each local replace points five levels up to the actual module; no backend/service is imitated. v0.0.0 versions are development placeholders.

A caller-injected memory transport fails twice then succeeds. All three remote attempts happen inside resilience, with one outer client/logging completion. A second logical call returns an uncertain partial result and is classified Reconcile without a retry. The returned error retains both causes and no private error text enters the record. Concurrency permits are released; breaker stats see each attempt. No engine, app, SDK, filesystem or socket is initialized.

Tests run inside synctest, including a four-second logical duration made of two one-second attempts and two seconds backoff. main does not depend on precise real timing; its retry waits are one nanosecond and it asserts counts/structure, not elapsed wall time.

```sh
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run .
GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go test -race ./...
```

From root: scripts/check-module.sh client/logging consumers. The composed graph includes its real tools; minimal does not include logging/resilience.

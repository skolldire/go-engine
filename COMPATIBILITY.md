# Compatibility Policy

`go-engine` follows [Semantic Versioning](https://semver.org/).

## Modules

The repository is a set of modules, released together under one tag:

| Module path | Directory |
|---|---|
| `github.com/skolldire/go-engine` | `/` |
| `github.com/skolldire/go-engine/aws` | `/aws` |
| `github.com/skolldire/go-engine/messaging` | `/messaging` |
| `github.com/skolldire/go-engine/http` | `/http` |
| `github.com/skolldire/go-engine/database/sql` | `/database/sql` |
| `github.com/skolldire/go-engine/database/redis` | `/database/redis` |
| `github.com/skolldire/go-engine/database/mongodb` | `/database/mongodb` |
| `github.com/skolldire/go-engine/database/memcached` | `/database/memcached` |
| `github.com/skolldire/go-engine/preset/full` | `/preset/full` |

Go requires a per-module tag (`aws/v0.30.0`, `database/redis/v0.30.0`, …) for
every sub-module. They are cut from the same commit and carry the same version,
so a sub-module never lags the core.

## Pre-`v1.0`

While the module is on `0.x`, the public API is **not yet stable**. Minor
versions may contain breaking changes as the API is stabilised toward `v1.0`.
The current stabilisation work (see `docs/plan-mejoras-go-engine.md`) intentionally
introduces breaking changes to fix incorrect timeout, lifecycle and security
contracts. Every breaking change is recorded in [`MIGRATION.md`](MIGRATION.md).

Pin an exact version if you need stability before `v1.0`:

```bash
go get github.com/skolldire/go-engine@v0.20.0
```

`v0.30.0` is the largest break so far: `pkg/app` and `pkg/config/viper` were
removed outright, with no deprecation window, and the repository was split into
the modules above. See [`MIGRATION.md`](MIGRATION.md).

## From `v1.0` onward

Once `v1.0` is tagged:

- **Patch** (`v1.0.x`): bug fixes and security fixes only; no API changes.
- **Minor** (`v1.x.0`): additive, backward-compatible changes (new functions,
  new optional config fields, new adapters). Existing code keeps compiling.
- **Major** (`vX.0.0`): breaking changes, released on a new module path
  (`/v2`, `/v3`, …) per Go module rules.

### Deprecation window

A symbol scheduled for removal is marked with a `// Deprecated:` doc comment for
at least one minor release before it is removed in the next major. Deprecated
symbols keep working during that window.

## What counts as the public API

- Exported identifiers under `pkg/`, `aws/`, `database/`, `messaging/`, `http/`,
  `provider/` and `preset/`.
- The YAML configuration keys each provider declares through `ConfigKey()`.

The following are **not** part of the compatibility guarantee and may change at
any time:

- Anything in `_test.go` files, any `testutil` package, and internal helpers.
- Log message wording and unexported behaviour.
- The exact error strings (use `errors.Is`/`errors.As` against exported error
  values, not string matching).

## Security fixes

Security fixes may ship in a patch release even if they slightly alter
behaviour (for example, the JWT middleware now fails closed when issuer/audience
are unset). Such changes are called out in `MIGRATION.md` and `SECURITY.md`.


## Go version policy

The `go` directive in every module is the **minimum Go version required to build
this library**, and it is deliberately kept at a version with no known
vulnerabilities rather than at the oldest version that would still compile.

Currently: **Go 1.26.6**.

The reasoning: a `toolchain` directive only affects builds of *this* repository —
Go ignores it in a dependency. So declaring a lower floor while building our own
CI with a patched toolchain would hand consumers a version they can compile with
that still carries the vulnerabilities `govulncheck` reports (six were reachable
in Go 1.26.5: `net/http`, `encoding/asn1` and `golang.org/x/net/idna`).

Raising the floor is treated as a breaking change and is recorded in
[`MIGRATION.md`](MIGRATION.md).

`scripts/smoke-external-consumer.sh` builds an external consumer with
`GOTOOLCHAIN` pinned to exactly this floor, so the promise is tested rather than
assumed.

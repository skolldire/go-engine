# Compatibility Policy

`go-engine` follows [Semantic Versioning](https://semver.org/).

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

- Exported identifiers under `pkg/`, `aws/`, `database/`, `messaging/`.
- The YAML configuration schema consumed by `pkg/config/viper`.

The following are **not** part of the compatibility guarantee and may change at
any time:

- Anything in `_test.go` files, `pkg/testutil`, and internal helpers.
- Log message wording and unexported behaviour.
- The exact error strings (use `errors.Is`/`errors.As` against exported error
  values, not string matching).

## Security fixes

Security fixes may ship in a patch release even if they slightly alter
behaviour (for example, the JWT middleware now fails closed when issuer/audience
are unset). Such changes are called out in `MIGRATION.md` and `SECURITY.md`.

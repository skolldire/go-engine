# D54 L5 — validation: approved contract, decisions and evidence

Experimental candidate, not released. Proposed version v0.1.0 is **unapproved**; the catalog row stays `unreleased` / `-`.

## Initial state

Branch `feature/split`, HEAD `bde907834b48ebeab6c8481629aeb0363cdea1b0`. `git status --porcelain` was **not** empty at the start: the worktree held the uncommitted work of lot L4, namely `httpclient/`, `httpclient/resty/`, `examples/standalone/httpclient/` and edits to `modules.tsv`, `go.work`, `CHANGELOG.md` and `MIGRATION.md`. That work is not part of this lot and was left as found. L5 touches the same four root files, but only by adding its own lines (two catalog rows, two workspace entries, one changelog line, one migration section), so the two lots stay separable line by line.

Go go1.27.4 darwin/arm64. `go list -m github.com/go-playground/validator/v10` resolves v10.30.6 in the repository and that exact version is required here; no dependency or `go` directive was updated. No file with " 2" in its name was touched. No commit, push, tag or workflow dispatch was made.

## D54 as approved (architect)

Module `github.com/skolldire/go-engine/validation`. Catalog row: kind `tool`, track `unreleased`, version `-`, checks `production`, deps `github.com/go-playground/validator/v10`. No library type in the public API.

API: `Config{FieldNames, RequiredStruct}`, `Rule func(ctx, value any, param string) bool`, `Dependencies{Rules map[string]Rule}`, `FieldError{Path, Rule, Param}`, `Errors []FieldError` with `Error()`, `Validator`, `DefaultConfig`, `Config.Validate`, `New`, `Validator.Struct(ctx, value)`. Errors: `ErrInvalidConfig` (`ConfigError{Field, Reason}`), `ErrInvalidCall`, `ErrNilContext`, `ErrInvalidRule`. Defaults: `FieldNames` `"json"`, `RequiredStruct` true.

Sections C (behavior) and D (migration) are implemented as written. No signature, exported field, default, exported error or format was added, removed or changed relative to D54.

## Inventory and migration map

| Origin (`pkg/utilities/validation`) | Destination | Action | Proof required |
|---|---|---|---|
| `NewValidator() *validator.Validate` | `New(cfg, deps) (*Validator, error)` | adapt | construction and config tests; no library type exported |
| `RegisterTagNameFunc` with the json tag, `-` mapped to empty | `FieldNames` `"json"` (default) or `"go"` | adapt | path tables for both, nested and in slices and maps |
| `registerCustomValidators`: `not_empty`, registration error discarded | not built in; `Dependencies.Rules`, failures are `ErrInvalidRule` | retire the built-in, develop injection | invalid-rule table; `not_empty` registered as a rule in tests and MIGRATION |
| `SetGlobalValidator` / `GetGlobalValidator` with a mutex | none | retire | source-level test: no package variable but the sentinels, no accessor |
| callers receive `validator.ValidationErrors` (with `Value()`) | `Errors` / `FieldError` without values | develop | password test over every formatting verb; library error unreachable |
| a malformed tag panics in the caller | error matching `ErrInvalidCall` | develop | malformed-tag table |
| `GetGlobalValidator().Var(value, tag)` in `aws/pkg/clients/sns`, `ses`, `ssm` | none | keep legacy unchanged | no legacy file modified; documented in MIGRATION |

### Starting facts verified against the code, with deviations

Confirmed: `NewValidator` returns the library's `*validator.Validate`, so the library type is the public API; field names come from the json tag; `not_empty` is registered and the registration error is discarded (`_ = err`); `SetGlobalValidator` and `GetGlobalValidator` keep a global behind a `sync.RWMutex`.

Deviations found:

- `GetGlobalValidator` also **creates** the global lazily on first use; there is always a validator even if none was set.
- `not_empty` is "not blank after trimming", not merely "not empty", and returns false for any non-string field. It depends on `pkg/utilities/helpers`.
- The three AWS consumers do not validate structs at all. Every call is `GetGlobalValidator().Var(value, "...")` on a single value (`required,e164`, `required,email`, `max=2048`, `max=4096`). D54 offers `Struct` only, so those calls have no direct counterpart; MIGRATION says so. This does not block the lot, since consumers are not migrated, but it matters for whoever migrates them.

## Internal decisions

Each is inside the approved signatures. The ones marked **(contract precision)** define observable behavior the approved text left open, and are the ones to look at first when validating the lot.

1. **(contract precision) What `RequiredStruct` means, and the library option of the same name.** D54.C defines it by the top-level value: with it, a struct or a pointer to one; without it, "any value the library can validate". The library validates non-structs only through `Var` with a tag, so "validable" is implemented as: a slice, array or map is validated element by element (`dive`), and anything else is `ErrInvalidCall`. Separately, the library has an option called `WithRequiredStructEnabled` that changes how `required` treats nested struct fields. The name coincidence is noted because the field could be read as that option; it is implemented as D54.C states.
2. **(contract precision) `WithRequiredStructEnabled` is always on.** The library recommends it and makes it the default in v11; without it `required` on a struct-typed field is silently ignored. This differs from the legacy validator and is listed in MIGRATION.
3. **(contract precision) Map keys appear in `Path`** (`labels[region]`), as the library reports them. A key is data, not type information. Fuzzing surfaced this: an input used as a map key shows up in the error by design. It is documented with the advice not to key maps by secrets. Removing keys from paths would make a failure inside a map impossible to locate.
4. **(contract precision) Rule names are limited to letters, digits, `_` and `-`**, a subset of what the library accepts, so a name can never contain a character the tag syntax gives meaning to.
5. **(contract precision) A malformed tag yields the library's message only for its two tag-parsing panics** (undefined rule, invalid tag), which are built from the rule and field names alone. Any other library panic is replaced by a fixed message, because it cannot be shown to be free of field values.
6. **Built-in collisions are detected by probing.** The library has no lookup and registering over a built-in replaces it silently. `New` tries the name on a throwaway value: only the library's "undefined validation function" panic means the name is free. This also catches aliases and reserved tags.
7. **"Duplicate" rule names cannot occur**: `Dependencies.Rules` is a map. Rules are registered in sorted order so the one reported for an invalid set is deterministic.
8. **Rule panics are told apart from library panics by a per-call flag** set while a rule runs. A rule's panic is re-raised with its original value; a library panic becomes `ErrInvalidCall`. The re-raise keeps the value but not the original stack.
9. **The caller's context reaches rules unwrapped.** The library is given a derived context carrying the per-call state; the rule receives the context passed to `Struct`.
10. **`json:"-"`, no tag and an options-only tag fall back to the Go name**, which is what the library does when the tag-name function returns an empty string, and what the legacy function did.
11. **The root type name is stripped from paths** (`Signup.email` → `email`), matching the example in D54.
12. **`FieldError.Rule` is the tag as written** (the library's `Tag()`), so an alias is reported by its alias name.
13. **A zero-value `Validator`** (not from `New`) returns `ErrInvalidCall` instead of panicking.
14. **An indirect requirement on `github.com/stretchr/testify` v1.12.1.** The library's graph selects testify v1.8.4 through a dependency's tests. That version's metadata is not in the offline module cache, and `go list -m all`, which the generic checker runs, needs it. v1.12.1 is the version the repository already resolves. `config/viper` carries the same line for the same reason. The other indirect requirements also use the repository's selected versions (`x/crypto` v0.59.0, `x/sys` v0.50.0, `x/text` v0.44.0).
15. **`go.sum` was assembled from checksums already in the repository** (`go.sum`, `aws/go.sum`). `go mod tidy -diff` could not be run offline; builds and tests with `-mod=readonly` pass.
16. **One fuzz target, capped at 512 bytes of input**, following what L4 learned about the fuzzing engine's minimization.
17. **No stress campaign**, per the light profile; a race test runs 32 goroutines × 200 validations on one `Validator`.

## Consumers and independent graph

Each consumer has its own `go.mod`, replaces only the in-repository modules it uses, and is verified offline by the generic checker (`GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly`). The validator library arrives through `validation`, so `consumer-deps.tsv` is unchanged.

- `examples/standalone/validation/minimal`: `validation` alone. Two injected rules, one reading the context; a built-in name refused at construction; failures in declaration order; the submitted password absent from the error.
- `examples/standalone/validation/composed`: `validation` and `router/errors`. A handler validates a JSON body and answers 422 with the failing paths in `detail`, 201 for a valid body and 400 for malformed JSON; neither the password nor the rejected value is in the response.

## Executed evidence

Local platform only: go1.27.4 darwin/arm64, Apple M1 Max. Linux was not run and no CI was dispatched. `make lint` used golangci-lint 2.14.0 from the session helper `/private/tmp/logging-env.sh` (PATH and caches only; not a repository dependency). Only what follows was executed.

```sh
(cd validation && GOWORK=off GOPROXY=off go test -race -count=5 ./...)
(cd validation && GOWORK=off GOPROXY=off go vet ./...)
(cd validation && GOWORK=off GOPROXY=off go test -count=1 -cover ./...)
make check-module M=validation MODE=deps
make check-module M=validation MODE=consumers
make check-module M=validation MODE=quick
make check-module M=validation MODE=fuzz
make check-module M=validation MODE=bench
./scripts/modules.sh validate
./scripts/modules.sh lint-lists
make lint
make lint-arch
make lint-docs
make check-modules
make test
make check-module M=config MODE=consumers
make check-module M=config/viper MODE=consumers
make check-module M=router/errors MODE=consumers
gorelease -base=none -version=v0.1.0   # in a copy outside the repository
```

Final result: every command exits 0. Three of them failed on their first run and were run again; the first results are reported here because they happened.

- **`go test -race -count=5` and the generic quick mode failed first**, on `TestReuseDoesNotGrow`: `allocations per call grew with reuse: 102 -> 103`. The test compared two `testing.AllocsPerRun` measurements with no margin, and the count moves by one between runs (map iteration order, pooled buffers). That is a defect of the test's oracle, not of the code: growth with reuse would be proportional to the 5000 calls between the two measurements. The assertion now allows a margin of 4. Rerun: `-race -count=5` passes (2.7s); quick passes with `-race -count=20` at `GOMAXPROCS` default, 1 and 4 (8.1s, 6.6s, 6.5s) and `go vet`.
- **`modules.sh validate` failed first** with `uncatalogued module harness/legacy-inventory`. That directory belongs to the following lot (L6), whose harness was created while this campaign was running, before its catalog row. With the row added, validate passes. Nothing in `validation` was involved.
- **Fuzz**: `FuzzStruct` 30s, 822,185 executions, passes. An earlier exploratory run found a failure in the target's own oracle (an input used as a map key appears in the path by design); the oracle was corrected and decision 3 records the behavior.
- **Benchmarks** (500ms, count 3, one struct with nested, slice and map fields): `Struct` 1.42–1.46 µs/op, 648 B/op, 21 allocs/op; the library used directly 1.54–1.59 µs/op, 794 B/op, 31 allocs/op. A failing struct with 13 failures: 3.7–3.9 µs/op, 4.8 KB/op, 97 allocs/op. The wrapper is not slower than the library on the passing path on this machine; this is a comparison, not a performance claim.
- **Coverage**: 96.5% of statements, as a diagnostic.
- **Root**: `lint-lists` passes; `make lint` reports 0 issues in every catalog module; `lint-arch`, `lint-docs`, `check-modules` and `make test` pass; the consumers of `config`, `config/viper` and `router/errors` still pass.
- **gorelease**, run outside the repository on a copy of the module with `-base=none -version=v0.1.0`: "v0.1.0 is a valid semantic version for this release". First version, no base to compare against.

Tests cover every item D54 lists for this module. Raw logs are under `validation/evidence/`, which is git-ignored; the root-level logs of the lot are there too. `SOURCES.sha256` and `EVIDENCE.sha256` list the SHA-256 of every source of the candidate and of every log; this file is not listed in them so that it can name them.

## Status

E1 contract and documentation: complete. E2 functional quality and security, light profile: complete locally. E3 integration (in-memory HTTP through `httptest`): complete. E4 independence and real composition: complete. E5 gorelease without a base: executed, passes. E6 version approval: **pending**; v0.1.0 is proposed, not approved. E7 consumers against a real tag: deferred until a tag exists.

`make all` was not run. No `git add`, commit, push or tag was executed by this work; files that appear staged were staged outside it and left as found.

Manifest hashes at the time of writing:

```
d8f883e7388ce3ba61a061b317286456f8f4026e12281906f601b3e2be22ee67  validation/SOURCES.sha256
a50fbbe8e6cdb28768d8bec3584626940e428c9a2c960920079cd66a6ae154b0  validation/EVIDENCE.sha256
```

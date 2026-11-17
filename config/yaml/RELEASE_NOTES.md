# D55 L5 — config/yaml: approved contract, decisions and evidence

Experimental candidate, not released. Proposed version v0.1.0 is **unapproved**; the catalog row stays `unreleased` / `-`.

## Initial state

Branch `feature/split`, HEAD `bde907834b48ebeab6c8481629aeb0363cdea1b0`. `git status --porcelain` was **not** empty at the start: the worktree held the uncommitted work of lot L4 (`httpclient/`, `httpclient/resty/`, `examples/standalone/httpclient/`, and edits to `modules.tsv`, `go.work`, `CHANGELOG.md` and `MIGRATION.md`). It is not part of this lot and was left as found; L5 only adds its own lines to those four files. The `config` core and `config/viper` had no uncommitted change and were not modified.

Go go1.27.4 darwin/arm64. `go list -m go.yaml.in/yaml/v3` resolves v3.0.6 in the repository and that exact version is required here; no dependency or `go` directive was updated. No file with " 2" in its name was touched. No commit, push, tag or workflow dispatch was made.

## D55 as approved (architect)

Module `github.com/skolldire/go-engine/config/yaml`, its own module like `config/viper`. Catalog row: kind `source`, track `unreleased`, version `-`, checks `production`, deps `config,go.yaml.in/yaml/v3`.

API, same shape as `config/json`: `Config{Path, MaxBytes, Required}`, `Dependencies{FS, Reader}`, `DefaultConfig`, `Config.Validate`, `New(cfg, deps) (config.Source, error)`. The source implements `fmt.Stringer` as `yaml:<path>`. Defaults: `MaxBytes` 1 MiB, `Required` true.

Section C (strict parsing over `yaml.Node`) and D (relation to `config/viper`) are implemented as written. No signature, exported field, default, exported error or format was added, removed or changed relative to D55. The expansion limit is an unexported constant documented in the README, since D55 lists no exported constant.

## Inventory

`config/yaml` has no legacy origin of its own; YAML is read today by Viper inside `config/viper` and the engine.

| Origin | Destination | Action | Proof required |
|---|---|---|---|
| none (YAML read by Viper) | `config/yaml` source | develop | the whole test suite |
| `config/json`: `Config`, `Dependencies`, `New`, `Load`, bounded read, close handling | same shape and behavior | adapt | construction, FS/Reader, Required, MaxBytes boundary, close tests |
| `config/json` value domain (`json.Number`, strings, bools, nil, maps, lists) | same domain | keep | equivalence test: same Document and same Decode result |
| `config/viper` | unchanged | keep | no file modified; consumer regression |
| `config` core | unchanged | keep | no file modified; consumer regression |

## How yaml.v3 v3.0.6 resolves scalars

Read from `resolve.go` and `decode.go` of the pinned version, then fixed by `TestScalarResolution`, which also checks that the library assigns the same tag to every plain scalar in the table.

- The parser resolves a plain scalar when it builds the node and stores the short tag in `Node.Tag`; an explicit tag sets `TaggedStyle`. A quoted or block scalar gets `!!str`.
- Booleans are exactly `true/True/TRUE` and `false/False/FALSE`. `yes`, `no`, `on`, `off`, `y`, `n` are **not** in the resolution table: they are `!!str`. (The library converts them only when decoding into a typed Go `bool`, a path this source does not use.)
- Null is `~`, `null`, `Null`, `NULL` and the empty scalar.
- Integers: `strconv.ParseInt(s, 0, 64)` after removing `_`, so `0x` hexadecimal, `0o` octal, `0b` binary **and YAML 1.1 octal with a leading zero** (`017` = 15) are integers. `08` fails as octal and falls through to the float pattern, giving 8.
- An integer beyond 64 bits matches the float pattern and is tagged `!!float`.
- Floats match `^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$`; `.inf`, `+.inf`, `-.inf`, `.nan` and their capitalized forms come from the table.
- A plain scalar starting with four digits and a dash is tried as a timestamp against four layouts and tagged `!!timestamp`.
- Sexagesimal (`1:30`) is not supported by v3 and is a string.
- Decoding into `*yaml.Node` copies the node tree: no alias is expanded, and the library's alias-ratio protection does not apply. Anchors are registered before their children are parsed, so a recursive alias produces a cyclic node graph that the caller must detect.
- A syntax error on the first line is reported without `line N`, as is an unknown anchor.

## Internal decisions

The ones marked **(contract precision)** define observable behavior the approved text left open.

1. **(contract precision) A document that is only null is an empty document.** D55 says an empty file is an empty Document. `---`, `~`, `null` and a comment-only file are treated the same; a root that is a list or another scalar is an error.
2. **(contract precision) Input must be UTF-8.** The library also reads UTF-16; a configuration file in another encoding is far more likely a mistake. A UTF-8 byte order mark is accepted.
3. **(contract precision) Malformed input is `*config.ConfigError`; limits match `config.ErrLimitExceeded`.** This mirrors `config/json`, which reports malformed JSON as `ConfigError`. The expansion limit wraps `ErrLimitExceeded`, like `MaxBytes`.
4. **(contract precision) An explicit tag that contradicts its content is an error** (`!!int nope`, `!!bool yes`, `!!null x`), not a coercion. An explicit `!!timestamp` is refused as a disallowed tag; only a plain scalar that the library would resolve as a timestamp is kept as text.
5. **(contract precision) Timestamp-like and `yes`/`no` keys are string keys**, consistent with how the same text is treated as a value.
6. **(contract precision) The limit counts every node visited during conversion**, keys included, and applies to documents without aliases too. Each alias use is converted again rather than shared, so the count is the real size of the result and uses of an alias are independent copies.
7. **(contract precision) Merge details.** A list of maps gives precedence to the earlier map; a merge is shallow; two `<<` keys in one map are a duplicate key; a quoted `"<<"` is an ordinary key; the merge value must be a mapping or a list of mappings.
8. **Numbers keep their digits.** Canonical decimal integers pass through unchanged. Other integer forms are resolved by the library and written back in decimal. Floats are rewritten textually into JSON number syntax (`.5` → `0.5`, `1.` → `1`, `+1e3` → `1e3`) without passing through a `float64`, so long decimals and integers beyond 64 bits are not rounded; the library's `float64` is a fallback only when the text cannot be rewritten.
9. **Syntax errors drop the library's message and keep its line.** The message can quote input. Consequence: no line for an error on line 1 or for an unknown anchor, because the library gives none. Documented.
10. **A panic inside the library is turned into "invalid YAML".** The library converts its own failures to errors, but it parses untrusted input and a configuration loader should not crash the process on a parser bug. Fuzzing did not trigger this path.
11. **Recursive aliases are detected with a set of collections being converted.**
12. **`ConfigError.Field` uses the same lowercase names as `config/json`** (`path`, `max_bytes`, `fs`, `dependencies`), since D55 asks for the same shape; for a parse error it is the key path, or the file path when there is none.
13. **The bounded reader is duplicated from `config/json`**, where it is unexported; changing the `config` core is out of scope.
14. **A composed consumer was added** next to the `minimal` one D55 lists, because the generic checker runs both for every production module. It combines `config/yaml`, `config` and `validation`.
15. **Fuzz time is 60s per target through `scripts/modules/overrides.tsv`** (`config/yaml fuzz-time 60s`), the data file the checker reads for this; no script was edited. Raw-document inputs are capped at 512 bytes.
16. **`go.sum` was taken from `config/viper/go.sum`** for the two modules required.

## Consumers and independent graph

Each consumer has its own `go.mod`, replaces only the in-repository modules it uses, and is verified offline by the generic checker. No external module is imported directly, so `consumer-deps.tsv` is unchanged.

- `examples/standalone/config/yaml/minimal`: `config` and `config/yaml` with an in-memory filesystem (`testing/fstest`). A base file and an override, an optional file that does not exist, an anchor, an alias and a merge key whose explicit key wins, a list replaced, a map merged, a key removed with `null`, a hexadecimal port, and `Decode` over a `defaults()` value. It also shows a duplicate key reported by path and line without the value, and a quoted number that is not converted.
- `examples/standalone/config/yaml/composed`: `config/yaml`, `config` and `validation`. Settings are loaded, decoded and validated; invalid values are named by path.

## Executed evidence

Local platform only: go1.27.4 darwin/arm64, Apple M1 Max. Linux was not run and no CI was dispatched. Only what follows was executed.

```sh
(cd config/yaml && GOWORK=off GOPROXY=off go test -race -count=5 ./...)
(cd config/yaml && GOWORK=off GOPROXY=off go vet ./...)
(cd config/yaml && GOWORK=off GOPROXY=off go test -count=1 -cover ./...)
make check-module M=config/yaml MODE=deps
make check-module M=config/yaml MODE=consumers
make check-module M=config/yaml MODE=quick
make check-module M=config/yaml MODE=fuzz
make check-module M=config/yaml MODE=bench
gorelease -base=none -version=v0.1.0   # in a copy outside the repository
```

The root commands (`modules.sh validate`, `lint-lists`, `make lint`, `lint-arch`, `lint-docs`, `check-modules`, `make test`, and the consumers of `config`, `config/viper` and `router/errors`) were run once for the lot with both modules registered and are recorded in [../../validation/RELEASE_NOTES.md](../../validation/RELEASE_NOTES.md), including one first-run failure of `validate` caused by a directory of the following lot. All pass, with 0 lint issues in this module.

- **Race**: `-race -count=5` passes (4.9s). The generic quick mode adds `-race -count=20` at `GOMAXPROCS` default, 1 and 4 (16.3s, 16.3s, 21.2s) and `go vet`.
- **Fuzz**, 60s per target, all pass with no finding: `FuzzDocument` 2,315,318 executions; `FuzzMergeKeys` 1,054,676; `FuzzAliases` 17,514. The last number is low for two reasons: each execution builds and converts a document that can expand to the 100,000-node limit, and Go's fuzzing engine spends part of the time minimizing inputs. It exercises the limit rather than the breadth of inputs; `FuzzDocument` is the broad one.
- **Benchmarks** (500ms, count 3, the same 40-section settings in both formats, 5.6 KB of YAML): YAML 390–433 µs/op, 284 KB/op, 5,421 allocs/op; `config/json` 63–65 µs/op, 74 KB/op, 1,764 allocs/op. Loading YAML costs roughly 6–7 times JSON on this machine. Configuration is loaded at startup, so this is recorded as a fact rather than treated as a problem.
- **Coverage**: 94.6% of statements, as a diagnostic.
- **No stress campaign**, per the light profile; a race test runs 16 goroutines × 100 loads on one source.
- **gorelease: exit 1, not a pass.** `go: github.com/skolldire/go-engine/config@v0.0.0: module lookup disabled by GOPROXY=off`. The module requires the unpublished `config` at the development placeholder, which gorelease cannot resolve without the local replace. This is a distribution impediment that lasts until `config` has a published version; no tag, proxy or dependency change was made to get around it. `config/viper` is in the same position.

Tests cover every item D55 lists for this module, including the equivalence with `config/json`: the same content gives the same `Document` and the same `Decode` result. Raw logs are under `config/yaml/evidence/`, which is git-ignored. `SOURCES.sha256` and `EVIDENCE.sha256` list the SHA-256 of every source of the candidate and of every log; this file is not listed in them so that it can name them.

## Status

E1 contract and documentation: complete. E2 functional quality and security, light profile with reinforced fuzzing: complete locally. E3 integration with the `config` loader: complete. E4 independence and real composition: complete. E5 gorelease without a base: executed, **blocked** by the unpublished `config` dependency. E6 version approval: **pending**; v0.1.0 is proposed, not approved. E7 consumers against a real tag: deferred until tags exist.

No `git add`, commit, push or tag was executed by this work.

Manifest hashes at the time of writing:

```
84668dbfa3f7858e253445c67732e0d49f2cd08e5ca3197b9032168ddc219bc9  config/yaml/SOURCES.sha256
81dd1c39c374a6619393c9faa1db33e1c7f37a0aec090ca35908205c0522ecbe  config/yaml/EVIDENCE.sha256
```

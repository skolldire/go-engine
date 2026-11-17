# Releases and repository modules

`modules.tsv` is the module inventory, verification and approved-version source of truth (D29/D40 approved). No independent version is approved by this iteration. All new tools stay unreleased until explicit user approval.

## Evidencias ola 0

Initial branch feature/split, HEAD 476792c. `git status --porcelain` empty. Go go1.27.4 darwin/arm64; system bash 3.2.57. Existing source/tests/consumers and declared v0.30.0 will remain unchanged.

Confirmed initial findings: CI's explicit lists omit config/config-viper/client/retry/circuitbreaker/ratelimit/resilience; they already include concurrency. version.yml's coordinated loops incorrectly include concurrency. Existing sibling v0.0.0 replaces cannot publish; the old checker mixes all tracks and rejects the existing tree. Version scripts assume one coordinated track. MODULES/workflows/workspace duplicate inventories; CLAUDE describes only the older families.

Count clarification: 24 check-* scripts total, 21 per-module deps/module/production scripts representing eight production modules (config and config/viper share three). Other scripts are documentation/version/tag gates, not duplicates. The exact manual target count is recorded in baseline evidence. No Go source, tests or consumers are changed by this wave.

User-approved clarification: default version preflight remains strictly ahead of the latest root tag. `--allow-current` exists only for post-tag verification in the disposable dry-run clone; the real workflow never uses it. Equal current version must have the complete expected tag set at HEAD.


## Catalog and daily verification

modules.tsv is the only maintained inventory. Columns are dir, path, kind, track,
version, checks and deps, separated by tabs. `deps` is the dependency boundary
(D41): `stdlib`, or the catalog dirs and external module paths the module may
import. Imports, graph and replaces are checked against it; there are no
per-module allowlist files. `modules.sh table` renders its live table;
`matrix` emits one JSON line decoded in the harness by Go encoding/json.

```sh
./scripts/modules.sh validate
./scripts/modules.sh lint-lists
./scripts/modules.sh table
make check-module M=config/viper MODE=quick
make check-all-modules
make all
```

The catalog has 19 rows: 10 coordinated modules, 8 unreleased production modules,
and the nonpublished example harness. Workspace membership is exactly the same
set. Registration checklist and all data formats/exceptions are in
[scripts/modules/README.md](scripts/modules/README.md). No Makefile, workflow or
script edit is needed to register an ordinary new module.

CI build/lint loops read modules.sh dirs; makefile CI uses catalog-driven Make.
CI prepares its cache with go mod download before offline generic verification.
Local offline checks require the same populated cache. No action/tool versions,
triggers or permissions were changed. version.yml's existing preflight gates
remain, with inventory/tag lists read from catalog/release-plan. It does not
publish independent modules.

## Tracks, approval and release planning

- coordinated: approved shared v0.30.0; sibling requirements must match the
  catalog. Strict preflight requires a version above the newest root release.
- independent: requires an explicit approved row/version. References to released
  siblings match their individual catalog versions. No publisher is added here.
- unreleased: version `-`, never selected for tagging. Sibling unreleased
  requirements may be v0.0.0 only with a correct relative replace. This is a
  valid development state, not an installable published release.
- none: nonpublished harness, version `-`.

Publishable tracks cannot require unreleased modules. Placeholders no longer
break make all merely because unrelated unreleased modules exist; they remain
forbidden for publishable requirements. Unknown in-repository sibling module
paths are rejected. External dependencies are unaffected by version syncing.

```sh
./scripts/check-module-versions.sh         # strict, before any tag
./scripts/release-plan.sh                  # print only coordinated tags
./scripts/release-plan.sh --track independent # print approved independent tags only
# After a separately approved coordinated version change:
./scripts/sync-module-versions.sh vX.Y.Z
```

sync updates only coordinated rows and requires from coordinated owners to
coordinated targets. It does not alter unreleased/independent/harness go.mod,
replace or exclude directives. It does not approve or publish anything.

The approved dry-run-only clarification `--allow-current` validates an already
created coordinated tag set at HEAD. The default remains strictly ahead. The
real workflow never uses this exception. All tagging dry runs occur in a
remote-free disposable clone, which is removed on exit. No tag is created,
moved or deleted in the real repository by these verification commands.

### Version proposals — not approved

These are review proposals, not catalog changes or authorization to tag:

| Module | Proposed track | Proposed version | Reason |
|---|---|---|---|
| concurrency | independent | v1.0.0 | v1 contract and real-tag validation gate exist; actual authorized tag evidence remains pending |
| config | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |
| client | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |
| retry | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |
| circuitbreaker | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |
| ratelimit | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |
| resilience | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |
| config/viper | independent | v0.1.0 | validated candidate; accumulate production usage before v1 |

All eight real catalog rows remain unreleased with `-`. Approval of versions,
resolvable dependency tags, compatibility reports and first real-tag consumers
remain separate work. No independent publication workflow was implemented.

## Executed evidence (wave 0)

| Evidence | Status | Proof |
|---|---|---|
| E1 catalog/rules/docs | passed | modules.tsv, modules.sh, generic checker, template and CLAUDE |
| E2 equivalence/mutation | passed on macOS | 8 modules × deps/consumers/quick, old and new; 40 mutation cases |
| E3 release dry run | passed | 10 coordinated tags only in disposable clone; strict/post-tag checks, negative promotion, independent matching and coordinated-only sync |
| E4 consistency | passed | validate/lint-lists, encoding/json parser, fictitious production module discovered by lint-arch/check-all-modules without code edits to automation |
| E5 public API compatibility | not applicable | repository automation only, no Go API changes |
| E6 version approval | pending user | proposal table only; all new modules remain unreleased |
| E7 authorized real tags | deferred | no workflow dispatch, real tags or publication in this task |

Workflow actionlint validation **passed** on macOS ARM64 with actionlint 1.7.12
and ShellCheck 0.11.0 installed through Homebrew. `actionlint -color=false`
validated all six workflows with exit code 0 and no diagnostics. Three unquoted
GITHUB_OUTPUT paths were quoted and one unused preflight variable was removed.
Evidence: release-evidence/wave0/actionlint.log and actionlint-version.log.
This is static validation; no hosted workflow, version.yml or release.yml was executed.

Final macOS ARM64 environment: Go1.27.4, Apple system Bash3.2.57, golangci-lint
2.14.0 already available externally. make all, make test and make example passed.
The 8 generic pipelines ran deps, consumers and quick (race count20 at default,
1 and 4 plus vet), and stress/fuzz/bench/all. Fuzz and benchmark smoke durations
were explicitly 1x/count3 for benchmarks; these validate automation and are not
new performance qualification claims. Concurrency's original expanded stress
counts/defaults were retained. Standalone consumers run GOWORK=off,
GOTOOLCHAIN=local, GOPROXY=off, GOFLAGS=-mod=readonly.

Linux ARM64: Bash5.2, Go1.27.4, catalog/list/matrix/unit/dry-run and all 8 generic
pipelines (deps, consumers, quick/stress/fuzz/bench/all, 1x smoke durations).
Full root make all/lint/fictitious-module demonstration and old/new equivalence
were executed on macOS only, not attributed to Linux or AMD64. The local Linux
container had network disabled, source mounted read-only and only evidence
output writable; temporary clone tags lived inside its disposable tmpfs.

Equivalence originals were captured from clean HEAD 476792c using git archive
into /private/tmp/wave0-original. To reproduce, extract that commit elsewhere and
set WAVE0_OLD_ROOT to it, then run test-modules.sh equivalence/mutation. All 24
old/new combinations pass. Every forbidden module dependency and forbidden
minimal/composed dependency was rejected by both. New detection improvements:
unused unexpected replaces (8 cases) and absent fuzz targets (8 cases) previously
returned success. Fuzz owner discovery now also prevents a target found in a
subpackage from silently running against the wrong package.

21 duplicate per-module scripts were removed only after equivalence and mutation
were green. check-concurrency-release.sh and test-concurrency-release.py remain
unchanged. Their Python dependency belongs to that existing published-tag gate,
not the new generic runtime. Existing historical module evidence referencing old
scripts remains historical; use this process for current verification.

Preserved exceptions: config/viper shares config's minimal/composed consumers
(minimal validates core config, composed exercises viper); CONFIG legacy env
fallback; existing _STRESS names as the default stress variable (D41), set beside
canonical NAMESTRESS;
config parser import constraints and client/resilience inverse-import boundaries
(both implied by the `deps` column since D41);
concurrency expanded stress hook; root configcomparison test/benchmark aliases.
The old 43 manual check targets are replaced by generic targets and pattern
aliases, deprecated for this release. No Go source/tests/consumers, go.mod,
go.sum, or declared v0.30.0 were changed.

Two in-progress shell processes were invalidated when their script files were
edited during execution (unexpected EOF). They were not accepted as evidence;
the affected harness and generic pipelines were rerun against fixed scripts.
Final syntax checks and accepted logs are from those reruns.

### Commands and artifacts

```sh
export PATH=/private/tmp/go-engine-toolchain/go/bin:/private/tmp/resilience-tools/bin:$PATH
export GOCACHE=/private/tmp/go-engine-build-cache GOMODCACHE=/Users/seven/go/pkg/mod
export GOPROXY=off GOLANGCI_LINT_CACHE=/private/tmp/go-engine-lint-cache
./scripts/test-modules.sh unit
./scripts/test-modules.sh equivalence
./scripts/test-modules.sh mutation
./scripts/test-modules.sh dry-run
./scripts/test-modules.sh fixture
make all
make test
make example
# Each production row, with <PREFIX> uppercase name without '-':
# env <PREFIX>_FUZZ_TIME=1x <PREFIX>_BENCH_TIME=1x \
#     <PREFIX>_RESULTS_DIR=<output> ./scripts/check-module.sh <dir> all
```

Full logs: release-evidence/wave0/. `lint-lists-initial.log` records C2's expected
initial failure against duplicated inventories. Negative-test log failures are
expected rejections, not omitted checks. The dry-run plan is release-plan.log.
Source/evidence manifests exclude their own recursive hash and are verified with
shasum -a 256 -c from the indicated directory. Tool cache paths are execution
environment, not runtime requirements.

Linux image sha256:e432b43af23a9328d56a7c499be0476810aa344acbcf65fc7c455d4ff5a40602;
4 CPUs/4GiB, /tmp tmpfs3GiB exec, --network none/--read-only. Go/cache were supplied
through stdin from the existing config-linux-validation.tar.gz; Linux Go1.27.4
archive SHA256 3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92.
Git safe.directory=/src was set for the read-only mount; only temporary clones
were tagged. No remote push, real tag mutation, commit or staging was performed.

Current modules.tsv SHA256: `dd5dcac3fc3fe6671cb99fbd3ee5646f5dad3b86b02192b88e05084997d6e1d3`.
Initial commit: `476792c`. Source hashes: release-evidence/wave0/SOURCE.sha256;
evidence hashes: release-evidence/wave0/SHA256SUMS. Retired-script names and baseline
counts are preserved alongside the logs.

Final working-tree observation: 175 staged deletions of historical module evidence
and an unstaged .gitignore change appeared after the initial clean preflight.
They are external to this task and were neither modified nor restored. New wave-0
evidence is retained locally under release-evidence/wave0 (currently ignored).

## Evidencias D43/D44

Initial branch feature/split, HEAD93b8e86f3b59119a4e91bed5cd313ef2cdbe3fa3; git status --porcelain empty. Go1.27.4 darwin/arm64. D43.1–D43.5 and D44 are approved by architect and user, superseding concrete tool dependencies, the logging clock and the planned Logrus bridge. No existing changes are mixed into this task.

C1 confirmed: resilience imports three concrete tools in source/tests; circuitbreaker exposes only generic Execute; root pkg/engine/config_modular_test.go imports unreleased config modules under configcomparison and calls private newDecoder; docs/ ignores four files retained locally and present in origin/master (4f3d3e9); client/logging mixes injected now with time.Since. No new code may import legacy production code except the explicitly nonpublished config comparison harness. No production legacy source is changed.

Increment results, commands, platforms, mutation tests, move map and hashes are recorded below. No commit, push, tags, version/track approvals or release workflow dispatch.


### D43/D44 incremental results

| Step | Result | Evidence |
|---|---|---|
| C1 | clean preflight; all six findings confirmed | initial state above |
| C2 | single-clock fake7s regression passes; four master documents restored byte-for-byte | c2-clock.log, restored-master.sha256, docs-status.txt |
| C3 | Run/Execute share one D37 core; original circuitbreaker assertions untouched | c3-run.log, circuitbreaker/race-*.log |
| C4 | resilience depends only on client/retry; local foreign implementations plus real-tool harness pass | c4-interfaces.log, c4-real-harness.log, c4-consumers.log, test-move-map.md |
| C5 | config comparison uses public RawConfig.Decode outside root; root tidy unchanged; tagged-source guard enforced | c5-comparison.log, harnesses.log, root-tidy-final.log, c5-source-policy.log |
| C6 | slog standard documented; Logrus catalog and graph bans enforced | policy-final.log, policy-final/d44-*.log |
| C7 | full macOS regression and scoped Linux verification pass | make-all.log, make-test.log, make-check-module.log, linux-arm64/execution.log |

Resilience integration moved every original Test/Fuzz/Benchmark function (28 total) and its one checked-in fuzz corpus file. The per-function location table is versioned in resilience/RELEASE_NOTES.md; integration/README.md maps files. No original case is discarded. Namespace/import changes and concrete Stats/Allow observations are confined to the harness. D43 deliberately changes local rejection identity: affected assertions now require errors.Is for BOTH ErrRejected and the original ErrOpen/ErrProbeLimit, while operation error identity assertions remain strict. The composed consumer changes only that rejection assertion and gains compile-time interface assignments. Minimal loses stale concrete-tool go.mod requires/replaces and its graph-description message changes; it now selects three library modules, not six.

Config comparison retains both original TestModularConfig functions and BenchmarkConfigDecoderComparison. Private newDecoder is replaced by exported engine.NewRawConfig(...).Decode; no new engine export or legacy production change. The public decoder includes metadata validation, so the benchmark now explicitly compares public APIs rather than claiming identical private-decoder overhead. No configcomparison build tag or workspace is needed. Existing check-config-engine/bench-config-engine hooks delegate to this harness with GOWORK=off; check-config-engine passes.

The generic source-policy helper is stdlib Go (Go was already a repository prerequisite), reads catalog data and parses Go imports irrespective of build tags. It skips nested modules as separate boundaries and does not mistake comments/string literals for imports. No module inventory is duplicated. The root catalog dep `.` resolves to its actual module path. Basic harness dependency checks do not require production fuzz targets. No Makefile or workflow change was needed for the two harnesses.

### D43/D44 executed commands and scope

Toolchain Go1.27.4. macOS ARM64 with system Bash3.2 and existing golangci-lint2.14.0. PATH uses /private/tmp/go-engine-toolchain/go/bin and /private/tmp/resilience-tools/bin; GOCACHE=/private/tmp/go-engine-build-cache, GOMODCACHE=/Users/seven/go/pkg/mod, GOPROXY=off. No dependency/directive/version/track of an existing module is upgraded; resilience's declared dependencies are intentionally reduced.

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
WAVE0_RESULTS_DIR="$PWD/release-evidence/d43-d44/policy-final" scripts/test-modules.sh decisions
WAVE0_RESULTS_DIR="$PWD/release-evidence/d43-d44/unit" scripts/test-modules.sh unit
# For each affected module: circuitbreaker, resilience, client/logging, logging
make check-module M=<dir> MODE=deps
make check-module M=<dir> MODE=consumers
make check-module M=<dir> MODE=quick
# Stress/bench for circuitbreaker, resilience and resilience/integration:
# <PREFIX>_RESULTS_DIR=... scripts/check-module.sh <dir> stress
# <PREFIX>_BENCH_TIME=100ms ... scripts/check-module.sh <dir> bench
(cd resilience/integration && GOWORK=off go test -race -count=20 ./...)
(cd harness/config-engine-comparison && GOWORK=off go test -race -count=20 ./...)
(cd harness/config-engine-comparison && GOWORK=off go test -run '^$' -bench . -benchtime=100ms -benchmem ./...)
GOWORK=off GOFLAGS=-mod=mod go mod tidy
make check-config-engine
make all
make test
```

All above pass. Generic quick is race count20/default GOMAXPROCS/1/4 plus vet for all four affected modules. Stress is count3; benchmark duration100ms/count3, no race. Real-tool harness stress retains its original32,000-call fixture and other lifecycle/retention cases; the interface-only module adds12,000 calls through foreign implementations. Additional fuzz campaigns ran2s per target in both resilience and its real harness, preserving the original seeds/model as well as testing the new interface boundary. These are short correction campaigns, not new performance/SLA qualifications. make all covers all18 actual standalone modules; standalone-coverage.txt verifies exact set equality against catalog-derived consumers, including the config/viper override. Their frameworks/SDK graphs remain bounded by their declared catalog contracts.

Root GOWORK=off tidy was run twice. go.mod and go.sum are byte-identical to initial HEAD93b8e86, not merely semantically unchanged. The only pkg/ change is removal of the test moved to the comparison harness. No legacy production code, family, preset, release workflow, version track, tag or index entry is modified.

Linux ARM64 verification used the existing local image e432b43af23a, four CPUs/4GiB, network disabled and source mounted read-only. The existing Go1.27.4 archive was injected into tmpfs; only result output was writable. Linux ran catalog/source/list rules, deps/consumers/quick (race20/default/1/4 + vet) for all four modules, real resilience harness race count20 and real harness stress count1. The config/engine harness, root tidy, root full regression/lint, policy mutation snapshots, benchmarks and short fuzz campaigns ran on macOS only; they are not attributed to Linux. No AMD64 validation is claimed.

The two requested policy mutations are performed only in a disposable current-tree snapshot: a coordinated root test importing config behind a build tag fails for the tagged source import; a client unreleased row declaring Logrus fails for that dependency. An additional graph mutation independently fails when Logrus is selected even without a corresponding direct import. Each case checks the intended diagnostic, not just a nonzero exit. An initial graph test failed too early on a missing offline transitive module; it was not accepted. The final fixture pins the already-resolved root x/sys version in the disposable copy and passes the intended graph-ban assertion. Nothing is added to the real module or cache to fake a published version.

An initial make all run correctly caught obsolete concrete-tool requirements in resilience's minimal consumer after the dependency reduction. Those stale requires/replaces and its six-module informational description were corrected; the final make all and explicit make test pass. No failure is silently treated as success.

### Restored documents and newly visible local files

.gitignore now ignores only /docs/modularization/ and /docs/modularization.md instead of all docs/. These four files match origin/master4f3d3e9 byte-for-byte: docs/adr/0001-core-provider-decoupling.md, docs/migration-engine.md, docs/plan-auditoria-2026-08.md, docs/plan-mejoras-go-engine.md. They were restored to the working tree from git object contents without staging. Their contents are not edited. The local modularization plan/answers were updated under the remaining ignore rules to record D43/D44 and remove logging/logrus as a target.

The following35 pre-existing local files become visible because the broad ignore was removed. They are NOT part of this task's additions, have not been staged/added or modified, and are left for the user's decision:

```text
docs/modules/concurrency-candidate.sha256
docs/modules/concurrency-evidence/SHA256SUMS
docs/modules/concurrency-evidence/execution-context.txt
docs/modules/concurrency-production.md
docs/modules/concurrency-release-fixture/composed-download.json
docs/modules/concurrency-release-fixture/composed.go.mod
docs/modules/concurrency-release-fixture/composed.go.sum
docs/modules/concurrency-release-fixture/harness/reject-corrupt-artifact/minimal-download.json
docs/modules/concurrency-release-fixture/harness/valid/composed-download.json
docs/modules/concurrency-release-fixture/harness/valid/composed.go.mod
docs/modules/concurrency-release-fixture/harness/valid/composed.go.sum
docs/modules/concurrency-release-fixture/harness/valid/minimal-download.json
docs/modules/concurrency-release-fixture/harness/valid/minimal.go.mod
docs/modules/concurrency-release-fixture/harness/valid/minimal.go.sum
docs/modules/concurrency-release-fixture/minimal-download.json
docs/modules/concurrency-release-fixture/minimal.go.mod
docs/modules/concurrency-release-fixture/minimal.go.sum
docs/modules/concurrency-release-readiness.md
docs/modules/concurrency-validation.md
docs/modules/concurrency.md
docs/modules/config-candidate.sha256
docs/modules/config-evidence/SHA256SUMS
docs/modules/config-evidence/execution-context.txt
docs/modules/config-revised-candidate.sha256
docs/modules/config-revised-evidence/SHA256SUMS
docs/modules/config-revised-evidence/core/package-module-footprint.txt
docs/modules/config-revised-evidence/execution-context.txt
docs/modules/config-revised-evidence/mapstructure-compose-doc.txt
docs/modules/config-revised-evidence/mapstructure-options-doc.txt
docs/modules/config-revised-evidence/mapstructure-value-hook-doc.txt
docs/modules/config-revised-evidence/viper/package-module-footprint.txt
docs/modules/config-revised-validation.md
docs/modules/config-validation.md
docs/modules/config-viper-candidate.sha256
docs/modules/config.md
```

### D43/D44 artifacts and remaining gates

Source hashes: release-evidence/d43-d44/SOURCE.sha256, relative to repository root. Evidence hashes: release-evidence/d43-d44/SHA256SUMS, relative to that evidence directory. Deleted-path list and exact restored-master hashes are alongside the logs. These ignored logs are local artifacts; this versioned report and module release notes preserve the approved decisions, commands, scopes, move map and limitations. Earlier module candidate hash manifests describe their earlier snapshots; this correction has its own manifest.

All requested implementation and applicable local checks are complete. No task-specific blocker remains. Existing version approval/publication/gorelease/tag-consumer gates are not claimed or changed by these corrections. No commit, push, tag creation or release workflow execution.

## D46.6 — composed application dependencies

The user approved scripts/modules/consumer-deps.tsv for exact external module paths used directly by composed applications. modules.tsv continues to define each library's own boundary; this supplementary application data cannot widen the library/minimal permissions or admit repository modules, wildcard paths or Logrus. The reader validates both directions against consumer go.mod and source/test imports, including unused/missing entries and missing directories. See scripts/modules/README.md for the schema and commands. client/otel composed uses this for SDK capture/ManualReader assembly while the wrapper and minimal remain API-only.

`scripts/test-modules.sh consumer-deps` passed the positive baseline and12 rejection cases in a disposable current-worktree snapshot: wrapper/minimal SDK, undeclared SDK composed, unused/Logrus/minimal/module/wildcard/repository/unknown-owner/missing-require/missing-directory entries. Existing unit/matrix and D43/D44 decision mutations also passed. This change does not approve versions, tracks, tags or release workflow changes; detailed module evidence is in client/otel/RELEASE_NOTES.md.

## Wave 2 closed (L1–L5)

Wave 2 is complete with lot L5. Its modules are implemented, registered in `modules.tsv` and `go.work`, covered by independent consumers and verified locally; none is released. Every row is `unreleased` with version `-`, and each module's proposed v0.1.0 awaits explicit approval (E6). Consumers against real tags (E7) are deferred until tags exist.

| Lot | Decision | Modules |
|---|---|---|
| L1 | D49 | `router/errors` |
| L2 | D51 | `router`, `router/chi`, `router/operations` |
| L3 | D52 | `auth/jwt`, `auth/jwt/http` |
| L4 | D53 | `httpclient`, `httpclient/resty` |
| L5 | D54, D55 | `validation`, `config/yaml` |

Each module's `RELEASE_NOTES.md` holds its approved contract, internal decisions and executed evidence. Closing the wave approves no version, creates no tag and dispatches no workflow. Three distribution impediments are recorded and remain open until the module each depends on is published: gorelease cannot resolve the unpublished sibling of `auth/jwt/http` (`auth/jwt`), of `httpclient/resty` (`httpclient`) and of `config/yaml` (`config`). The legacy packages these modules replace (`pkg/router`, `http/pkg/rest`, `pkg/utilities/validation` and the Viper-based YAML loading) are unchanged and supported; no consumer has been migrated.

## Auditoría de paridad (L6)

Auditoría del código heredado frente a los módulos nuevos, previa a la limpieza (L7). Es un documento de trabajo local; aquí solo queda su resumen. No cambia código, catálogo de módulos publicables ni versiones. Su único añadido versionado es el harness `harness/legacy-inventory` (kind harness, track none), que inventaría los identificadores exportados de los módulos coordinated y de `example` y comprueba que la matriz los cubre todos.

Alcance: 67 paquetes y 1.071 identificadores exportados, más 59 capacidades que no son identificadores (comportamientos del engine, claves YAML de los providers y comportamientos fijados por tests). La comprobación de exhaustividad pasa.

| Estado | Identificadores | Capacidades |
|---|---:|---:|
| MIGRADO | 9 | 1 |
| MIGRADO CON RUPTURA | 208 | 20 |
| PLANIFICADO | 757 | 33 |
| HUECO | 1 | 1 |
| DESCARTE PROPUESTO | 96 | 4 |

La ola 2 cubre el núcleo transversal; los adaptadores de servicios (AWS, mensajería, gRPC, NoSQL, SQL) y el builder siguen pendientes. Los dos huecos sin destino son las instancias con nombre de la configuración YAML heredada y la instrumentación HTTP de servidor. Solo 84 de las 208 filas con ruptura tienen su identificador nombrado en la guía de migración enlazada; las demás están documentadas a nivel de paquete.

Lotes restantes propuestos, pendientes de aprobación:

| Lote | Ola | Contenido |
|---|---|---|
| L8 | 3 | `presets/httpapi` y ejemplo HTTP sin engine |
| L9 | 3b | Exportadores OTLP y middleware HTTP de servidor |
| L10 | 3b | Instancias con nombre en la configuración; MIGRATION de `concurrency` |
| L10b | 3 | Tablas por identificador en las guías de migración de la ola 2 |
| L11 | 4 | `database/atomic`, `database/postgres`, `database/mysql` |
| L12 | 4 | `database/gorm` y dialectores |
| L13 | 5 | `idempotency`, `outbox`, `inbox` y sus stores |
| L14 | 3b | `database/redis`, `database/memcached`, `database/mongodb` |
| L15 | 3b | `messaging/kafka`, `messaging/rabbitmq` |
| L16 | 3b | `rpc/grpc` |
| L17a / L17b | 3b | `aws/config` y clientes por servicio |
| L18 | 3b | Fachada `integration/cloud` e `integration/aws`, o su descarte |
| L19 | 6 | Testkits de adaptadores |
| L20 | 7 | `engine`, `engine/providers/*`, presets y ejemplo ensamblado |
| L7 | limpieza | Retirada del código heredado, al final |

L7 solo puede empezar cuando toda fila sea MIGRADO, MIGRADO CON RUPTURA (con su identificador en la guía) o DESCARTE APROBADO y el usuario haya creado el tag final heredado. El catálogo declara v0.30.0 coordinated, pero ese tag todavía no existe; el último publicado es v0.22.0.

## D56 — decisiones aprobadas para el cierre de L6

Registradas por instrucción del usuario el 2026-11-17. Las once decisiones están aprobadas; no son pendientes ni prueba de implementación/publicación. Esta sección prevalece sobre las propuestas históricas L6.

| Decisión | Resolución aprobada |
|---|---|
| D56.1 | Descartar todos los grupos propuestos por L6: helpers reemplazados por stdlib; checkers concretos de health y helpers OTel; logrusadapter (D44), task_executor→concurrency; pkg/testutil y http/pkg/testutil; EnableLogging/WithResilience; conmutador/no-op del engine. Métodos envoltorio AWS (s3/sqs/sns/ses/ssm/cognito/dynamo), Redis/Memcached/MongoDB: **descarte aprobado: usar cliente nativo** (D07). Adaptadores conservan constructor, Config validada, health, acceso nativo y valor propio (consumo SQS con visibilidad/ack, presign S3, consumidores Kafka/RabbitMQ, cierre gRPC acotado). Descartar pkg/integration/cloud, pkg/integration/observability e integration/aws/*; L18 retirado, sin consumidores reales de la abstracción multi-cloud. |
| D56.2 | Builder nuevo con engine.Stack LIFO explícita; providers son funciones sin estado; variables Go sustituyen Get[T]/ComponentNames. Sin lifecycle compartido (D26). Implementación L20 sigue siendo requisito. |
| D56.3 | Tras L7 raíz sin Go/go.mod; engine nuevo en github.com/skolldire/go-engine/engine. go get del path raíz mantiene la última versión heredada. No se elimina ahora la raíz. |
| D56.4 | Último release heredado v0.30.0 para los10 tags de release-plan.sh. Usuario los crea/publica con version.yml desde master sin módulos nuevos. feature/split debe aceptar esa versión ya publicada sin desactivar el preflight de versiones futuras; ajuste pendiente antes del retiro, sin crear tags aquí. |
| D56.5 | Versiones nuevas aprobadas: v0.1.0 para todos los módulos nuevos y v1.0.0 para concurrency. Publicación L21 en orden de dependencias, bases client/retry/config/httpclient/auth/jwt primero. La aprobación E6 no prueba publicación/E7; no se promociona el catálogo ni se reescriben requires/publica nada en este preflight bloqueado. |
| D56.6 | Ola3b aprobada para L9–L17. No equivale a lotes implementados/commiteados. |
| D56.7 | Instrumentación HTTP de servidor mediante receta de otelhttp del ecosistema, sin módulo propio. pkg/telemetry/otel.NewMiddleware se clasifica MIGRADO CON RUPTURA y enlaza guía que nombra ese identificador. |
| D56.8 | Instancias con nombre como mapa: redis: {instances: {cache: ...}}; herramienta de conversión de archivos heredados. Sin fuente de compatibilidad para listas. Implementación/conversor L10 pendiente, se reclasifica el antiguo HUECO como PLANIFICADO. |
| D56.9 | D33: MariaDB en database/mysql, Flavor(mysql\|mariadb), default mysql y Capabilities(flavor). SQLite en database/sqlite con modernc.org/sqlite sin CGO; TxLock=immediate, Pool.MaxOpen=1 cuando ReadOnly=false; segundo pool ReadOnly con TxLock=deferred para lecturas paralelas. Implementación pendiente. |
| D56.10 | AWS migra solo operaciones con uso demostrado más semántica propia D56.1; resto por cliente nativo. Descartar wrappers no afirma que sus consumidores ya estén migrados. |
| D56.11 | Quitar testkit/logging, testkit/context y testkit/httpclient/resty de ola6. Se conservan los testkits de adaptadores que queden en L19. |


## Limpieza (L7)

Preflight 2026-11-17, rama feature/split, HEAD e57ebf2b8d32399c3097b1462442196fe06c747e. Estado inicial `git status --porcelain` vacío, antes de registrar D56. Este preflight solo documenta decisiones/reclasificación y reconoce DESCARTE APROBADO en el harness; no autoriza borrar nada ni afirma cierre de lotes/publicación. Versiones aprobadas D56.5 pendientes de promoción/publicación por L21, sin cambiar catálogo/requires para fingir ese requisito.

Conteo vigente D56 (identificadores/capacidades):

| Estado | Identificadores | Capacidades | Total |
|---|---:|---:|---:|
| MIGRADO | 9 | 1 | 10 |
| MIGRADO CON RUPTURA | 207 | 20 | 227 |
| PLANIFICADO | 557 | 31 | 588 |
| HUECO | 0 | 0 | 0 |
| DESCARTE PROPUESTO | 0 | 0 | 0 |
| DESCARTE APROBADO | 298 | 7 | 305 |
| **Total** | **1071** | **59** | **1130** |

Comprobaciones de entrada y resultado final se registran a continuación. Las secciones L6 anteriores conservan la instantánea histórica, no decisiones todavía abiertas.

### Resultado de las condiciones de entrada

**L7 BLOQUEADO. No se borró ningún archivo/directorio y no se preparó ningún commit de retirada.** Aprobar D56 no es demostrar migraciones implementadas ni publicar versiones.

| Condición obligatoria | Resultado observado | Evidencia / acción |
|---|---|---|
| Matriz sin PLANIFICADO/HUECO/DESCARTE PROPUESTO, check de inventario y guías por identificador | **NO SE CUMPLE** | Tras D56 quedan557 identificadores+31 capacidades PLANIFICADO (588 filas). Hay0 HUECO/0 DESCARTE PROPUESTO,305 descartes aprobados. Se conservan1.071 identificadores únicos+59 capacidades; eso no sustituye el inventario Go. La revisión exhaustiva de guías no se declara aprobada. |
| L8/L9/L10/L10b/L10c/L11–L17/L19/L20/L21 cerrados y commiteados | **NO ACREDITADO / L20 ausente** | No existe engine/go.mod ni fila engine; tampoco presets/httpapi, database/atomic/mysql/sqlite, messaging/kafka, rpc/grpc, aws/config, outbox/inbox. La matriz conserva sus implementaciones como pendientes. HEAD es el cierre de auditoría L6, no prueba de L20/L21. No se afirma auditar cada commit de todos los lotes tras fallar la primera condición. |
| Diez tags heredados remotos v0.30.0 | **NO VERIFICADO por parada de preflight** | No se ejecutó git ls-remote --tags tras detectar el bloqueo. Antes de un nuevo intento es obligatorio ejecutar `git ls-remote --tags origin` y cotejar exactamente el plan (incluyendo refs anotadas). No se da por existente ni ausente un tag remoto por la instantánea histórica L6. |
| Publicación D56.5/L21 y consumidores contra tags reales E7 | **NO ACREDITADO** | Catálogo actual:10 coordinated,24 unreleased,5 none,0 independent. D56.5 aprueba versiones pero no demuestra publicación. No se ejecutan consumidores E7 ni se cambian tracks/requires para aparentarla. |
| CI Linux de feature/split verde | **NO VERIFICADO por parada de preflight** | Ningún workflow consultado/ejecutado/dispatchado en esta comprobación. Se necesita resultado verde asociado al commit que vaya a limpiarse. No se atribuye resultado Linux. |
| git status limpio y make all antes de retirar | **PARCIAL: limpio al entrar; make all NO EJECUTADO** | El estado inicial fue vacío antes de los cambios obligatorios de documentación D56. No se considera ese trabajo preparatorio como suciedad preexistente. make all se omite al fallar el gate1; no se declara aprobado. |

Comando de inventario realmente intentado (sin cambiar el SDK ni dependencias):

```sh
(cd harness/legacy-inventory && GOWORK=off /private/tmp/go-engine-toolchain/go/bin/go run . check ../.. ../../docs/modularization/parity.md)
```

Exit1 de compilación: `package bufio is not in std`, igual para fmt/go/parser/io/os y otros. El binario responde `go version go1.27.4 darwin/arm64`, pero el GOROOT temporal contiene directorios de la biblioteca estándar vacíos (p.ej. src/fmt); no hay `go` alternativo en PATH. El harness **no se ejecutó**, no es un fallo de exhaustividad ni un check aprobado. Log local `/private/tmp/go-engine-l7-preflight/inventory-check.log`; no es entregable versionado. No se instala/restaura una toolchain como parte de un retiro ya bloqueado.

Checks estáticos realmente ejecutados: conteo1130 filas,1071 identificadores únicos/59 capacidades, sin duplicados; conteo por estado; `git diff --check` pasa. El harness acepta ahora DESCARTE APROBADO además de los estados históricos; no se cambió la comparación bidireccional ni se elimina PLANIFICADO para hacer pasar L7. La receta D56.7 se contrastó con el código ya instalado otelhttp v0.72.0 (NewHandler/WithTracerProvider/WithMeterProvider); no se añadió require ni se actualizó dependencia.

Cambios preparatorios limitados a RELEASING.md, matriz local ignorada docs/modularization/parity.md, harness/legacy-inventory (estado/README) y receta versionable telemetry/otel/MIGRATION.md. El descarte de wrappers mantiene pendientes los constructores/config/clientes nativos/health/lifecycle y semánticas propias que aún no están implementados; los consumidores heredados no se migraron por reclasificar. La regla D56.4 de aceptar el release final ya publicado queda registrada pero su ajuste de código se difiere al gate con los tags realmente acreditados; no se rebaja hoy el preflight de versiones futuras.

Lista de commits preparados: **ninguno de retirada**. No se toca example/presets/familias/providers/pkg/harness de comparación, go.mod/go.sum raíz, go.work, modules.tsv ni workflows. Sin git add/reset/checkout/commit/push/tag, sin reescritura de historial. No archivos con " 2" tocados. Antes de L7 deben cerrarse los lotes y588 filas pendientes, documentar las rupturas, restaurar una toolchain completa y acreditar tags/E7/CI/make all. La aprobación de las11 decisiones ya está registrada y no se vuelve a pedir.

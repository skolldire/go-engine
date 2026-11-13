# Releases and repository modules

`modules.tsv` is the module inventory, verification and approved-version source of truth (D29/D40 approved). No independent version is approved by this iteration. All new tools stay unreleased until explicit user approval.

## Evidencias ola 0

Initial branch feature/split, HEAD 476792c. `git status --porcelain` empty. Go go1.27.4 darwin/arm64; system bash 3.2.57. Existing source/tests/consumers and declared v0.30.0 will remain unchanged.

Confirmed initial findings: CI's explicit lists omit config/config-viper/client/retry/circuitbreaker/ratelimit/resilience; they already include concurrency. version.yml's coordinated loops incorrectly include concurrency. Existing sibling v0.0.0 replaces cannot publish; the old checker mixes all tracks and rejects the existing tree. Version scripts assume one coordinated track. MODULES/workflows/workspace duplicate inventories; CLAUDE describes only the older families.

Count clarification: 24 check-* scripts total, 21 per-module deps/module/production scripts representing eight production modules (config and config/viper share three). Other scripts are documentation/version/tag gates, not duplicates. The exact manual target count is recorded in baseline evidence. No Go source, tests or consumers are changed by this wave.

User-approved clarification: default version preflight remains strictly ahead of the latest root tag. `--allow-current` exists only for post-tag verification in the disposable dry-run clone; the real workflow never uses it. Equal current version must have the complete expected tag set at HEAD.


## Catalog and daily verification

modules.tsv is the only maintained inventory. Columns are dir, path, kind, track,
version and checks, separated by tabs. `modules.sh table` renders its live table;
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

Workflow actionlint validation is **pending**: actionlint is not installed. It
was not installed/upgraded or claimed as passed. Catalog/list/JSON tests do not
substitute for actionlint or actual hosted GitHub Actions execution. No version.yml
or release.yml was executed.

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
fallback; existing _STRESS names declared as data beside canonical NAMESTRESS;
config parser import constraints; client/resilience inverse-import boundaries;
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

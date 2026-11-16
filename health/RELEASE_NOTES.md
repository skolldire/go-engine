# health candidate — wave 1

## Initial state before code

Branch feature/split, HEAD3d317bdc849be1b43576bc4373e80bbd736e596e, Go1.27.4 darwin/arm64. Initial git status --porcelain:

```
 M CHANGELOG.md
 M MIGRATION.md
 M RELEASING.md
 M go.work
 M modules.tsv
 M scripts/check-module.sh
 M scripts/modules.sh
 M scripts/modules/README.md
 M scripts/test-modules.sh
?? client/otel/
?? examples/standalone/client/otel/
?? examples/standalone/telemetry/
?? scripts/modules/consumer-deps.tsv
?? scripts/modules/consumerdeps/
?? telemetry/
```

These are prior D45/D46 telemetry/client instrumentation and generic-checker changes. Preserve them and attribute none to this task. Shared catalog/workspace/changelog/migration additions remain separate. No index change, commit, push, tag, release workflow, legacy production edit, existing Go directive/dependency update or file containing " 2" is authorized. Use the existing catalog and generic checks. Version proposals remain unapproved, tracks unreleased/-.

## H1 — D47 approved contract and inventory before implementation

Stdlib-only module, Config{Timeout,MaxParallel,MaxChecks} positive in declaration order; defaults5s/4/128; ConfigError wraps ErrInvalidConfig. Checker/CheckerFunc; Registry New/Register/Live/Ready/Check; Status up/down and Reason ok/failed/timeout/busy/panic. Report{Live,Ready,Checks}, CheckResult{Name,Status,Reason,Duration,Err json:"-"}. Ordered independent report slices; Err never copied to a serializable text field. No logger, driver, HTTP, metrics, context replacement, background polling or per-poll abandoned wrappers. Per-check single live execution and global MaxParallel limit persist across all evaluations; stuck checks remain busy, release only when actually finished, and late results are discarded. Register freezes at first Ready/Check; duplicate/name/count/nil checks are explicit. Ready alone returns ErrNotReady for a down result; Check returns diagnostic Report without treating dependency failure as invocation error. ctx nil ErrNilContext. Panic recovery is allowed only in owned checker goroutines; Goexit detected with completion flag. ErrPanic is required by D47.F although omitted from B's shorter sentinel list.

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| pkg/health/entity.go Config + logger.Service | independent Config/Registry | adapt, remove logger and zero normalization | literal/default/config validation |
| service.go Register append with no validation | Register bounded unique frozen registry | develop | names/typed nil/duplicates/limits/concurrent freeze |
| service.go GetStatus one goroutine/check/poll | persistent live-slot/check accounting | replace mechanism in new module only | gated stuck check, no growth, release/recovery |
| DependencyStatus.Error=err.Error | CheckResult.Err json:"-" + fixed Reason | deliberate privacy break | error identity, JSON contains no secret |
| IsReady/IsLive/GetStatus | Ready/Live/Check | adapt readiness error vs diagnostic | no checks/up/down/cancel/timeout |
| checkers.go Redis/SQL/HTTP concrete checkers | caller CheckerFunc / adapter modules | exclude | README recipes only, no driver imports |
| handler.go routes/HealthResponse | future router/operations | exclude | no net/http in production module |
| pkg/engine + preset/full + example | later migration | preserve legacy | root regression |
| messaging/pkg/integration/kafka/checker.go | adapter-owned checker | preserve legacy | root regression |

Verified logger dependency, zero timeout→5s normalization, duplicate/unbounded registration, per-query goroutine launch without surviving-execution tracking, err.Error copying and core HTTP/adapter mixing in current source. Source inventory is descriptive, not evidence of target behavior. The additional public PanicError shape was proposed before health code and then approved as recorded below. H2–H7 implemented; executed E1–E5 evidence follows. E6 proposed independent v0.1.0 unapproved; E7 real tags deferred.

## Approved PanicError clarification before implementation

User approved the exact health-local shape: PanicError{Value any; Stack []byte; Goexit bool}. Error returns the fixed ErrPanic text without formatting Value/Stack; Is recognizes ErrPanic. Capture the current checker goroutine's stack with runtime.Stack. Panic preserves the recovered value and Goexit=false; runtime.Goexit has Value=nil and Goexit=true. Report.Err remains json:"-"; JSON tests must exclude both Value and Stack and prove errors.Is/errors.As in both cases. This follows concurrency's Value/Stack/fixed-error convention without importing concurrency; the additional explicit Goexit marker is approved here. Stack capture uses a64KiB current-goroutine buffer like concurrency and can be truncated at that internal bound. ErrPanic joins the public sentinel list required by D47.F.

Resume state: HEADd0ff735 (external commit of prior client/otel work); utilities D48 and its shared-file additions are in progress/uncommitted and remain separate. No agent commit occurred. Original initial-state evidence above remains historical. Health starts implementation only after this clarification.

Evaluation mechanics: first Ready/Check freezes registration, including a nil-context invocation; nil returns ErrNilContext before evaluating. A common caller-derived Timeout bounds the evaluation, including slot admission; each admitted callback receives a child context of that bound, canceled after it finishes. Admission/entry liveness and the global live count are synchronized; only checker execution gets an owned goroutine. Caller goroutines wait for slots. Busy entries are observed without spawning a replacement. Durations include admission until completion/timeout (busy is immediate zero). Errors returned before timeout are retained unchanged; if a timeout is observed before a result exists, Err is the effective context error. Callback results after deadline cannot become fresh cached state. No result/history lives in Registry.

## H2–H7 executed evidence and reproduction

Health continuation validated with Go1.27.4 on darwin/arm64 (Apple M1 Max) and linux/arm64 (local disposable container,4 CPUs/4GiB, network disabled). No AMD64 validation is claimed. Root regression/lint/catalog/related-consumer checks ran on macOS only. No existing Go directive, dependency version, legacy production source/test or workflow was changed. The library has no require/go.sum; go list-m lists only its own module and the test build has132 stdlib dependency packages.

H2 Config/errors/registration compiled and passed before H3. H3 evaluation/context/ordered report/slot timeouts passed before adding H4 recovery. H4 normal/abnormal finalization and persistent-slot tests passed; H5 stress/fuzz/bench then H6 independent consumers/guards and H7 documentation followed. Two initial lint findings (intentional nil-context contract test and expression spelling) and one test-only missing return after runtime.Goexit were fixed before accepted campaigns. Failed attempts are not counted as approved validation.

Offline environment: GOTOOLCHAIN=local, GOPROXY=off and a prepared toolchain/cache. Module/consumer commands use GOWORK=off GOFLAGS=-mod=readonly; the generic checker sets them itself. Commands executed from root:

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
make check-module M=health MODE=deps
make check-module M=health MODE=consumers
make check-module M=health MODE=quick
make check-module M=health MODE=stress
HEALTH_FUZZ_TIME=3s make check-module M=health MODE=fuzz
HEALTH_BENCH_TIME=100ms make check-module M=health MODE=bench
make lint
make lint-arch lint-docs check-modules
make test
scripts/check-module.sh config consumers
scripts/check-module.sh config/viper consumers
scripts/check-module.sh logging consumers
make check-module M=utilities MODE=deps
make check-module M=utilities MODE=consumers
```

All listed commands passed on macOS, using golangci-lint2.14.0. Root make test includes legacy health, profile/helpers, engine, families and every catalog module, including both new modules. Its existing local test listeners needed sandbox permission; no service/network is used by the new suites. Root go.mod/go.sum and Git index remain unchanged. Utilities campaigns had already passed independently; only lightweight shared-metadata/consumer checks were repeated for final composition.

Quick ran race/count20 with default GOMAXPROCS,1,4 and vet on each ARM64 platform. Additional GODEBUG=panicnil=1 race/count20 for TestPanicAndGoexitPrivateAndRecoverable passed on both. The two nested completion/return flags distinguish recovered legacy nil-panic from Goexit without formatting the recovered value. Tests verify Value identity on panic, nil Value on Goexit, correct Goexit bool, nonempty runtime.Stack, fixed Error text, errors.Is(ErrPanic), errors.As(*PanicError) and JSON absence of Value/Stack in both cases. A panic payload whose String method itself panics is never formatted. Normal errors retain exact identity, including a json.Marshaler that would expose a secret if Err were not omitted. Report slice/name mutation cannot change later evaluations.

Timeout/cancellation tests use synctest, not precision sleeps. Parent100ms deadline/cause beats the default5s; callback receives that cause. Config copied by value and Live invokes no checker. Nil-context evaluations freeze registration per D47.C's first-call rule. Known fast results stay up while another check times out. A stuck callback is never replaced: the two-slot test performs1,000 busy polls, checks runtime.NumGoroutine with synctest barriers, verifies exactly two original callback invocations, then releases them and proves a new evaluation executes fresh callbacks instead of reusing late results. Concurrent evaluations share the same global limit. Tests release every artificial gate and wait for live entries to finish.

Stress/count3 on macOS:3,200 concurrent evaluations per run (9,600 total) across eight checkers, yielding7,456/7,647/7,398 actual callback executions as busy results intentionally prevent duplicate work. Outcomes alternate success, failure, panic, Goexit and context observation. Each run also performs2,000 canceled polls over two stuck entries and then releases them, with total0 in the standard goroutineleak profile. Memory fixture abandons1,000 fully evaluated registries per run, half with captured panic stacks; warmed post-GC heap pairs424,504→424,136,463,304→463,672,476,120→476,856 bytes remain below the fixture-specific8MiB runtime/cache allowance. This does not promise a global memory cap or reclamation of noncooperative foreign code.

Fuzz targets Config/Names/RegistrySequence,3s/parallel2: macOS185,830/18,914/114,565 executions; Linux106,439/72,664/97,583. Sequences and lengths are bounded so fuzz cannot create infinite real waits. Config extremes do not cause allocations proportional to MaxParallel/MaxChecks in New. Name validation uses an independent ASCII-set oracle; sequence model checks registration limit/freeze and ordered completed reports. No panic, invalid result or unexpected invocation error found within these campaigns. Coverage97.0% is a diagnostic, not sufficient proof by itself.

Benchmarks b.Loop/benchmem100ms/count3 measure evaluation with1/16/128 quick checks and construction of reports of those sizes on both platforms. ReportConstruction is a struct/slice-allocation baseline, not a full evaluation. Representative macOS one-check evaluation1200B/14allocs;128-check evaluation approximately72,7KB/777allocs, with scheduler-dependent costs. No SLA, zero-allocation or equivalence to the leaking legacy implementation is claimed. Deliberate callback waiting time is not presented as module overhead.

Linux ran the same race/count20 matrix, vet, legacy nil-panic check, stress/count3, three bounded fuzz campaigns, benchmarks100ms/count3, and minimal/composed go run/go test-race. Its memory fixture heap pairs236,272→236,640,246,896→246,528,255,920→255,552 bytes also remain within the measured-fixture allowance. Execution used image e432b43af23a with the official Go1.27.4 linux-arm64 archive, copied source/cache in disposable tmpfs and network disabled. Root lint/regression and generic CLI checks are macOS-only evidence, not attributed to Linux.

Minimal resolves only health+stdlib, using CheckerFunc, readiness probe, diagnosis, freeze and JSON privacy. Composed resolves health+logging+stdlib and explicitly logs a private normal check error through a caller-owned slog logger, while neither that text nor panic Value/Stack reaches serialized Report. It does not import engine/router/drivers and health itself never imports logging. Generic imports/module-graph/exact-replace checks passed offline. No per-module script, Makefile/workflow change or duplicated dependency list was added.

Gorelease ran using the externally installed tool in a temporary source copy outside the worktree, GOWORK=off/GOPROXY=off, -base=none -version=v0.1.0. Result: v0.1.0 is a valid semantic version for this release. First version with no published base; no historical compatibility comparison, tag, proxy, commit or publication was invented. This is a proposed version, not approval.

Raw evidence/ logs/graphs/coverage are ignored local artifacts. This versioned summary retains commands, actual platform/workload bounds, results and limitations. SOURCES.sha256 identifies candidate files; EVIDENCE.sha256 identifies local logs, excluding the manifests. Key logs: quick.log, stress/stress.log, fuzz/fuzz-Fuzz*.log, bench/benchmarks.log, legacy-nil-panic.log, root-lint.log, root-guards.log, root-test.log, related-consumers.log, make-deps.log, make-consumers.log, gorelease.log and linux-arm64.log.

| Evidence | Result |
|---|---|
| E1 | D47/PanicError clarification, inventory, README/MIGRATION/CHANGELOG and contract recorded. |
| E2 | Functional/race/vet, persistent-worker/cap/privacy tests, bounded stress/fuzz/bench and standard leak profile passed on both ARM64 platforms within stated scopes. |
| E3 | In-memory checks, deterministic timeouts/gates and abnormal completion; no driver, socket, collector or framework. |
| E4 | stdlib-only library/minimal and real logging composed pass offline with generic graph/replaces guards. |
| E5 | External gorelease passed, first version without base. |
| E6 | Proposed independent v0.1.0 remains unapproved; catalog stays unreleased/-. |
| E7 | Real-tag consumers deferred until authorized publication. |

Limitations: uncooperative checkers retain their bounded slots until they terminate; multiple registries/evaluation callers have independent budgets; arbitrary checker-owned resources/subgoroutines remain caller responsibilities. No endpoint, driver construction, forceful cancellation, readiness cache, polling, observability, lifecycle or runtime-global mutation is added. Stack may truncate at64KiB and contains private data outside Error/standard Report JSON. Both health/utilities are local functional candidates, not published stability claims. No commit, push, tags, index edit, release workflow execution or legacy production migration.

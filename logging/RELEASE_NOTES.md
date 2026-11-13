# Logging candidate

## Initial state and approved contract (C1)

Branch feature/split, initial HEAD 8799600f1334eb72f87c57b7d3d0d9266ed3deee; initial `git status --porcelain`: empty. Toolchain Go1.27.4 darwin/arm64. No existing changes are included in this task. D41 logging and D41.1–D41.8 are approved by the architect as supplied by the user. This decision is separate from the repository automation document's identically numbered dependency-boundary decision; neither supersedes the other.

Public API: Config{Level string, Format string, AddSource bool, RedactKeys []string, MaxValueBytes int}; Dependencies{Output io.Writer, Attrs []slog.Attr, Redactor Redactor}; Redactor func([]string,slog.Attr)slog.Attr; DefaultConfig() Config; (Config).Validate() error; New(Config,Dependencies)(*slog.Logger,error); ErrInvalidConfig and ConfigError{Field,Reason} with Unwrap. Config tags are mapstructure/json snake_case. No additional public API.

Defaults: info/json/false/[password,token,authorization,secret]/4096. Validate in declaration order, then Output; literal zeros are not defaulted. Empty redact list disables key selection, empty key entries fail. Keys match case-insensitively and exactly. Writer is borrowed, no I/O at construction, globals or background execution. JSON/Text output handlers are standard slog handlers; a wrapper transforms only user attributes without ReplaceAttr or an extra writing lock. Derivatives share the standard handler mutex.

With/Dependencies attrs materialize immediately; event attrs materialize on emission after Enabled. LogValuer resolves through Value.Resolve. Redactor receives copied group paths for non-group slog attributes only, outside writer locks. Returned groups are processed recursively; redactor must terminate and be concurrency-safe. Sensitive keys cannot be restored by a redactor. Built-in time/level/msg/source bypass transforms; same-named user attrs do not.

Depth: top attr value 1; children of map/struct/slice/array/group add 1; WithGroup paths excluded. Depth17 becomes [TRUNCATED_DEPTH]. Path-local pointer/map/slice identities detect [CYCLE]. Container limit256 plus one [TRUNCATED_ITEMS] marker; per top attr4096 nodes then [TRUNCATED_SIZE]. Sensitive attributes redact without evaluating children. Text leaves normalize invalid UTF-8 then truncate on rune boundaries including …(truncated); tiny limits use a rune-safe prefix of the marker. [REDACTED] is never truncated. Native numbers/bool/time/duration and built-in msg are exempt. Limits are not whole-record memory or callback-cost bounds.

Struct names: json tag first part, '-' omitted, flags ignored, otherwise Go name; either Go/output name can match sensitive keys. Exported embedded fields stay nested. JSON marshalers are decoded with UseNumber and protected as a tree; failure becomes [UNSERIALIZABLE]. Errors/Stringer/TextMarshaler are opaque text unless JSON marshaler takes precedence. []byte becomes base64 text. Nil pointers/interfaces are null. Maps support string-based/integer keys (decimal), sorted; unsupported keys/functions/channels/unsafe pointers become [UNSUPPORTED]. Text rendering is human-readable, not stable parseable structure.

No secret detection in free messages, opaque text, keys, embedded JSON strings or bytes. No durable delivery; Logger discards writer errors, Handler.Handle exposes them. Caller owns mutable payload synchronization and shared writers across independent New calls. No recovery of caller panics except the behavior of slog.Value.Resolve itself. New logger has fixed level, no context extraction, no Close/Flush/lifecycle.

## Inventory and migration map

| Origin | Destination | Action | Required test |
|---|---|---|---|
| pkg/utilities/logger/entity.go Config/Service | Config + native *slog.Logger | adapt, no interface bridge | literal validation, defaults, API consumer |
| service.go NewService/configureLogger | New | replace implicit stdout, normalization, logrus | no writes/globals at New, JSON/text |
| service.go output/file/multiwriter helpers | caller-owned io.Writer | exclude | borrowed writer never closed |
| service.go WithField/WithFields | slog.With/WithGroup | adapt shallow legacy fields to materialized attrs | post-With mutation, independent branches |
| service.go contextExtractor/createEntry | caller attrs or later wrapper | exclude implicit extraction | context handling, no implicit fields |
| service.go SetLogLevel | fixed Config.Level | exclude mutation | Enabled and disabled LogValuer |
| service.go Error/WrapError | slog.Error + caller error management | adapt; no wrapping by logger | error text boundary and causes unaffected |
| service.go FatalError/ExitFunc | application termination | exclude | no fatal API/process exit |
| sanitizer.go SanitizeFields/String | structural key protection | deliberate semantic break from regex | nested keys, opaque text boundary |
| writer.go / logrusadapter | later logging/logrus | exclude | stdlib-only graph |
| pkg/core/client observability + logger consumers in providers/presets | later wrappers/migration | leave unchanged | root regression, composed consumer |

Legacy defaults normalize level/format and may create stdout/file/multiple outputs. The legacy sanitizers inspect top-level strings and regexes, not arbitrary nested object graphs. WithFields shallow-copies field maps while sharing Logrus and ContextExtractor; level is mutable. Error writes the error text as well as its error field; WrapError uses github.com/pkg/errors. FatalError calls Logrus.Fatal and can exit. Construction accepts a writer interface backed by Logrus and may emit fallback warnings. Consumers include pkg/core/client/{entity,observability,resilience}.go, provider/otel and adapter/provider/preset packages across the existing families. Legacy logger, its tests and its consumers remain unchanged.

## Increment evidence

C1: clean preflight, inspected source, contract and migration map recorded before implementation.
C2: strict config/validation and independent defaults pass unit tests.
C3–C5: standard handlers, transformations, materialization, ownership, concurrency, limits and special types pass unit/race and testing/slogtest conformance. Native slog JSON duration is numeric; tests preserve that standard encoding.
C6: race count20/default/1/4, stress count3, standard goroutineleak, four 15s fuzz targets, 500ms/count3 benchmarks and vet pass on macOS ARM64 and Linux ARM64. Final quick rerun includes added confidentiality/captured-key assertions. Coverage91.9% is diagnostic only.
C7: minimal/composed public consumers and derived dependency guards pass offline; composed imports real client.
C8: README/MIGRATION/CHANGELOG and root references complete; final guards, regressions, evidence and source hashes checked. Version proposed only.

## Acceptance

| Evidence | Status | Scope |
|---|---|---|
| E1 contracts/documentation | passed | D41 logging + eight clarifications, ownership, confidentiality, limits, migration |
| E2 functional/resources | passed | unit/race/conformance/stress/leaks/fuzz/bench, both ARM64 platforms |
| E3 integration | passed | memory writers and adapter, fixed fields, failures and concurrent records; no external service applies |
| E4 independence | passed | offline minimal logging+stdlib and composed logging+client+stdlib, real modules, derived import/module/graph/replace boundary |
| E5 gorelease | passed for initial version | external tool, isolated source copy, -base=none -version=v0.1.0; validates candidate with no historical base, no published compatibility comparison |
| E6 version approval | pending | proposal logging/v0.1.0 only; catalog track unreleased, version - |
| E7 real-tag consumers | deferred | requires separately authorized publication |

No commit, push, tags or workflow dispatch.


## Executed commands and scope

Go1.27.4 on darwin/arm64 and linux/arm64. macOS Bash3.2, Linux Bash5.2. Local toolchain PATH points at /private/tmp/go-engine-toolchain/go/bin; GOCACHE=/private/tmp/go-engine-build-cache, GOMODCACHE=/Users/seven/go/pkg/mod, GOTOOLCHAIN=local, GOPROXY=off. Existing golangci-lint2.14.0 is used; no dependency was added or upgraded. The module go.mod has no require. No Makefile/workflow/script was modified.

The current catalog's approved dependency-boundary automation uses a seventh deps column instead of per-module allowlists. Logging registers tool/unreleased/-/production/stdlib, adds its go.work member and two consumers. The existing generic scripts derive the allowlists from this data. No duplicate lists/hooks are introduced.

Reproduce from the root with Go1.27.4 on PATH:

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
scripts/check-module.sh logging deps
scripts/check-module.sh logging consumers
LOGGING_RESULTS_DIR="$PWD/logging/evidence/local" \
LOGGING_FUZZ_TIME=15s LOGGING_BENCH_TIME=500ms \
scripts/check-module.sh logging all
make lint
make lint-arch
make lint-docs
make check-modules
make test
for m in concurrency config client retry circuitbreaker ratelimit resilience config/viper; do
    scripts/check-module.sh "$m" consumers
done
```

Generic quick executes GOWORK=off go test -race -count=20 -timeout=120s ./... at default GOMAXPROCS,1,4, then go vet. Stress executes TestStress with -race and count3. Every fuzz target ran 15s with two workers; benchmarks ran 500ms/count3 without race. Minimal/composed run GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. Full module graph contains only itself; composed additionally selects client. Replaces are development-only, not evidence of go get against a published version.

Fuzz final counts: darwin/arm64 Configuration864059, TextAndKeys260194, Structures200840, Derivation454110 (1779203 executions); linux/arm64 Configuration692508, TextAndKeys392848, Structures147755, Derivation371307 (1604418). These are bounded campaigns, not a proof for all possible inputs. Final fuzz logs use the filenames containing the owning package suffix. Earlier unsuffixed logs are preliminary runs; the original failing oracle is preserved as initial-fuzz-failure.log and the minimized seed is checked in under testdata/fuzz.

Stress per repetition: 24 callers/7200 complete records, no duplicates or disclosed sentinel. Retention exercises1200 abandoned derivatives with 64KiB payloads in eight rounds. macOS baselines~295–326KiB and final~308–346KiB stay within the fixture-specific8MiB allowance after GC; this is not a heap promise for arbitrary application payloads. Profile goroutineleak reports no leaks; all helper goroutines/gates terminate. Instance/bounds stress also passes. testing/slogtest.TestHandler passes with native handler semantics and redaction disabled for the standard fixtures.

Representative macOS local measurements with io.Discard: native slog JSON single integer447ns/op median versus logging525ns/op; preformatted With429ns/op; nested protection2183ns/op and1170B/21allocs per event. Disabled logging~6.82ns/op. These compare defined fixtures on this host; no throughput, zero-allocation or SLA guarantee is made. JSON/text, native reference, construction, per-event/static attributes, truncation and shared/independent parallel execution have full logs.

Root macOS lint, architectural/docs/version checks, make test across the workspace and the eight existing modules' standalone consumers pass. The first sandboxed root regression could not bind local httptest/gRPC/Redis ports; it is retained as root-regression.log and is not counted as a passing suite. The authorized rerun root-test.log with local-port access passed. No legacy production/tests/consumers were modified. Linux runs logging/consumers/guards and full qualification, not root lint or the workspace suite. No AMD64 validation is claimed.

Linux is a local ARM64 execution container, not a service dependency: network disabled, repository read-only, only logging/evidence/linux-arm64 writable and temporary toolchain/cache in tmpfs. Image e432b43af23a, four CPUs and4GiB memory. The existing Go1.27.4 Linux ARM64 archive is injected on stdin; no downloads, servers or new service integrations.

External gorelease command: copy logging/*.go, go.mod and testdata to a temporary directory outside Git; GOWORK=off /private/tmp/go-engine-tools/gorelease -base=none -version=v0.1.0. Tool result: v0.1.0 is a valid semantic version for this release. The temporary copy is removed; there is no published base or artificial tag.

## Findings and resolutions

- Strict/native slog JSON duration encoding is a number, not formatted text; assertions follow the standard handler. Pointers to time.Time/duration preserve native semantics instead of going through text truncation.
- A nil pointer implementing LogValuer emits null without evaluating the nil receiver. Non-nil resolution uses slog.Value.Resolve, including its own panic/loop handling.
- Initial fuzzing found a test-oracle mismatch: encoding/json repairs invalid key bytes individually while strings.ToValidUTF8 can coalesce runs. The oracle now round-trips the key through encoding/json; no implementation workaround changes key semantics. The minimized input remains a regression seed.
- Caller callbacks execute before writing locks. The standard-handler conformance, blocked-writer test and concurrent derivatives validate the composition rather than introducing custom writing locks.
- Built-in fields remain untouched, including duplicate same-named user attrs. Message/opaque text is not secret-scanned. Redaction is not universal DLP; limits do not bound user callbacks or entire records.

Versionable source manifest: logging/SOURCES.sha256 (verify from root with shasum -a256 -c logging/SOURCES.sha256). Local ignored logs: logging/evidence/{darwin-arm64,linux-arm64}; evidence SHA256SUMS is relative to logging/evidence. These local logs do not ship as versioned deliverables; this document preserves the commands, results, limitations and acceptance status.

# utilities candidate — wave 1

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

## U1 — D48 approved contract and inventory before implementation

One stdlib-only module, no require even in tests; public packages profile, text, collections and pointers. No strings/defaults package. D48 replaces the historical contract's package list. Profile{Local,Test,Stage,Prod}, ParseScope trim/lower/last hyphen token, empty scope Local, unknown token wraps ErrUnknownProfile and returns empty Profile; String and exact IsX. No environment access. text retains legacy JoinNonEmpty trimming nonempty parts and RemoveChars sequential literal replacements; CleanSpaces uses Unicode Fields/Join; Truncate counts range runes, clamps negatives, suffix fits limit, and repairs each invalid byte to U+FFFD even when no truncation is needed. Other text functions preserve invalid bytes except their requested whitespace/removal transformations.

Collections Keys/Values unordered, Merge new map/last wins/nil ignored, Filter/Map fresh outer slices (nil stays nil), Find returns first matching value, First/Last preserve zero+false for empty. D48.G explicitly requires match callbacks once per element in order: Find scans the entire input even after remembering its first match. This deliberately differs from the legacy short circuit; its later callback panics propagate. No function keeps internal state/references; container copies are shallow Go value copies, not universal deep clones. Ptr creates a pointer to a value copy, ValueOrZero handles nil, FirstNonNil returns the original first nonnil pointer.

| Origin | Destination | Action | Required evidence |
|---|---|---|---|
| pkg/utilities/app_profile/service.go GetScopeValue/GetProfileByScope | profile.ParseScope(string) | adapt; explicit input, validate final token | D48 table, deterministic fuzz, no env access |
| app_profile IsLocalProfile vs HasSuffix checks | Profile.IsX exact constants | correct inconsistent semantics | dev-local local, contest rejected |
| pkg/router/service.go profile consumer; aws/pkg/clients/sns/service.go helper consumer | application assembly later | preserve legacy; SNS profile claim not confirmed | root regression, no consumer migration |
| helpers/strings.go CleanSpaces regexp | text.CleanSpaces Fields/Join | adapt Unicode whitespace | Unicode/invalid bytes, stdlib benchmark |
| helpers Truncate byte slicing | text.Truncate runes+bounded suffix | deliberate break; no panic at negative limit | UTF-8/rune limits/fuzz |
| helpers JoinNonEmpty/RemoveChars | text same names | preserve trimming/literal sequential operations | empty/whitespace/invalid bytes |
| helpers/maps.go Keys/Values/Merge | collections | adapt generic copies, no sorting | nonmutation/nil/precedence/order-free checks |
| helpers/slices.go Filter/Map/Find/First/Last | collections | adapt nil consistency and full Find callback traversal | counts/order/panics/new storage |
| helpers/pointers.go Ptr/ValueOrZero + coalesce.go CoalescePtr | pointers | preserve/rename FirstNonNil | pointer identity/zero/nil |
| helpers/defaults.go and coalesce.go scalar defaults | cmp.Or | retire new wrappers, not legacy APIs | migration map/std examples |
| redundant checks/maps/slices/strings constructors | len, slices, strings, make and v,ok | retire from new scope | migration table |
| helpers/errors.go wrappers | errors.New/fmt.Errorf %w | retire new wrappers | no new error package/API |

Confirmed legacy GetScopeValue reads SCOPE and defaults empty to local, GetProfileByScope accepts any final token, local matches whole scope while other predicates use suffix (contest passes test). Legacy CleanSpaces regexp \s+ is narrower than Unicode Fields; Truncate slices bytes and negative input can panic. Legacy Map(nil) makes a nonnil empty slice, Filter can return nil for empty nonnil input, Find stops at first match. SNS's profile-consumer claim was not confirmed: SNS imports helpers; pkg/router is the confirmed profile consumer. Legacy DefaultString/CoalesceString treat whitespace as empty, while cmp.Or treats only zero values; the migration explicitly preserves that distinction instead of claiming equivalence. No undocumented helper is copied automatically. D48 alone defines the new surface. U2–U7 implemented; executed E1–E5 evidence follows. E6 proposed independent v0.1.0 unapproved; E7 tag consumers deferred.

## Executed validation and candidate (U2–U7)

Go1.27.4 darwin/arm64 (Apple M1 Max) and Go1.27.4 linux/arm64 (local disposable container,4 CPUs/4GiB, network disabled). No AMD64 run is claimed. Root lint/regression and generic guards/related-consumer regression are macOS-only. On macOS, golangci-lint2.14.0 was already installed externally. During work HEAD advanced externally from the recorded initial3d317bd to d0ff735 (add client/otel). No commit was executed by this agent; prior D45/D46 files remained preserved and this observation does not rewrite initial evidence.

U2 profile table/predicates/determinism passed before U3. U3 Unicode text/invalid bytes/negative limits passed before U4. U4 unordered map contents, new outer storage, nil/empty behavior, complete ordered callbacks, propagated panics and pointer identity passed before U5–U7. Both independent consumers and the catalog/stdlib boundary passed. No require/go.sum exists in the library; composed owns its config/viper dependencies and replace directives.

With GOTOOLCHAIN=local, GOPROXY=off and a prepared cache (module/consumer commands also GOWORK=off GOFLAGS=-mod=readonly):

```sh
scripts/modules.sh validate
scripts/modules.sh lint-lists
scripts/check-module.sh utilities deps
scripts/check-module.sh utilities consumers
scripts/check-module.sh utilities quick
scripts/check-module.sh utilities stress
UTILITIES_FUZZ_TIME=3s scripts/check-module.sh utilities fuzz
UTILITIES_BENCH_TIME=100ms scripts/check-module.sh utilities bench
make lint
make lint-arch lint-docs check-modules
make test
scripts/check-module.sh config consumers
scripts/check-module.sh config/viper consumers
scripts/check-module.sh logging consumers
```

Commands passed on macOS; root make test result is recorded in root-test.log. Quick executes go test-race-count20 in all four packages with default GOMAXPROCS,1,4, then vet. Linux executed the same three race matrices/vet, collections stress/count3, all five fuzz targets3s/parallel2, benchmarks100ms/count3 and both consumers go run/go test-race. Root checks are not attributed to Linux. Standard goroutineleak reports total0. The pure library starts no goroutines; stress goroutines are caller/test-owned and joined.

Stress exercises32,000 collection pipelines/map copies per run,96,000 total, plus4,000 1024-int batches abandoned per memory run. Warmed post-GC heap pairs on macOS271,952→271,984,279,088→279,232,285,712→285,728 bytes stayed below the fixture-specific4MiB runtime/cache allowance. This is not a general process-memory bound. Stress is intentionally in collections, where copies/callbacks/concurrency are exercised; other packages have unit/fuzz/bench coverage and generic stress logs correctly report no matching TestStress in those packages.

Fuzz executions, target3s each (elapsed can include baseline/finalization): macOS ParseScope160,192; Truncate362,140; Text178,008; Collections246,249; Pointers425,214. Linux respectively26,224/240,127/23,172/251,381/359,699. No panic/nondeterminism/mutation/invalid Truncate output found within these bounded campaigns. Coverage100% in each package is diagnostic, not sufficient acceptance evidence by itself.

Benchmarks b.Loop/benchmem compare Keys with slices.Collect(maps.Keys), CleanSpaces with Fields/Join, profile parsing with stdlib token extraction, and measure Unicode truncation, map transforms and pointer reads.100ms/count3 on both ARM64 platforms; no throughput/SLA/zero-allocation guarantee. The stdlib token baseline omits profile validation, so it is a reference cost, not equivalent behavior. Find is not compared as equivalent to a short-circuit stdlib search because D48.G requires its full traversal.

Independent minimal imports only utilities and stdlib. Composed reads SCOPE explicitly in main, selects a known profile and base+profile YAML in a supplied fstest FS, decodes config/viper over seeded defaults and rejects unknown profiles. No engine/driver/resource construction or implicit cwd/env access in utilities. Generic module/import/graph/replace checks passed offline. config/viper uses the existing versions; no dependency/directive update was made. Linux preparation initially attempted to overwrite read-only files in a temporary copy of the cache; restoring write permission on that disposable copy resolved setup. Neither the real module cache nor dependency versions were changed. The failed preparation is not counted as validation.

Gorelease ran externally in a temporary copy, outside the worktree, GOWORK=off/GOPROXY=off with -base=none -version=v0.1.0. Result: v0.1.0 is a valid semantic version for this release. First version, no published base, no historical API comparison or artificial tag/proxy. This validates a proposal, not approval/publication.

Raw evidence/ files are ignored local artifacts; this versioned summary keeps commands, platform/workload bounds, measured results and limitations. SOURCES.sha256 identifies candidate files; EVIDENCE.sha256 identifies local logs, excluding the manifests themselves. No check omitted is claimed passed.

| Evidence | Result |
|---|---|
| E1 | D48 contract, inventory/migration table, README/MIGRATION/CHANGELOG complete. |
| E2 | Unit/race/vet, bounded stress/five fuzz targets/benchmarks and standard leak profile passed on both ARM64 platforms as scoped above. |
| E3 | Pure composition fixtures and explicit FS/profile/config precedence; no service/network. |
| E4 | stdlib-only module/minimal; real config+config/viper composed passed offline. |
| E5 | External gorelease passed; first version without base. |
| E6 | Proposed independent v0.1.0 remains unapproved; catalog unreleased/-. |
| E7 | Consumers against a real authorized tag deferred. |

No legacy production/consumer migration, aliases, global state, release workflow execution, commit, push, tags or index edits. Utilities was validated first. Health's PanicError clarification was subsequently approved and its own implementation/evidence is recorded separately in health/RELEASE_NOTES.md. No distribution/version approval is implied by either candidate.

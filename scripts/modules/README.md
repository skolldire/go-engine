# Module automation (D40)

`modules.tsv` is authoritative. Bash 3.2+ and standard Unix utilities are supported; Go/Make/git are existing prerequisites. No jq, Python, Perl or yq runtime dependency is introduced.

## Add a module

- Add one tab-separated catalog row with dir/path/kind/track/version/checks/deps. New unapproved modules use unreleased, version `-`. `deps` is `stdlib`, or a comma-separated list of catalog dirs and external module paths; production modules must declare it.
- Add the dir to go.work (the reader checks set equality).
- Create the module's own go.mod and implementation under its approved contract.
- For production, write at least one `Fuzz*` target. Nothing else is listed per module.
- Supply examples/standalone/<dir>/{minimal,composed} with their own go.mod and documented local replaces.
- Run modules.sh validate/lint-lists and check-module.sh <dir> deps/consumers/quick; use stress/fuzz/bench for qualification.
- Do not edit Makefile, workflows or scripts just to register the module. No tag is eligible until the catalog track/version is explicitly approved.

## The derived boundary (D41)

The `deps` column is the only per-module dependency declaration. `check-module.sh <dir> deps` derives the rest from go.mod and the build graph:

- Direct imports, tests included, come only from stdlib, the module itself and `deps`. This replaces the former forbidden-imports (config parsers) and forbidden-importers (client, resilience) lists: a module whose `deps` does not name client cannot import it.
- Every module selected in the graph is reachable in `go mod graph` from a declared dep, so an undeclared module cannot hide as `// indirect`.
- Every declared dep is required in go.mod, so the catalog cannot go stale.
- Replaces are exactly the in-repository modules of the graph, each pointing at its catalog dir. Unused or missing replaces fail.
- At least one `Fuzz*` target exists. With `FUZZ_BASELINE_REF=<ref>`, no target present in that ref may disappear; CI sets it to the pull request's base branch.

Consumers apply the same rules. `minimal` may import the module and its `deps`; `composed` may also import unreleased tool, composition and source modules, never the legacy core, families, presets or the harness. The module must appear in at least one consumer's graph.

Selected versions are not pinned by the checker; go.sum and `-mod=readonly` keep them reproducible, and a version change shows in the go.mod diff.

`fuzz` mode discovers targets with go test -list in every package of the module and fuzzes each one in its owning package.

## Exceptions kept as data/hooks

`overrides.tsv` holds every per-module exception as `dir<TAB>key<TAB>value`; `modules.sh validate` rejects unknown dirs and keys. Keys: consumers-dir, stress-env, env-prefix (legacy fallback), fuzz-time, bench-time, stress-hook. A module without rows uses the defaults: fuzz 30s, bench 500ms, stress variable `<PREFIX>_STRESS`, consumers in examples/standalone/<dir>. Canonical prefix is uppercase name without hyphens. Stress sets `<PREFIX>STRESS=1` plus the stress variable; duration/output overrides use `<PREFIX>_FUZZ_TIME`, `_BENCH_TIME`, `_RESULTS_DIR`, with legacy fallback.

config/viper shares the existing config minimal/composed consumers through its consumers-dir override; its tests read CONFIG_STRESS. Minimal validates config core; composed exercises real viper/concurrency. This preserves the source's existing integration scope without changing consumers. Its legacy environment fallback is CONFIG; canonical is CONFIGVIPER.

concurrency.sh preserves the original expanded stress campaign: lifecycle count=5/default/1/4, sustained workload and retention. Defaults remain fuzz 60s/bench 1s (other modules 30s/500ms). Generic stress remains TestStress/count=3. Only declared exceptions get hooks; hooks receive mode and dir, output goes to MODULE_RESULTS_DIR.

config.sh preserves the root configcomparison test and benchmark as engine/bench-engine hook modes. Temporary aliases check-config-engine/bench-config-engine route there. Other old check-<name>[-deps|-quick|-stress|-fuzz|-bench|-production] aliases use one Make pattern and resolve names through the catalog. They are deprecated for this release and will be removed later; prefer `make check-module M=config/viper MODE=quick`.

Read RELEASING.md for tracks, release approval, dry-run and executed evidence. `scripts/test-modules.sh` provides unit/matrix, equivalence, mutation, dry-run, fictitious-module and boundary demonstrations; `boundary` mutates a disposable snapshot once per D41 rule and expects each violation to be rejected for its own reason. Equivalence needs the original snapshot (WAVE0_OLD_ROOT; reproduction command in RELEASING); it is historical migration evidence, not a permanent CI dependency.

## D43/D44 policy corrections

`modules.sh validate` parses Go imports with the standard-library helper import-policy.go, independent of build tags. Coordinated module source cannot import an unreleased catalog module. Nested modules (including standalone consumers/harnesses) are separate boundaries, not scanned as their parent; hidden directories and duplicate " 2" files are skipped. The parser does not mistake strings/comments for imports. The root dependency in deps is written `.` and resolves to the catalog's root module path.

Unreleased/independent rows cannot declare github.com/sirupsen/logrus; check-module also rejects it in selected graphs, including transitive dependencies. Nonpublished comparison harnesses can import the legacy root explicitly; this is not permission for a production module to recreate legacy coupling. Production rows retain the fuzz gate; basic harness deps checks do not demand fuzz targets. Harnesses join go.work/catalog and CI automatically without pretending to be released libraries or requiring standalone consumers.

`scripts/test-modules.sh decisions` mutates a disposable snapshot: tagged coordinated-to-unreleased import, forbidden Logrus deps declaration and Logrus graph. Snapshots overlay the current changed/untracked files and deletions onto archived HEAD, so uncommitted harness migrations are faithfully checked without changing the live repository or index.

## Composed application dependencies (D46.6)

`scripts/modules/consumer-deps.tsv` declares additional external dependencies of composed applications: module-dir, literal `composed`, comma-separated exact external module paths. No wildcard or in-repository module is allowed; minimal/module entries fail validation. Existing catalog permissions are not repeated here. The client/otel composed application declares SDK trace/metric so it can construct capture processors/readers; the API-only wrapper and minimal consumer receive no such permission.

`modules.sh validate` and `modules.sh consumer-deps <dir>` validate owner/consumer directories, exact paths, duplicates and D44 Logrus rejection. Every extra must occur in the consumer go.mod and be used by a direct import in its own sources/tests. Unused, redundant or missing declarations fail. Imports are parsed independently of build tags; nested modules, hidden/testdata directories and " 2" files are excluded. Direct external imports outside existing permissions require declarations even if their modules are already transitively selected. The shared checker then applies the same selected-graph reachability, exact-replace and no-Logrus rules, only to composed. It never widens the wrapped module or minimal boundary.

Run `scripts/test-modules.sh consumer-deps` for disposable-snapshot mutation coverage: SDK rejected in wrapper/minimal/undeclared composed, declared composed accepted, unused/Logrus/minimal/module/wildcard/repository/unknown-owner/missing-require/missing-directory declarations rejected. No mutation touches the worktree or index. Go is already required; the helper uses only stdlib, with offline go commands. No per-module checker is introduced.

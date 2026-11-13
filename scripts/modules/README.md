# Module automation (D40)

`modules.tsv` is authoritative. Bash 3.2+ and standard Unix utilities are supported; Go/Make/git are existing prerequisites. No jq, Python, Perl or yq runtime dependency is introduced.

## Add a module

- Add one tab-separated catalog row with dir/path/kind/track/version/checks. New unapproved modules use unreleased, version `-`.
- Add the dir to go.work (the reader checks set equality).
- Create the module's own go.mod and implementation under its approved contract.
- For production, provide `<name>-modules.txt`, `<name>-fuzz.txt`, `<name>-{minimal,composed}-modules.txt` and `<name>-{minimal,composed}-replaces.txt` in scripts/allowlists. Empty module file means stdlib-only; an absent module file has the same meaning for standalone deps invocation. Names replace `/` with `-`.
- Supply examples/standalone/<dir>/{minimal,composed} with their own go.mod and documented local replaces.
- Run modules.sh validate/lint-lists and check-module.sh <dir> deps/consumers/quick; use stress/fuzz/bench for qualification.
- Do not edit Makefile, workflows or scripts just to register the module. No tag is eligible until the catalog track/version is explicitly approved.

Allowlist module lines are exact `module-path [selected-version]` entries, comments start `#`. Graphs are closed: all declared entries must be selected, and no additional module/package dependency is accepted. Pin versions when the prior contract pins them. Graph edges validate permitted module paths (MVS can include older edge versions); selected versions are checked separately. Replaces include every directive in the consumer go.mod, including unused ones.

Fuzz files contain full names (`FuzzConfig`), one per line. Every name must exist in exactly one package according to go test -list; the runner discovers that owning package and fuzzes it, including subpackages; Go itself otherwise exits successfully when a fuzz regex matches nothing.

## Exceptions kept as data/hooks

`<name>-options.tsv`: env-prefix (legacy fallback), stress-env (existing Go tests' variable), fuzz-time, bench-time, stress-hook. Canonical prefix is uppercase name without hyphens. Stress sets `<PREFIX>STRESS=1` plus its declared legacy variable; duration/output overrides use `<PREFIX>_FUZZ_TIME`, `_BENCH_TIME`, `_RESULTS_DIR`, with legacy fallback. Existing tests use `_STRESS`, so that spelling is preserved as data rather than editing Go tests.

config/viper shares the existing config minimal/composed consumers through config-viper-consumers-dir.txt. Minimal validates config core; composed exercises real viper/concurrency. This preserves the source's existing integration scope without changing consumers. Its legacy environment fallback is CONFIG; canonical is CONFIGVIPER.

`*-forbidden-imports.txt` rejects direct source import patterns even when transitively allowlisted (config/config-viper parser boundary). `*-forbidden-importers.txt` retains client D26 and resilience inverse-import checks, skipping optional modules that do not yet exist.

concurrency.sh preserves the original expanded stress campaign: lifecycle count=5/default/1/4, sustained workload and retention. Defaults remain fuzz 60s/bench 1s (other modules 30s/500ms). Generic stress remains TestStress/count=3. Only declared exceptions get hooks; hooks receive mode and dir, output goes to MODULE_RESULTS_DIR.

config.sh preserves the root configcomparison test and benchmark as engine/bench-engine hook modes. Temporary aliases check-config-engine/bench-config-engine route there. Other old check-<name>[-deps|-quick|-stress|-fuzz|-bench|-production] aliases use one Make pattern and resolve names through the catalog. They are deprecated for this release and will be removed later; prefer `make check-module M=config/viper MODE=quick`.

Read RELEASING.md for tracks, release approval, dry-run and executed evidence. `scripts/test-modules.sh` provides unit/matrix, equivalence, mutation, dry-run and fictitious-module demonstrations. Equivalence needs the original snapshot (WAVE0_OLD_ROOT; reproduction command in RELEASING); it is historical migration evidence, not a permanent CI dependency.

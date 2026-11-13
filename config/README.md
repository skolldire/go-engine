# config — experimental candidate, not released

Independent Go 1.27.4 module; stdlib only, including tests. `config/json` and
`config/env` are packages in this module. No engine, Viper, mapstructure, drivers,
logger or dependency on other go-engine modules. E6/E7 are deferred until the
modularization release work; this candidate is not a published stable API.

## Load and decode

```go
sourceConfig := jsonsource.DefaultConfig()
sourceConfig.Path = "app.json"
source, err := jsonsource.New(sourceConfig, jsonsource.Dependencies{FS: filesystem})
if err != nil { return err }
loader, err := config.New(config.DefaultConfig(), config.Dependencies{
    Sources: []config.Source{source},
})
if err != nil { return err }
snapshot, err := loader.Load(ctx)
if err != nil { return err }
cfg := tool.DefaultConfig()
if err := snapshot.Decode("tool", &cfg); err != nil { return err }
if err := cfg.Validate(); err != nil { return err }
```

Import aliases: `config` is github.com/skolldire/go-engine/config and
`jsonsource` is github.com/skolldire/go-engine/config/json. `filesystem` is an
explicit fs.FS selected by the application. The application constructs its tool;
config never imports the tool, validates business rules or controls its lifecycle.
Executable examples: [minimal](../examples/standalone/config/minimal) and
[composed with concurrency](../examples/standalone/config/composed).

## Contract

`New(Config, Dependencies)` validates/copies config and Sources without I/O.
`Load(ctx)` loads Sources in order (last wins), normalizes/copies them, merges,
resolves references and returns an immutable Snapshot with a random 128-bit hex
Revision. Revisions identify successful loads; they do not hash secrets, prove
semantic equality or implement remote CAS. Failed loads return no snapshot.

| Config field | Default | Validation |
|---|---:|---|
| MaxSourceBytes | 1 MiB | Positive; deterministic normalized budget per Source |
| MaxDepth | 64 | Positive; root level 1; checked before descent |
| MaxExpansionDepth | 8 | Positive; maximum nested reference expansion |
| RejectUnknown | true | false rejected |

Go literals are used as supplied. New never fills zero fields. Snapshot.Decode
uses a deep copy of the seeded target and commits only on success; omitted fields
retain defaults, explicit zero/false/empty values are honored. Maps merge by key;
lists replace. null removes keys during merge; surviving list nulls cannot decode.
Objects merge objects, scalars replace scalars, lists replace lists. Cross-shape
collisions fail with both source names. No raw mutable getters exist.

`Decode(section, target)` requires a non-nil pointer. Empty section means root;
other sections use dot-separated `[a-z0-9_-]+` segments. Has returns false for
invalid/missing paths. Dot-containing map keys cannot be addressed as sections.
Field names derive snake_case, preserving acronym boundaries; `config:"name"`
overrides and `config:"-"` skips. json/yaml/mapstructure tags are ignored.
Exported anonymous structs flatten unless explicitly tagged; collisions fail.

Supported values: bool/string, signed/unsigned integers, float32/64, time.Duration,
structs, string-keyed maps, slices, exact-length arrays, TextUnmarshaler values.
Pointers/interfaces/functions/channels are unsupported. Durations require unit
strings (`0s`, not `0` or numeric literals). JSON numeric strings cannot populate
numbers. Only env/resolved text can convert to scalar types. Integers require
strict base10 syntax; overflow/decimal-to-int/float32 precision loss are errors.
TextUnmarshaler runs on a fresh value, never the seeded object's mutable state.
Validate on target is always the application's responsibility.

## Sources and ownership

JSON: `DefaultConfig()` gives MaxBytes=1 MiB, Required=true; Path is required.
Dependencies accepts exactly one FS or Reader(ctx)→io.ReadCloser. FS paths must
satisfy fs.ValidPath; Reader mode uses Path as a source identifier. Every opened
reader is closed once, also on failure; close errors are returned. Required=false
ignores only fs.ErrNotExist. Duplicate keys, malformed JSON and extra values fail.
Decoder.UseNumber and token parsing preserve decimal precision. jsontext was not
importable in the available toolchain; the user approved this stdlib replacement.
No experimental Go feature is required.

Env: Config has nonempty Bindings (path→variable) and optional Required paths;
Dependencies.Env is mandatory. New copies bindings and rejects invalid paths,
prefix collisions and required paths without bindings. Every binding is read
once per Load; missing omits the path, missing required fails, empty remains empty.
The source never enumerates environment variables or implicitly calls os.Getenv.

Sources produce Document containing nil/bool/string/json.Number/integers/finite
float64/[]any/map[string]any. Other types fail with ErrInvalidDocument. env uses
an internal marker to carry textual provenance. Loader owns normalized copies;
a Source must not mutate the returned document during its Load/copy operation.
Subsequent mutations do not change a Snapshot. Strings are immutable Go values.

## References

`${ref}` and `${ref:-fallback}` may occur within strings; `$${` escapes `${`.
References are names or scheme:name; scheme `[a-z][a-z0-9+.-]*`, name
`[A-Za-z0-9_./-]+`. Resolver receives the entire ref. ErrReferenceNotFound means
missing: only missing uses fallback. Empty is present and overrides fallback.
Without Resolver any real reference fails. Returned values/fallbacks are expanded
recursively, cycles/depth/size limits enforced. Each distinct ref is looked up
once per Load, including missing refs. No shell, commands or expressions.

For a coherent environment, the application captures the required environment
into an immutable map once, provides the same lookup function to env and its
Resolver, and maps missing to config.ErrReferenceNotFound. Minimal demonstrates
this with an explicit captured map. config cannot atomically snapshot an arbitrary
external lookup backend on the caller's behalf.

## Limits, concurrency and errors

Normalized measure: one per node plus key/string/numeric-text lengths. Raw JSON
MaxBytes is separate. Final expanded measure <= MaxSourceBytes × number of Sources;
overflow of this budget is rejected in New. Sources may allocate before returning;
config does not limit arbitrary callback heap usage. Supply bounded Sources.

Load on one Loader is serialized by a cancelable semaphore in the calling
goroutine. A shared Source/Resolver across different Loaders must be concurrent-safe.
Snapshot methods are concurrent-safe with separate targets; sharing a target is
a caller race. Caller dependencies must cooperate with context to stop promptly;
reads are checked between calls, without abandoned background goroutines. There
is no Close, watcher, background worker or implicit I/O in New.

Sentinels: ErrInvalidConfig, ErrSectionMissing, ErrInvalidDocument,
ErrLimitExceeded, ErrUnresolvedReference, ErrReferenceNotFound, ErrReferenceCycle,
ErrNilContext. ConfigError{Field,Reason} unwraps ErrInvalidConfig. Diagnostics add
field/source identifiers, never values. Source.String identifiers must not contain
secrets. Injected Source/Resolver/I/O errors retain their causes with %w: callers
are responsible for not putting secrets in these external errors. Parser and
conversion failures created here are sanitized.

## Validation

From the repository root with Go 1.27.4:

```sh
make check-config check-config-quick check-config-stress check-config-fuzz check-config-bench
```

The scripts run offline with GOWORK=off and -mod=readonly. CONFIG_RESULTS_DIR
selects the log directory; CONFIG_FUZZ_TIME (default 30s/target) and
CONFIG_BENCH_TIME (default 500ms, three repetitions) select campaign lengths.
Unit/composition tests require no external service. Heavy harnesses, YAML, Viper,
SSM, reload and engine migration are separate tasks. See
[implementation evidence](../docs/modules/config.md).

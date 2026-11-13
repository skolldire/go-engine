# logging (unreleased candidate)

Explicit construction of `*slog.Logger` with structural redaction. Go1.27.4, stdlib only, no background work or global logger. Track unreleased; no published stability claim.

```go
cfg := logging.DefaultConfig()
cfg.Level = "info"
logger, err := logging.New(cfg, logging.Dependencies{Output: output})
if err != nil { return err }
logger.With("component", "orders").InfoContext(ctx, "accepted",
    "request", map[string]any{"password": "hidden", "count": 1})
```

`output` is a caller-owned `io.Writer`. New validates and copies configuration; it neither opens nor writes nor closes that resource. JSON/Text serialization and synchronization come from standard slog handlers, without ReplaceAttr. Each logger's derivatives share the standard mutex. Independent New calls sharing one writer need caller synchronization. There is no guaranteed durable delivery, async queue, flush, level mutation, shutdown, retry or process exit.

## Public contract

```go
type Config struct {
    Level string
    Format string
    AddSource bool
    RedactKeys []string
    MaxValueBytes int
}
type Redactor func(groups []string, attr slog.Attr) slog.Attr
type Dependencies struct {
    Output io.Writer
    Attrs []slog.Attr
    Redactor Redactor
}
func DefaultConfig() Config
func (Config) Validate() error
func New(Config, Dependencies) (*slog.Logger, error)
type ConfigError struct { Field, Reason string }
var ErrInvalidConfig error
```

Config fields have matching snake_case `mapstructure` and `json` tags. No config module is imported. Defaults: Level info, Format json, AddSource false, RedactKeys password/token/authorization/secret, MaxValueBytes4096. Level accepts debug/info/warn/error, Format json/text, MaxValueBytes>0. Literal values are validated without implicit defaults in declaration order; empty redact-key entries fail, while a nil/empty list disables key selection. Output is validated after Config. Errors name fields without values, support errors.As to *ConfigError and errors.Is to ErrInvalidConfig. A typed nil hidden inside io.Writer remains the caller's responsibility.

## Ownership and evaluation

Dependencies.Attrs and Logger.With materialize values immediately, including LogValuer, Redactor and normalization. Later mutation of their maps/slices/structs does not change that derived logger. The supplied slices are not modified. Event attributes materialize during emission; disabled records do not resolve or traverse their values. Payloads must not be concurrently mutated while emitting or deriving. LogValuer resolves through slog.Value.Resolve with standard limits and panic behavior; arbitrary caller callbacks are not recovered by this module.

Redactor receives a copied path of WithGroup/slog.Group names for each non-group slog attribute. Map/struct entries do not invoke it. It may discard slog.Attr{}, rename, transform or return groups. Returned attributes are protected again; returned groups recurse with the same limits. A callback must be concurrency-safe and terminate. Recursion through the same logger from a redactor is unsupported unless the caller explicitly makes it finite. Callbacks run before the standard writing lock. WithGroup names remain separate from normalization depth. A sensitive WithGroup is represented once as a fixed [REDACTED] attribute; subsequent contents/derivations in that protected scope are suppressed without evaluation.

## Redaction and normalization

Keys compare exactly with strings.EqualFold, without paths, regex, substring matching or trimming. Sensitive values become the complete `[REDACTED]` marker before traversing children; that marker is exempt from text truncation. Scalar LogValuer resolution and the optional Redactor precede key protection for ordinary slog attributes.

Protection applies to static/event attributes, groups, maps, exported struct fields and nested collections. Struct fields use the first json-tag component, omit json:"-", ignore omitempty/string flags, and otherwise keep the Go name. Either the Go name or output name may trigger protection. Exported embedded fields stay nested; private fields are not inspected. No unsafe field access is used. Pointer/map/slice identities are observed solely to detect cycles.

Special values:

- time.Time/time.Duration, numbers and bool retain native values; json.Number preserves decimal spelling.
- JSON marshalers take precedence over opaque error/Stringer/TextMarshaler treatment. Their JSON is decoded with UseNumber and protected structurally; invalid JSON/errors become `[UNSERIALIZABLE]` without the error text.
- Errors, Stringer and TextMarshaler are opaque text. TextMarshaler errors become `[UNSERIALIZABLE]`.
- []byte is base64 text, then text-limited. Nil pointers/interfaces/maps/slices become null.
- Maps support string-based and integer keys (base10); keys are selected/emitted lexically. Other key types, channels, functions, complex numbers and unsafe.Pointer become `[UNSUPPORTED]`.

JSON uses normalized map[string]any/[]any and standard encoding/json through the JSON handler; map keys are sorted by encoding/json. Native slog.Group field order remains its attribute order. Text uses the standard human-readable handler and is not a stable parseable format for nested structures. Built-in time/level/msg/source bypass redaction and truncation. A same-named user attribute is transformed normally; standard slog permits duplicate keys.

## Fixed limits

The root user attribute value has depth1; map/struct/list/array/group children increment it. WithGroup path names do not. A value at depth17 becomes `[TRUNCATED_DEPTH]`. Pointers/interfaces unwrap without increasing structural depth. Address identities on the current traversal path identify `[CYCLE]`; shared acyclic objects remain independent values. Pointer traversals also consume the node budget.

Maps/slices/arrays retain at most256 members, followed by one `[TRUNCATED_ITEMS]` entry. Maps use the marker string as both the synthetic key and value. Structs and groups remain subject to the node budget. At most4096 visited nodes per top-level attribute; unvisited tails become `[TRUNCATED_SIZE]`. The output can include a sentinel for an unfinished container at each ancestor; these are not extra visited input nodes. Literal keys matching marker strings may collide with the synthetic marker entries. Limits apply separately to each top-level attribute.

MaxValueBytes counts UTF-8 bytes of each textual leaf before format escaping. Invalid UTF-8 is repaired with U+FFFD. Truncation preserves rune boundaries and reserves space for `…(truncated)`; tiny limits emit a rune-safe prefix of that marker (possibly empty for limits1/2). It applies to strings, error/Stringer/TextMarshaler output and base64 bytes. Numbers/bool/time/duration, fixed protection markers and built-in msg are exempt. JSON serialization itself is never truncated.

These limits are not a whole-record memory bound. Number of top-level attrs, keys, sorting input traversal and caller callbacks have their own cost. A custom marshaler's output is decoded before bounded normalization; callback output allocations are not constrained here. Writers/Stringer/LogValuer/Redactor can block or allocate; this module cannot force their completion.

## Confidentiality and errors

No secret scanning of free messages, keys, error/Stringer text, JSON embedded in strings, bytes or already-formatted data. Never put secrets in those locations. This deliberately differs from the legacy regex sanitizer. Supplying custom redactors/marshalers is application code and requires review of its own disclosure behavior.

Logger methods discard handler/writer errors, as standard slog does. An explicit handler call can observe a writer error:

```go
h := logger.Handler()
if h.Enabled(ctx, record.Level) {
    if err := h.Handle(ctx, record); err != nil { return err }
}
```

No error is also logged internally. A slow writer blocks the caller. Canceling context does not suppress a record; no implicit context extraction or deadline is added. Source PC is preserved for AddSource.

## Validation

From the root: `scripts/check-module.sh logging deps|consumers|quick|stress|fuzz|bench|all`. `LOGGING_RESULTS_DIR`, `LOGGING_FUZZ_TIME` (default30s), `LOGGING_BENCH_TIME` (default500ms) configure campaigns. `quick` runs race count20 at default/1/4 GOMAXPROCS plus vet. Stress uses LOGGING_STRESS=1; time is not simulated with precision sleeps. Tests include testing/slogtest, memory-backed integration, standard goroutineleak, reference isolation, fuzzing and benchmarks against native slog.

See RELEASE_NOTES.md for executed scope, logs, hashes and pending distribution gates. See MIGRATION.md for deliberate breaks. Examples under examples/standalone/logging run offline with their own go.mod and real public APIs; composed uses client without adding it to this module's dependencies.

# config/yaml

Experimental D55 candidate, not released. Go 1.27.4; depends on `config` and the repository's existing `go.yaml.in/yaml/v3` v3.0.6. A separate module, so `config` itself gains no dependency.

A strict YAML source for [`config`](../README.md). It reads one YAML document into the same values `config/json` produces, and treats everything YAML permits beyond that as an error that names the line, the column and the key path, never the value.

```go
cfg := yamlsource.DefaultConfig()
cfg.Path = "app.yaml"
source, err := yamlsource.New(cfg, yamlsource.Dependencies{FS: os.DirFS("/etc/myapp")})
// handle err
loader, err := config.New(config.DefaultConfig(), config.Dependencies{Sources: []config.Source{source}})
// handle err
snapshot, err := loader.Load(ctx)
// handle err
settings := DefaultSettings()
err = snapshot.Decode("", &settings)
```

`yamlsource` is `github.com/skolldire/go-engine/config/yaml`.

## Source

Same shape as `config/json`.

| Field | Default | Rule |
|---|---|---|
| `Path` | none | required; with `FS` it must satisfy `fs.ValidPath` |
| `MaxBytes` | 1 MiB | > 0 |
| `Required` | `true` | |

`Dependencies` takes exactly one of `FS` or `Reader func(context.Context) (io.ReadCloser, error)`. With a `Reader`, `Path` only names the source. `New` performs no I/O. The source implements `fmt.Stringer` as `yaml:<path>`; do not put secrets in the path.

`Load` reads at most `MaxBytes` and fails with `config.ErrLimitExceeded` as soon as there is one byte more, without reading the rest. Every reader that was opened is closed exactly once, on success and on failure, and a close error is returned. `Required=false` turns a missing file into an empty document and forgives nothing else: only `fs.ErrNotExist`. The input must be UTF-8; a UTF-8 byte order mark is accepted, UTF-16 is not.

## What is accepted

**One document.** An empty file, a file with only comments, or a document that is just `---`, `~` or `null` is an empty document. A second document (another `---` with content) is an error. The root must be a mapping.

**String keys only.** A key that YAML reads as an integer, float, boolean or null, or that is a list or a map, is an error; quote it (`"123": x`) to use it as text. Keys are compared exactly and are not lowercased. The same key twice in one map is an error carrying the key path.

**Core tags only.** `!!str`, `!!int`, `!!float`, `!!bool`, `!!null`, `!!map`, `!!seq` and the merge key. Application tags (`!vault`), `!!binary`, `!!set`, `!!omap`, `!!timestamp` written explicitly and any other tag are errors. An explicit tag that contradicts its content (`!!int nope`, `!!bool yes`) is an error rather than a coercion.

### Scalars

Resolution was checked against the code of yaml.v3 v3.0.6 (`resolve.go`) and is fixed by tests.

| Written (unquoted) | Value | Note |
|---|---|---|
| `true` `True` `TRUE` `false` `False` `FALSE` | boolean | YAML 1.2 |
| `yes` `no` `on` `off` `y` `n` and any other case | string | not booleans |
| `~` `null` `Null` `NULL`, or nothing | null | |
| `42` `-17` `+5` | number `42` `-17` `5` | |
| `0x1F` `0o17` `017` `0b1010` `1_000` | number `31` `15` `15` `10` `1000` | written back in decimal |
| `1.5` `.5` `1.` `1e3` `1E+3` | number `1.5` `0.5` `1` `1e3` `1e3` | digits kept as written |
| `.inf` `-.inf` `.nan` (any case) | **error** | no finite value |
| `2001-12-14` `2001-12-14T21:59:43Z` | string | not a timestamp |
| `30s` `1h30m` `1:30` `1.2.3` | string | |
| anything in quotes, `\|` or `>` blocks | string | `"8080"` is text |

Things to note:

- **`017` is 15.** yaml.v3 still reads a leading zero as YAML 1.1 octal. `08`, which is not valid octal, is the number 8. Write decimal numbers without a leading zero.
- **Numbers are exact.** They are emitted as `json.Number` in JSON syntax and the digits are not passed through a `float64`, so a long decimal or an integer beyond 64 bits is not rounded while loading.
- **Dates and durations stay text.** A plain `2026-01-02` or `30s` is a string here and is interpreted when a section is decoded, by the type of the target field (`time.Duration`, a `TextUnmarshaler`, or a plain `string`).
- **File values keep their written type.** As in the rest of `config`, text from a file is not converted when decoding: `"8080"` in quotes does not populate an `int` field, `yes` does not populate a `bool`, and a bare `30` does not populate a `time.Duration`. Only values from the environment or from resolved references convert.

### Null

`null` is kept in the document. When sources are merged, `config` removes a key whose value is null, which is how an override file unsets something a base file set.

### Anchors, aliases and merge keys

Anchors (`&name`) and aliases (`*name`) are allowed, for values and for string keys. Each use of an alias is an independent copy.

Merge keys work with their usual meaning:

```yaml
base: &base {host: db.internal, port: 5432}
replica:
  <<: *base
  host: replica.internal   # explicit keys win, wherever "<<" is written
both:
  <<: [*base, *other]      # in a list, an earlier map wins over a later one
```

A merge is shallow: an explicit nested map replaces the merged one rather than combining with it. The merge value must be a mapping or a list of mappings. Two equal explicit keys are still an error, and so are two `<<` keys in one map. A quoted `"<<"` is an ordinary key.

**Expansion is bounded at 100,000 nodes.** Every node produced while expanding aliases and merge keys is counted, keys included; a document that would exceed the limit fails with `config.ErrLimitExceeded`. This is a fixed constant of the contract. It is what stops a "billion laughs" file, a few hundred bytes of nested aliases describing billions of values, and it applies equally to a document without aliases. An alias that refers back into the anchor it is part of is an error.

## Errors

Malformed input is a `*config.ConfigError` (matching `config.ErrInvalidConfig`); the size and expansion limits match `config.ErrLimitExceeded`; open, read and close failures keep their cause.

```text
config server.tls.cert: duplicate YAML key at line 5 column 5 from yaml:app.yaml
```

`Field` is the key path (or the file path when the problem is not at a key), and the reason gives the line and column. No error contains a scalar value from the file. Key names do appear, as part of the path. For a syntax error the library's message is dropped because it can quote the input, and only the line is kept; yaml.v3 reports no line for an error on the first line or for an unknown anchor, so those errors have none.

## `config/yaml` or `config/viper`

Both read YAML into `config`. Choose by what you need:

| | `config/yaml` | `config/viper` |
|---|---|---|
| Formats | YAML | YAML, JSON, TOML |
| Dependencies | yaml.v3 | Viper and its dependency tree |
| Key case | preserved | lowercased by Viper |
| Dots in keys | part of the key | path delimiters |
| Duplicate keys, custom tags, several documents | errors | Viper's native conventions |
| Numbers | exact decimal text | Viper's native types |

Use `config/yaml` for new configuration: it is strict, keeps keys as written and adds one small dependency. Use `config/viper` when you need TOML, or compatibility with files and key-casing behavior that already depend on Viper.

See [MIGRATION](MIGRATION.md), [RELEASE_NOTES](RELEASE_NOTES.md) and the independent consumers under `examples/standalone/config/yaml`. The proposed v0.1.0 is unapproved and unreleased; this is not published stability.

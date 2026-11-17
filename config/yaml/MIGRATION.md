# Migration — experimental, not released

`config/yaml` has no legacy counterpart of its own. Today YAML is read by Viper, inside `config/viper` and inside the engine's configuration loading; both stay unchanged and supported. There are no aliases or compatibility bridges, and nothing is migrated automatically.

This guide is for moving a YAML file from a Viper-based loader to `config/yaml`.

Before:

```go
cfg := vipersource.DefaultConfig()
cfg.Files = []string{"app.yaml"}
source, err := vipersource.New(cfg, vipersource.Dependencies{FS: files})
```

After:

```go
cfg := yamlsource.DefaultConfig()
cfg.Path = "app.yaml"
source, err := yamlsource.New(cfg, yamlsource.Dependencies{FS: files})
```

`config/viper` takes a list of files in one source; here each file is its own source, and `config` merges them in order. The rest of the pipeline (`config.New`, `Load`, `Decode`) is the same.

## What changes for the file

The Viper column states only what [config/viper's README](../viper/README.md) documents as its accepted native behavior. Where it says nothing, check your file against the right-hand column rather than assuming what Viper did.

| `config/viper` | `config/yaml` | What to do |
|---|---|---|
| keys become lowercase, including maps in lists | keys kept as written | make keys match what the decoder expects |
| dots in keys are path delimiters | a key containing a dot is one key | nest the maps explicitly |
| file and value conventions are Viper's own | the strict rules below | review the file against them |

Rules `config/yaml` enforces, each an error when broken:

- one document per file;
- no repeated key in a map;
- string keys only (quote `"1"` to use it as a key);
- core tags only: no application tags, `!!binary` or explicit `!!timestamp`;
- no `.inf` or `.nan`;
- at most 100,000 nodes after expanding aliases and merge keys.

And values it reads differently from a typed YAML decoder: a plain date such as `2026-01-02` stays a string, to be decoded into a `string` or a `TextUnmarshaler`; numbers are exact decimal text, so large and long values are not rounded.

`yes`/`no`/`on`/`off` are strings, as yaml.v3 follows YAML 1.2. Anchors, aliases and merge keys work, and `null` in an override removes the key when sources are merged.

## Things to check after switching

- **Key case.** This is the change most likely to surface: a file that relied on Viper lowercasing `Port` to match a `port` field needs its keys rewritten.
- **Octal-looking numbers.** `017` is 15 (yaml.v3 keeps YAML 1.1 octal). If a value such as a file mode or an identifier has a leading zero, quote it or write it as intended.
- **Quoted values.** `port: "8080"` is text, and `config` does not convert file text into numbers. This is `config`'s rule for every file source, not specific to YAML.
- **Large generated files.** A document that expands past 100,000 nodes is refused.

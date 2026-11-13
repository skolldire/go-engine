# config/viper — experimental candidate

Go 1.27.4, independent module `github.com/skolldire/go-engine/config/viper`.
Depends on config and Viper v1.21.0; core uses mapstructure/v2 v2.5.0. Versions
already selected by the repository are retained; the dependency boundary is the
`deps` column of modules.tsv (config and spf13/viper), checked by
`make check-module M=config/viper MODE=deps`. No engine/provider,
archived mapstructure, direct YAML parser, globals, automatic environment,
search paths, CONF_DIR, watcher, retry or logger.

```go
cfg := viperSource.DefaultConfig()
cfg.Files = []string{"application.yaml", "production.yaml"}
source, err := viperSource.New(cfg, viperSource.Dependencies{FS: filesystem})
```

New returns config.Source, validates/copies configuration without I/O. Files is
nonempty and fs.ValidPath; yaml/yml/json/toml extensions only. Each file is opened
from the supplied fs.FS, read with MaxBytes (default 1 MiB per file) and closed
exactly once. Viper.New/ReadConfig are local per load; MergeConfigMap applies files
in order. Required defaults true: missing files are errors; false skips only
fs.ErrNotExist. Permission, syntax, I/O and Close errors are not swallowed.
Cancellation is checked between reads/files, without abandoning a background
read goroutine. Shared immutable FS/source is safe across concurrent Loaders.
Source does not decode structs; the base Loader merges sources/resolves references
and Snapshot.Decode uses strict mapstructure v2 over seeded configuration.

## Native Viper limitations — explicitly accepted

- Keys become lowercase, including maps in lists.
- AllSettings interprets dots as path delimiters: EXAMPLE.COM can become nested
  example/com. Literal data keys are not guaranteed.
- The JSON codec reads numbers as float64: 9007199254740993 is already rounded to
  9007199254740992 before config receives it. Later decoding cannot recover it;
  strict integer decoding can also reject scientific decimal representations.
- File merge/value conventions are Viper's native semantics, matching the legacy
  reader. No claim of equivalence to config/json parsing is made.

Use config/json when literal keys, case preservation, strict JSON lexing or exact
numeric input matters. These differences are characterized by unit tests. Never
use the Viper JSON codec to validate financial integer precision.

Source.String is viper:<files>. Parser/merge errors contain filename/format but
no content; injected filesystem errors retain causes, so the caller owns their
confidentiality. Source/file names must be safe diagnostic identifiers.

## Independent validation

From repository root: make check-config-viper-quick check-config-viper-stress
check-config-viper-fuzz check-config-viper-bench check-config. The composed
consumer uses real concurrency, YAML plus env.Resolver and explicit lifecycle.
The module's local replace config=>.. is development-only; consumers explicitly
replace both selected modules. Release work must select a resolvable config tag.
E6/E7 remain deferred; this work creates no tags or stable publication.

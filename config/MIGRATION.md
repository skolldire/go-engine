# Migration to independent config

## Experimental candidate (not released)

`github.com/skolldire/go-engine/config` is an optional independent loader.
Existing engine/RawConfig/providers are unchanged; there is no compatibility
alias and no automatic migration. New applications explicitly provide FS/readers,
environment bindings and Resolver, seed their own tool defaults, Decode, Validate
and construct tools themselves.

| Legacy | Independent config |
|---|---|
| Viper file discovery/cwd/executable fallback | Explicit FS or Reader and Path; YAML/Viper adapters later |
| mapstructure tags and legacy matching, weak conversion | Same mapstructure tags/MatchName with mapstructure v2 strict hooks |
| comma-string to slice from every source | Typed lists from files; strict CSV only from env/resolved text |
| Missing section Decode no-op | ErrSectionMissing |
| os.Getenv at Decode, unresolved retained | Explicit captured lookup, resolve once per Load, missing error |
| Empty treated like absent for :- | Empty is present; fallback only for ErrReferenceNotFound |
| Raw mutable tree | Immutable snapshot, no raw getter |
| Implicit overwrite/merge behavior | Last source wins within same shape; cross-shape error, null deletes |
| Partially mutated target on error | Atomic Decode over seeded defaults |

Duplicate JSON keys and content after the root are rejected. Preserve default
values by decoding into DefaultConfig(), not a zero literal. Explicit 0/false are
not normalized. Maps merge with seeded values and lists replace. Conversion and
parser errors omit values; external dependency errors remain the caller's
confidentiality responsibility. No module is published by this extraction.

## D34 revised: mapstructure v2 and optional Viper source

The experimental config-tag decoder is replaced before release. Use
mapstructure tags (including explicit ,squash), not config tags. Untagged names
use the engine's case/underscore MatchName. config depends on mapstructure/v2
v2.5.0; Viper v1.21.0 lives only in config/viper. Neither replaces engine yet.

Before, engine loaded its own files and RawConfig.Decode performed weak decoding
and implicit environment expansion. DecodeNamed/InstanceNames remain legacy
assembly helpers rather than new config APIs. After:

```go
sourceCfg := viperSource.DefaultConfig()
sourceCfg.Files = []string{"application.yaml", "production.yaml"}
source, err := viperSource.New(sourceCfg, viperSource.Dependencies{FS: filesystem})
if err != nil { return err }
resolver, err := env.Resolver(capturedLookup)
if err != nil { return err }
loader, err := config.New(config.DefaultConfig(), config.Dependencies{
    Sources: []config.Source{source}, Resolver: resolver,
})
if err != nil { return err }
snapshot, err := loader.Load(ctx)
if err != nil { return err }
cfg := tool.DefaultConfig()
if err := snapshot.Decode("instances.orders", &cfg); err != nil { return err }
if err := cfg.Validate(); err != nil { return err }
```

The application supplies FS, explicit file paths and a captured immutable lookup
shared by env.Source and env.Resolver; it may read CONF_DIR explicitly if desired.
It owns construction and shutdown. A missing reference now fails instead of
reaching the client literally. Empty stays empty even with fallback, and embedded
references resolve. Missing sections fail while preserving defaults. File strings
such as "8080", "1" or CSV are not weakly converted; env/resolved text uses strict
formats and CSV with no empty elements. Unknown keys fail in every decoded section.
No mutable raw getter remains; no validation method is called implicitly.

config/viper intentionally keeps native lowercase/dotted-key/JSON float64
semantics. Use config/json for literal data keys and exact numeric input. Required
file handling is explicit: true requires every path, false ignores only not-exist.
The old reader may silently skip optional overlays; choose Required deliberately.
Comparative tests use a configcomparison build tag in pkg/engine without changing
its production code, imports or module requirements. Benchmarks compare the actual
legacy decoder with the new snapshot Decode using the same fixtures.

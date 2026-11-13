# config/viper candidate — not released

Proposed config/viper/v0.1.0, pending E6. Source-only module, Go 1.27.4, Viper
v1.21.0 pinned. YAML/YML/JSON/TOML from explicit FS, bounded per-file reads and
ordered merge. No decoding, process environment, globals, watchers or engine.
Native key/precision limitations are documented and characterized.

config v0.0.0 and replace => .. are development placeholders. E6 must set a
resolvable config dependency tag before publication. E7 requires external
consumers against actual tags, never local replaces. No tags are created here.

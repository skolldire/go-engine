# Changelog

## Experimental (not released)

- D55: strict YAML source for `config` over yaml.v3 v3.0.6, in its own module, with the same shape as `config/json`: `FS` or `Reader`, bounded read, `Required`, `yaml:<path>` identifier.
- Single document, string keys, duplicate keys rejected, core tags only, YAML 1.2 booleans, timestamps kept as text, exact decimal numbers with hexadecimal and octal converted, non-finite numbers rejected.
- Anchors, aliases and merge keys with expansion bounded at 100,000 nodes and recursive aliases rejected; errors with line, column and key path and no values.
- Proposed v0.1.0 unapproved; no published base. `config/viper` and the `config` core unchanged.

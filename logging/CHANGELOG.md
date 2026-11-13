# Changelog

## Experimental (not released)

- Add an independent stdlib-only slog factory with strict configuration and borrowed output.
- Apply D41 logging and D41.1–D41.8: materialized static attrs, structural redaction, cycles/depth/items/nodes, rune-safe text limits, optional Redactor and native JSON/text handling.
- Add unit/race/conformance/integration, stress, standard goroutineleak, fuzz and comparative slog benchmarks, plus two offline public-API consumers.
- Leave legacy logger and consumers unchanged. Proposed logging/v0.1.0 is not approved or tagged.

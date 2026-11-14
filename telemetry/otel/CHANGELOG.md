# Changelog

## Experimental (not released)

- Explicit native OTel v1.47.0 trace/metric providers, API interfaces, caller-owned construction and transferred processor/reader shutdown ownership (D45).
- Approved D45.3 native span-only value-length scope and revised D45.4 environment Resource merge with Config precedence; sampler/count limits remain explicit.
- Single-attempt concurrent shutdown with cached errors; no exporter, facade or automatic global installation.

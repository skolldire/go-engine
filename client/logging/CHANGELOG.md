# Changelog

## Experimental (not released)

- Add generic completion middleware for client with a borrowed two-method Logger, fixed debug/info level and optional duration (D42.1–D42.4 approved).
- Preserve context, results and error identities; omit payload/error text and abnormal completions. No retries, timeouts, backend dependency or background work.
- Add three Logger implementations in tests, isolated integration, race/stress/fuzz/leak/benchmark campaigns and independent minimal/composed consumers.
- Keep the legacy client/logger and tools unchanged. Version client/logging/v0.1.0 is a proposal only.

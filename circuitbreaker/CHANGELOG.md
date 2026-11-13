# Changelog

## Experimental (not released)

- Initial circuitbreaker/v0.1.0 candidate implementing approved D37.1–D37.5: stdlib-only typed execution, classified completions, lazy windows, generation-safe recovery, abnormal termination and counter rollover.
- Legacy production code remains unchanged. Publication/manifests and real-tag consumers deferred.

### D43.2 additive correction

- Add Run(context.Context, func(context.Context) error) error for backend-neutral composition. Execute[T] and Run share one private D37 state-machine execution path. Existing Execute validation/error identities and all generation/panic/Goexit guarantees remain unchanged.

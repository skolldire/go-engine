# Changelog

## Experimental (not released)

- D47 stdlib-only registry with explicit defaults/validation, unique bounded frozen registrations and separate readiness/diagnostic errors.
- Persistent global/per-check slots bound stuck callbacks across polls; ordered independent reports hide raw causes from JSON.
- Own PanicError{Value,Stack,Goexit}, runtime.Stack capture, fixed ErrPanic text and recover restricted to checker workers.
- Independent minimal/logging consumers; legacy production/endpoints/drivers unchanged.

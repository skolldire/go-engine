# Changelog

## Experimental (not released)

- Initial resilience/v0.1.0 candidate implements approved D39.1–D39.5, closing D32 with total → retry → rate → concurrency → breaker → attempt timeout → I/O.
- Typed pure Wrap, explicit Config policy, caller-owned resources, cause-aware budgets and local-rejection Stop precedence. Legacy production unchanged. Distribution remains deferred.

### D43.1 approved correction

- Replace concrete breaker/rate/concurrency dependencies with consumer-side Run/Wait/Acquire interfaces; module dependencies are client,retry only.
- Add ErrRejected with original rejection causes; local rejection Stops before caller classification. Reject typed nil pointers at construction and tolerate nil release.
- Preserve concrete integrations, stress, fuzz inputs and benchmarks in the nonpublished resilience/integration harness; new local-interface tests validate independent composition.

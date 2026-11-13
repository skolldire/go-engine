# Changelog

## Experimental (not released)

- Initial resilience/v0.1.0 candidate implements approved D39.1–D39.5, closing D32 with total → retry → rate → concurrency → breaker → attempt timeout → I/O.
- Typed pure Wrap, explicit Config policy, caller-owned resources, cause-aware budgets and local-rejection Stop precedence. Legacy production unchanged. Distribution remains deferred.

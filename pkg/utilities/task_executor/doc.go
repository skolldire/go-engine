// Package task_executor provides a bounded-concurrency worker pool with priorities, timeouts and panic containment.
//
// Deprecated: use github.com/skolldire/go-engine/concurrency for bounded
// cooperative execution. This legacy executor can return on timeout while a
// task continues running; it is frozen and will be removed in the migration
// release. The new pool does not promise equivalent priority/timeout semantics.
package task_executor

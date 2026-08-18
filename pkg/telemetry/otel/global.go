package otel

import (
	"fmt"
	"sync"
)

// The OpenTelemetry globals are a process-wide resource with exactly one owner.
// Documenting that was not enough: a second provider still overwrote the first
// silently, and the losing engine's spans disappeared with no error anywhere.
//
// Claiming the globals makes the conflict an explicit failure at construction,
// where it can be acted on, instead of missing telemetry discovered later.
var (
	globalMu    sync.Mutex
	globalOwner string
	globalTaken bool
)

// claimGlobalProviders records this provider as the owner of the process-wide
// OpenTelemetry state, failing if another one already holds it.
func claimGlobalProviders(service string) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalTaken {
		return fmt.Errorf(
			"the OpenTelemetry global providers are already installed by %q: "+
				"they are process-wide and installing them twice makes the first "+
				"provider's spans disappear silently — set skip_global_providers "+
				"on every telemetry provider but one",
			globalOwner)
	}

	globalTaken = true
	globalOwner = service
	if globalOwner == "" {
		globalOwner = "an unnamed service"
	}

	return nil
}

// releaseGlobalProviders gives the globals back, so a provider that is shut
// down does not keep the process locked out for its remaining lifetime.
func releaseGlobalProviders() {
	globalMu.Lock()
	defer globalMu.Unlock()

	globalTaken = false
	globalOwner = ""
}

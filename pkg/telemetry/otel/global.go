package otel

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// The OpenTelemetry globals are a process-wide resource with exactly one owner.
// Documenting that was not enough: a second provider still overwrote the first
// silently, and the losing engine's spans disappeared with no error anywhere.
//
// Ownership is handed out as a token rather than a boolean. A plain flag let a
// provider release a claim it no longer held: shutting A down twice freed the
// globals that B had taken in between, so B kept exporting while the process
// believed the slot was free and a third provider could install over it.
var (
	globalMu     sync.Mutex
	globalToken  uint64
	globalOwner  string
	globalTaken  bool
	globalSerial atomic.Uint64
)

// globalClaim identifies one successful acquisition of the process-wide state.
// The zero value owns nothing.
type globalClaim struct {
	token uint64
}

// held reports whether this claim represents a real acquisition.
func (c globalClaim) held() bool { return c.token != 0 }

// claimGlobalProviders records this provider as the owner of the process-wide
// OpenTelemetry state, failing if another one already holds it.
func claimGlobalProviders(service string) (globalClaim, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalTaken {
		return globalClaim{}, fmt.Errorf(
			"the OpenTelemetry global providers are already installed by %q: "+
				"they are process-wide and installing them twice makes the first "+
				"provider's spans disappear silently — set skip_global_providers "+
				"on every telemetry provider but one",
			globalOwner)
	}

	token := globalSerial.Add(1)

	globalTaken = true
	globalToken = token
	globalOwner = service
	if globalOwner == "" {
		globalOwner = "an unnamed service"
	}

	return globalClaim{token: token}, nil
}

// releaseGlobalProviders gives the globals back, but only if this claim is
// still the one that holds them.
//
// That check is the point: a repeated Shutdown must be a no-op rather than
// freeing whatever provider happens to own the globals by then.
func releaseGlobalProviders(c globalClaim) {
	if !c.held() {
		return
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	if !globalTaken || globalToken != c.token {
		return // someone else owns them now; not ours to release
	}

	globalTaken = false
	globalToken = 0
	globalOwner = ""
}

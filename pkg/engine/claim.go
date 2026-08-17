package engine

import (
	"fmt"
	"sync"
)

// claimed tracks which Provider values have already been bound to an engine.
//
// Providers are stateful by design: Init stores the client it built so Close
// can release it. That makes a Provider single-use. Rather than document the
// rule and hope, the core enforces it.
var claimed sync.Map // map[Provider]string

// claimProvider binds p to an engine, failing if it is already bound.
func claimProvider(p Provider, name string) error {
	if prev, loaded := claimed.LoadOrStore(p, name); loaded {
		return fmt.Errorf(
			"provider %q was already used by another engine (as %q): a Provider holds the "+
				"component it builds, so construct a new one per engine", name, prev)
	}
	return nil
}

// releaseProvider undoes a claim, so a rolled-back build does not leave the
// provider permanently unusable.
func releaseProvider(p Provider) {
	claimed.Delete(p)
}

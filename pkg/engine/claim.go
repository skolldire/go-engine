package engine

import (
	"fmt"
	"reflect"
	"sync"
)

// claimed tracks which Provider values have already been bound to an engine.
//
// Providers are stateful by design: Init stores the client it built so Close
// can release it. That makes a Provider single-use, and the core enforces it
// rather than documenting the rule and hoping.
var claimed sync.Map // map[Provider]string

// claimable reports whether p can be used as a map key.
//
// A Provider is free to be a struct value containing a slice or a map, which is
// perfectly legal Go and satisfies the interface — but is not hashable, and
// using it as a sync.Map key panics with "hash of unhashable type". Guarding
// here trades a crash for a lost check on a rare shape: nearly every provider
// is a pointer, which is always comparable.
func claimable(p Provider) bool {
	v := reflect.ValueOf(p)
	if !v.IsValid() {
		return false
	}
	return v.Type().Comparable()
}

// claimProvider binds p to an engine, failing if it is already bound.
func claimProvider(p Provider, name string) error {
	if !claimable(p) {
		// Not hashable: skip the single-use check rather than crash. Such a
		// provider carries the same reuse hazard, so it should still be
		// constructed fresh per engine.
		return nil
	}
	if prev, loaded := claimed.LoadOrStore(p, name); loaded {
		return fmt.Errorf(
			"provider %q was already used by another engine (as %q): a Provider holds the "+
				"component it builds, so construct a new one per engine", name, prev)
	}
	return nil
}

// releaseProvider undoes a claim, so a closed engine's providers can be reused
// and a rolled-back build does not leave them permanently unusable.
func releaseProvider(p Provider) {
	if !claimable(p) {
		return
	}
	claimed.Delete(p)
}

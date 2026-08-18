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

// claimable reports whether p is a provider the single-use check applies to.
//
// Only pointers are tracked, and that is not a limitation — it is the exact set
// of providers that can alias:
//
//   - A pointer provider is shared. Two engines Init-ing it would have the
//     second overwrite the first's stored client, and the first engine's Close
//     would then release a resource it no longer owns. This is the hazard.
//   - A value provider is copied into the interface, so each engine gets its
//     own. It cannot alias, and keying a map by its value would instead produce
//     false positives: two independently constructed but equal providers would
//     look like reuse.
//   - A value provider carrying a slice or map is not even hashable, and using
//     it as a sync.Map key panics with "hash of unhashable type".
//
// So tracking pointers covers every case that needs covering, and skips the
// cases where tracking would be wrong or fatal.
func claimable(p Provider) bool {
	v := reflect.ValueOf(p)
	return v.IsValid() && v.Kind() == reflect.Pointer
}

// claimProvider binds p to an engine, failing if it is already bound.
func claimProvider(p Provider, name string) error {
	if !claimable(p) {
		// A non-pointer provider is copied per engine, so there is nothing to
		// alias and nothing to claim.
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

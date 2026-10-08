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

// requirePointerProvider rejects a Provider that is not a pointer.
//
// A value provider is not merely unusual, it is broken twice over:
//
//   - Init runs on a copy, so the client it stores is discarded and the Close
//     that should release it sees a zero value.
//   - Copying does not isolate it. A value provider holding a pointer, slice or
//     map shares that state with every copy, so two engines alias each other
//     and the first one's Close acts on what the second built. That is a real
//     failure, not a theoretical one — it was reproduced before this check
//     existed.
//
// Requiring a pointer removes both problems and makes the single-use claim
// enforceable, since pointers are always comparable.
// It takes no name: it must run before any method is called on the provider,
// because calling Name on a nil pointer inside a non-nil interface panics.
func requirePointerProvider(p Provider) error {
	v := reflect.ValueOf(p)

	if !v.IsValid() {
		return fmt.Errorf("provider is nil")
	}
	if v.Kind() != reflect.Pointer {
		return fmt.Errorf(
			"provider %T is not a pointer: Init would run on a copy and its "+
				"state would be discarded, and any pointer, slice or map it holds "+
				"would be shared with every other engine that used it — "+
				"return &%T{...} from your constructor", p, p)
	}
	if v.IsNil() {
		return fmt.Errorf("provider is a nil %T", p)
	}

	return nil
}

// claimProvider binds p to an engine, failing if it is already bound.
func claimProvider(p Provider, name string) error {
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
	if v := reflect.ValueOf(p); !v.IsValid() || v.Kind() != reflect.Pointer {
		return
	}
	claimed.Delete(p)
}

package engine

import (
	"fmt"
	"strings"

	"github.com/mitchellh/mapstructure"
)

// rawSection is the core's RawConfig implementation: a value lifted verbatim
// out of the configuration file, decoded on demand by whoever owns its type.
type rawSection struct {
	key    string
	value  any
	exists bool
}

var _ RawConfig = (*rawSection)(nil)

func (r *rawSection) Raw() any     { return r.value }
func (r *rawSection) Key() string  { return r.key }
func (r *rawSection) Exists() bool { return r.exists }

// Decode maps the section onto target. Environment placeholders such as
// ${VAR} or ${VAR:-default} are resolved during decoding, matching the
// behaviour of the typed configuration loader.
func (r *rawSection) Decode(target any) error {
	if target == nil {
		return fmt.Errorf("decode %q: target must be a non-nil pointer", r.key)
	}
	if !r.exists {
		// Nothing to decode: leave target at its zero value so providers can
		// treat "absent" and "empty" identically.
		return nil
	}

	// Collect metadata so a key that matched no field is reported rather than
	// dropped. A typo such as `endpont:` used to leave the field at its zero
	// value and start the service against the wrong target, with nothing in the
	// logs to explain it.
	var md mapstructure.Metadata
	decoder, err := newDecoderWithMetadata(target, &md)
	if err != nil {
		return fmt.Errorf("decode %q: %w", r.key, err)
	}
	if err := decoder.Decode(r.value); err != nil {
		return fmt.Errorf("decode %q: %w", r.key, err)
	}

	if len(md.Unused) > 0 {
		sortStrings(md.Unused)
		return fmt.Errorf("decode %q: unknown configuration %s %s",
			r.key, pluralKeys(len(md.Unused)), strings.Join(md.Unused, ", "))
	}
	return nil
}

// pluralKeys keeps the error message grammatical for one or many keys.
func pluralKeys(n int) string {
	if n == 1 {
		return "key"
	}
	return "keys"
}

// NewRawConfig builds a RawConfig from an already-parsed value. It is exported
// for provider tests, which need to drive Init without a configuration file.
func NewRawConfig(key string, value any) RawConfig {
	return &rawSection{key: key, value: value, exists: value != nil}
}

// NewMissingRawConfig builds a RawConfig representing an absent section.
func NewMissingRawConfig(key string) RawConfig {
	return &rawSection{key: key, exists: false}
}

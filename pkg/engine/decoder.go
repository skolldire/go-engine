package engine

import (
	"strings"

	"github.com/mitchellh/mapstructure"
)

// newDecoder builds the one decoder configuration used everywhere in the
// engine: for the core sections, and for every section a provider decodes.
//
// Having a single definition matters. The core loader and the per-provider
// decoder must agree on how a duration, a comma-separated list and a key name
// are interpreted, or the same YAML would mean different things depending on
// who read it.
func newDecoder(result any) (*mapstructure.Decoder, error) {
	return mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           result,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			// Durations are written as "10s" throughout the configuration
			// (router.read_timeout, health.timeout, grpc.timeout, ...). Without
			// this hook they fail to decode as int.
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			EnvVarDecodeHook(),
		),
		MatchName: matchName,
	})
}

// matchName binds snake_case YAML keys to Go field names even when a struct
// carries no mapstructure tag, matching the behaviour of the loader this engine
// replaces. Dropping it would silently stop binding configuration for any
// untagged struct a consumer passes in.
func matchName(mapKey, fieldName string) bool {
	if strings.EqualFold(mapKey, fieldName) {
		return true
	}
	return strings.EqualFold(strings.ReplaceAll(mapKey, "_", ""), strings.ToLower(fieldName))
}

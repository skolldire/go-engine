package engine

import (
	"fmt"
	"reflect"
	"strings"
	"time"

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
	return newDecoderWithMetadata(result, nil)
}

// newDecoderWithMetadata is newDecoder with mapstructure metadata collection,
// used to report configuration keys that matched no field.
func newDecoderWithMetadata(result any, md *mapstructure.Metadata) (*mapstructure.Decoder, error) {
	return mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Metadata:         md,
		Result:           result,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			// Rejecting a bare number for a duration comes first: with weakly
			// typed input `read_timeout: 10` silently means 10 nanoseconds, so
			// the server appears configured while every request times out
			// instantly. Documentation alone cannot fix that.
			rejectBareDurationHook(),
			// Durations are written as "10s" throughout the configuration
			// (router.read_timeout, health.timeout, grpc.timeout, ...). Without
			// this hook they fail to decode as int.
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			envVarDecodeHook(),
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

// rejectBareDurationHook fails when a numeric literal is assigned to a
// time.Duration field, pointing at the form that actually works.
func rejectBareDurationHook() mapstructure.DecodeHookFuncType {
	durationType := reflect.TypeOf(time.Duration(0))

	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if to != durationType {
			return data, nil
		}
		switch from.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return nil, fmt.Errorf(
				"%v is not a duration: a bare number means nanoseconds, write it as a "+
					"duration string such as \"10s\", \"500ms\" or \"1m\"", data)
		default:
			return data, nil
		}
	}
}

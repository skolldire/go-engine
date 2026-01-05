package helpers

// GetOrDefault returns the value for the given key, or defaultValue if the key doesn't exist.
// This avoids the common pattern of checking if a key exists before accessing it.
//
// Example:
//
//	m := map[string]int{"a": 1, "b": 2}
//	value := GetOrDefault(m, "a", 0)  // returns 1
// GetOrDefault returns the value associated with key in m or defaultValue when key is not present.
func GetOrDefault[K comparable, V any](m map[K]V, key K, defaultValue V) V {
	if val, ok := m[key]; ok {
		return val
	}
	return defaultValue
}

// HasKey checks if a map contains the specified key.
// This is a more readable alternative to the `val, ok := m[key]` pattern when you only need to check existence.
//
// Example:
//
//	m := map[string]int{"a": 1}
//	HasKey(m, "a")  // returns true
// HasKey reports whether the given key is present in m.
// A nil map is treated as empty and will always report false.
func HasKey[K comparable, V any](m map[K]V, key K) bool {
	_, ok := m[key]
	return ok
}

// Keys returns all keys from the map as a slice.
//
// Example:
//
//	m := map[string]int{"a": 1, "b": 2}
//	keys := Keys(m)
// Keys returns a slice containing all keys present in the map.
// The order of keys is not specified and may vary between calls.
// The returned slice has length equal to the number of entries in the map.
func Keys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Values returns all values from the map as a slice.
//
// Example:
//
//	m := map[string]int{"a": 1, "b": 2}
//	values := Values(m)
// Values extracts all values from the provided map into a slice.
// The order of values in the returned slice is unspecified and may vary between iterations.
func Values[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// Merge combines multiple maps into a single map.
// Later maps override earlier maps for duplicate keys.
//
// Example:
//
//	m1 := map[string]int{"a": 1, "b": 2}
//	m2 := map[string]int{"b": 3, "c": 4}
//	result := Merge(m1, m2)
// Merge merges the provided maps into a newly allocated map.
// When the same key appears in multiple input maps, the value from the later map in the argument list overrides earlier ones.
func Merge[K comparable, V any](maps ...map[K]V) map[K]V {
	result := make(map[K]V)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// NewStringMap creates a new map[string]string.
// This is a more readable alternative to make(map[string]string).
//
// Example:
//
//	m := NewStringMap()
// NewStringMap creates and returns a new, empty map[string]string.
func NewStringMap() map[string]string {
	return make(map[string]string)
}

// NewStringInterfaceMap creates a new map[string]interface{}.
// This is a more readable alternative to make(map[string]interface{}).
//
// Example:
//
//	m := NewStringInterfaceMap()
//	m["key"] = "value"
// NewStringInterfaceMap creates and returns a new empty map[string]interface{}.
func NewStringInterfaceMap() map[string]interface{} {
	return make(map[string]interface{})
}

// NewMap creates a new map with the specified key and value types.
// This is a generic helper for map initialization.
//
// Example:
//
//	m := NewMap[string, int]()
// NewMap creates a new empty map with the specified key and value types.
func NewMap[K comparable, V any]() map[K]V {
	return make(map[K]V)
}



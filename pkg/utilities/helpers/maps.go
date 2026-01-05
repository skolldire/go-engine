package helpers

// GetOrDefault returns the value for the given key, or defaultValue if the key doesn't exist.
// This avoids the common pattern of checking if a key exists before accessing it.
//
// Example:
//
//	m := map[string]int{"a": 1, "b": 2}
//	value := GetOrDefault(m, "a", 0)  // returns 1
//	value := GetOrDefault(m, "c", 0)  // returns 0
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
//	HasKey(m, "b")  // returns false
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
//	// returns []string{"a", "b"} (order may vary)
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
//	// returns []int{1, 2} (order may vary)
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
//	// returns map[string]int{"a": 1, "b": 3, "c": 4}
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
//	m["key"] = "value"
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
//	m["number"] = 42
func NewStringInterfaceMap() map[string]interface{} {
	return make(map[string]interface{})
}

// NewMap creates a new map with the specified key and value types.
// This is a generic helper for map initialization.
//
// Example:
//
//	m := NewMap[string, int]()
//	m["key"] = 42
func NewMap[K comparable, V any]() map[K]V {
	return make(map[K]V)
}




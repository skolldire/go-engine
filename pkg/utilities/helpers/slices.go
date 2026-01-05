package helpers

// Contains checks if a slice contains the specified value.
// This avoids writing manual loops for simple membership checks.
//
// Example:
//
//	Contains([]int{1, 2, 3}, 2)  // returns true
//	Contains([]int{1, 2, 3}, 4)  // returns false
// Contains reports whether value is present in slice.
// It returns true if an element equal to value is found, false otherwise.
func Contains[T comparable](slice []T, value T) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// Filter creates a new slice containing only elements that satisfy the predicate.
//
// Example:
//
//	numbers := []int{1, 2, 3, 4, 5}
//	evens := Filter(numbers, func(n int) bool { return n%2 == 0 })
// Filter returns a new slice containing the elements of slice that satisfy predicate,
// preserving their original order. If no elements satisfy predicate the returned slice is nil.
func Filter[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// Map creates a new slice by applying the mapper function to each element.
//
// Example:
//
//	numbers := []int{1, 2, 3}
//	doubled := Map(numbers, func(n int) int { return n * 2 })
//	// returns []int{2, 4, 6}
//
//	names := []string{"alice", "bob"}
//	upper := Map(names, func(s string) string { return strings.ToUpper(s) })
// Map applies mapper to each element of slice and returns a new slice containing the mapped results in the same order.
// The returned slice has the same length as the input; each result is placed at the corresponding index.
func Map[T, U any](slice []T, mapper func(T) U) []U {
	result := make([]U, len(slice))
	for i, v := range slice {
		result[i] = mapper(v)
	}
	return result
}

// Find returns the first element that satisfies the predicate, along with a boolean
// indicating if an element was found.
//
// Example:
//
//	numbers := []int{1, 2, 3, 4, 5}
//	value, found := Find(numbers, func(n int) bool { return n > 3 })
//	// value = 4, found = true
//
//	value, found := Find(numbers, func(n int) bool { return n > 10 })
// Find returns the first element in slice that satisfies the predicate.
// If no element satisfies the predicate it returns the zero value of T and false.
func Find[T any](slice []T, predicate func(T) bool) (T, bool) {
	var zero T
	for _, v := range slice {
		if predicate(v) {
			return v, true
		}
	}
	return zero, false
}

// Concat concatenates multiple slices into a single slice.
//
// Example:
//
//	slice1 := []int{1, 2}
//	slice2 := []int{3, 4}
//	result := Concat(slice1, slice2)
// Concat concatenates the provided slices into a single slice, preserving the order of elements.
// The returned slice contains all elements from the first slice, then the second, and so on.
func Concat[T any](slices ...[]T) []T {
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	result := make([]T, 0, totalLen)
	for _, s := range slices {
		result = append(result, s...)
	}
	return result
}

// First returns the first element of a slice and a boolean indicating if the slice is not empty.
//
// Example:
//
//	numbers := []int{1, 2, 3}
//	value, ok := First(numbers)
//	// value = 1, ok = true
//
//	var empty []int
//	value, ok := First(empty)
// First returns the first element of the provided slice and true if the slice is non-empty.
// If the slice is empty it returns the zero value of T and false.
func First[T any](slice []T) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	return slice[0], true
}

// Last returns the last element of a slice and a boolean indicating if the slice is not empty.
//
// Example:
//
//	numbers := []int{1, 2, 3}
//	value, ok := Last(numbers)
//	// value = 3, ok = true
//
//	var empty []int
//	value, ok := Last(empty)
// Last returns the last element of the slice and a boolean indicating whether the slice is non-empty.
func Last[T any](slice []T) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	return slice[len(slice)-1], true
}

// FirstOrDefault returns the first element of a slice, or defaultValue if the slice is empty.
//
// Example:
//
//	numbers := []int{1, 2, 3}
//	value := FirstOrDefault(numbers, 0)
//	// returns 1
//
//	var empty []int
//	value := FirstOrDefault(empty, 0)
// FirstOrDefault returns the first element of slice; if slice is empty it returns defaultValue.
func FirstOrDefault[T any](slice []T, defaultValue T) T {
	if len(slice) == 0 {
		return defaultValue
	}
	return slice[0]
}



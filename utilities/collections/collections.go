package collections

func Keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
func Values[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
func Merge[K comparable, V any](maps ...map[K]V) map[K]V {
	out := make(map[K]V)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
func Filter[T any](s []T, keep func(T) bool) []T {
	if s == nil {
		return nil
	}
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
func Map[T, U any](s []T, f func(T) U) []U {
	if s == nil {
		return nil
	}
	out := make([]U, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}
func Find[T any](s []T, match func(T) bool) (T, bool) {
	var out T
	found := false
	for _, v := range s {
		yes := match(v)
		if yes && !found {
			out = v
			found = true
		}
	}
	return out, found
}
func First[T any](s []T) (T, bool) {
	if len(s) > 0 {
		return s[0], true
	}
	var zero T
	return zero, false
}
func Last[T any](s []T) (T, bool) {
	if len(s) > 0 {
		return s[len(s)-1], true
	}
	var zero T
	return zero, false
}

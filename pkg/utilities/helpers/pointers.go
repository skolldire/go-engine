package helpers

func ValueOrZero[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

func Ptr[T any](v T) *T {
	return &v
}

func IsNilOrZero[T comparable](ptr *T) bool {
	if ptr == nil {
		return true
	}
	var zero T
	return *ptr == zero
}

func ValueOrError[T any](ptr *T, err error) (T, error) {
	if ptr == nil {
		var zero T
		return zero, err
	}
	return *ptr, nil
}

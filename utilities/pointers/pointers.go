package pointers

func Ptr[T any](v T) *T { return &v }
func ValueOrZero[T any](p *T) T {
	if p != nil {
		return *p
	}
	var zero T
	return zero
}
func FirstNonNil[T any](ptrs ...*T) *T {
	for _, p := range ptrs {
		if p != nil {
			return p
		}
	}
	return nil
}

package pointers

import "testing"

func TestPointers(t *testing.T) {
	v := 42
	p := Ptr(v)
	*p = 9
	if v != 42 || ValueOrZero(p) != 9 || ValueOrZero[int](nil) != 0 {
		t.Fatal("value copy/zero")
	}
	q := Ptr(7)
	if FirstNonNil(nil, p, q) != p || FirstNonNil[int]() != nil || FirstNonNil[int](nil, nil) != nil {
		t.Fatal("first pointer identity")
	}
	m := map[string]int{"a": 1}
	if ValueOrZero(Ptr(m))["a"] != 1 {
		t.Fatal("generic value")
	}
}
func FuzzPointers(f *testing.F) {
	f.Add(int64(42))
	f.Fuzz(func(t *testing.T, v int64) {
		p := Ptr(v)
		if p == nil || ValueOrZero(p) != v || FirstNonNil(nil, p) != p {
			t.Fatal("pointer invariant")
		}
	})
}
func BenchmarkValueOrZero(b *testing.B) {
	p := Ptr(42)
	b.ReportAllocs()
	for b.Loop() {
		_ = ValueOrZero(p)
	}
}

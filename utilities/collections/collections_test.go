package collections

import (
	"maps"
	"reflect"
	"slices"
	"testing"
)

func TestMapsAndOuterCopies(t *testing.T) {
	a := map[string]int{"a": 1, "b": 2}
	b := map[string]int{"b": 3}
	merged := Merge(a, nil, b)
	if !maps.Equal(merged, map[string]int{"a": 1, "b": 3}) {
		t.Fatal(merged)
	}
	merged["a"] = 9
	if a["a"] != 1 || b["b"] != 3 {
		t.Fatal("input mutated")
	}
	if Merge[string, int]() == nil {
		t.Fatal("nil merge")
	}
	keys := Keys(a)
	values := Values(a)
	slices.Sort(keys)
	slices.Sort(values)
	if !slices.Equal(keys, []string{"a", "b"}) || !slices.Equal(values, []int{1, 2}) {
		t.Fatal("map contents")
	}
	if len(Keys(map[string]int(nil))) != 0 || len(Values(map[string]int(nil))) != 0 {
		t.Fatal("nil maps")
	}
}
func TestSlicesCallbacksAndNil(t *testing.T) {
	input := []int{1, 2, 3}
	var order []int
	got := Filter(input, func(v int) bool { order = append(order, v); return v%2 == 1 })
	if !slices.Equal(got, []int{1, 3}) || !slices.Equal(order, input) {
		t.Fatal("filter/order")
	}
	got[0] = 9
	if input[0] != 1 {
		t.Fatal("filter aliases input")
	}
	order = nil
	mapped := Map(input, func(v int) string { order = append(order, v); return string(rune('0' + v)) })
	if !slices.Equal(mapped, []string{"1", "2", "3"}) || !slices.Equal(order, input) {
		t.Fatal("map/order")
	}
	order = nil
	v, ok := Find(input, func(v int) bool { order = append(order, v); return v >= 2 })
	if !ok || v != 2 || !slices.Equal(order, input) {
		t.Fatal("find must visit each element exactly once")
	}
	if Filter[int](nil, nil) != nil || Map[int, int](nil, nil) != nil {
		t.Fatal("nil contract")
	}
	empty := []int{}
	if Filter(empty, func(int) bool { return false }) == nil || Map(empty, func(v int) int { return v }) == nil {
		t.Fatal("non-nil empty output")
	}
	if v, ok := Find[int](nil, nil); v != 0 || ok {
		t.Fatal("find nil")
	}
	if v, ok := First(input); v != 1 || !ok {
		t.Fatal("first")
	}
	if v, ok := Last(input); v != 3 || !ok {
		t.Fatal("last")
	}
	if v, ok := First[int](nil); v != 0 || ok {
		t.Fatal("first empty")
	}
	if v, ok := Last(empty); v != 0 || ok {
		t.Fatal("last empty")
	}
	if _, ok := Find(input, func(int) bool { return false }); ok {
		t.Fatal("unexpected match")
	}
}
func TestCallbackPanicsPropagate(t *testing.T) {
	token := &struct{}{}
	for _, call := range []func(){func() { Filter([]int{1}, func(int) bool { panic(token) }) }, func() { Map([]int{1}, func(int) int { panic(token) }) }, func() {
		Find([]int{1, 2}, func(v int) bool {
			if v == 2 {
				panic(token)
			}
			return true
		})
	}} {
		func() {
			defer func() {
				if recover() != token {
					t.Error("panic changed")
				}
			}()
			call()
		}()
	}
}
func FuzzCollections(f *testing.F) {
	f.Add([]byte{1, 2, 3}, byte(2))
	f.Fuzz(func(t *testing.T, data []byte, target byte) {
		if len(data) > 4096 {
			t.Skip()
		}
		before := slices.Clone(data)
		out := Map(data, func(v byte) byte { return v })
		filtered := Filter(data, func(v byte) bool { return v == target })
		v, ok := Find(data, func(v byte) bool { return v == target })
		if !slices.Equal(data, before) || !slices.Equal(out, data) || ok != slices.Contains(data, target) || (ok && v != target) {
			t.Fatal("collection invariant")
		}
		for _, value := range filtered {
			if value != target {
				t.Fatal("filter")
			}
		}
		if len(out) > 0 {
			out[0]++
			if !reflect.DeepEqual(data, before) {
				t.Fatal("copy alias")
			}
		}
	})
}
func BenchmarkMap(b *testing.B) {
	input := make([]int, 128)
	b.ReportAllocs()
	for b.Loop() {
		_ = Map(input, func(v int) int { return v + 1 })
	}
}
func BenchmarkStdlibKeys(b *testing.B) {
	m := map[int]int{1: 1, 2: 2, 3: 3}
	b.ReportAllocs()
	for b.Loop() {
		_ = slices.Collect(maps.Keys(m))
	}
}
func BenchmarkKeys(b *testing.B) {
	m := map[int]int{1: 1, 2: 2, 3: 3}
	b.ReportAllocs()
	for b.Loop() {
		_ = Keys(m)
	}
}

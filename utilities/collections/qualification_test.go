package collections

import (
	"bytes"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestStressPureCollections(t *testing.T) {
	if os.Getenv("UTILITIES_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 2000 {
				input := []int{1, 2, 3, 4}
				out := Map(Filter(input, func(v int) bool { return v%2 == 0 }), func(v int) int { return v * 2 })
				if !slices.Equal(out, []int{4, 8}) || !slices.Equal(input, []int{1, 2, 3, 4}) {
					t.Error("pure operations")
				}
				a := map[int]int{1: 1}
				b := Merge(a, map[int]int{1: 2})
				b[1] = 3
				if a[1] != 1 {
					t.Error("input mutation")
				}
			}
		})
	}
	wg.Wait()
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		t.Fatal("standard profile missing")
	}
	var report bytes.Buffer
	if err := profile.WriteTo(&report, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "total 0") {
		t.Fatal(report.String())
	}
	t.Log(report.String())
}
func TestStressReleasedResults(t *testing.T) {
	if os.Getenv("UTILITIES_STRESS") != "1" {
		t.Skip("expanded campaign")
	}
	batch := func() {
		for range 1000 {
			input := make([]int, 1024)
			_ = Map(input, func(v int) int { return v + 1 })
			_ = Merge(map[int]int{1: 1}, map[int]int{2: 2})
		}
	}
	batch()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 3 {
		batch()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > before.HeapAlloc+4<<20 {
		t.Fatalf("fixture retention before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
	}
	t.Logf("4000 1024-int fixture batches, before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
}

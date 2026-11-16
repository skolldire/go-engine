package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/skolldire/go-engine/utilities/collections"
	"github.com/skolldire/go-engine/utilities/pointers"
	"github.com/skolldire/go-engine/utilities/profile"
	"github.com/skolldire/go-engine/utilities/text"
)

func run() error {
	p, err := profile.ParseScope(" API-PROD ")
	if err != nil || !p.IsProd() {
		return fmt.Errorf("profile mismatch")
	}
	if _, err = profile.ParseScope("contest"); err == nil {
		return fmt.Errorf("unknown profile accepted")
	}
	if text.Truncate("界😊abcd", 4) != "界..." || text.CleanSpaces(" a\u2003b ") != "a b" {
		return fmt.Errorf("text mismatch")
	}
	input := []int{1, 2, 3}
	got := collections.Map(collections.Filter(input, func(v int) bool { return v > 1 }), func(v int) int { return v * 2 })
	if !slices.Equal(got, []int{4, 6}) || pointers.ValueOrZero(pointers.FirstNonNil[int](nil, pointers.Ptr(42))) != 42 || cmp.Or(0, 42) != 42 {
		return fmt.Errorf("helper mismatch")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("utilities minimal: explicit profile, text, collections and pointers verified")
}

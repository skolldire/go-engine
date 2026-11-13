package main

import "testing"

func TestStandalone(t *testing.T) {
	if err := verify(); err != nil {
		t.Fatal(err)
	}
}

package main

import "testing"

func TestIndependent(t *testing.T) {
	if err := verify(); err != nil {
		t.Fatal(err)
	}
}

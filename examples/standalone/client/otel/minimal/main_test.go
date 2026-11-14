package main

import "testing"

func TestStandalone(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}

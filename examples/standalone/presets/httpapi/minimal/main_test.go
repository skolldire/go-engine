package main

import "testing"

func TestStandalone(t *testing.T) {
	if e := run(); e != nil {
		t.Fatal(e)
	}
}

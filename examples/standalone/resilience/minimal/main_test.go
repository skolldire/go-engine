package main

import "testing"

func TestConsumer(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}

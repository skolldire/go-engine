package main

import (
	"testing"
	"testing/synctest"
)

func TestStandalone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		if e := run(); e != nil {
			t.Fatal(e)
		}
	})
}

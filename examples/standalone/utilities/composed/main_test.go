package main

import (
	"errors"
	"github.com/skolldire/go-engine/utilities/profile"
	"testing"
)

func TestStandalone(t *testing.T) {
	for _, scope := range []string{"", "dev-local", "api-test", "api-stage", " API-PROD "} {
		if err := run(scope); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(run("contest"), profile.ErrUnknownProfile) {
		t.Fatal("unknown profile accepted")
	}
}

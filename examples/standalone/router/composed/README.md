# router composed

In-memory local integration; borrowed resources are explicitly assembled. Local replaces point only to the selected modules above this directory. No engine or framework.

Run with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly: `go run .` and `go test -race ./...`. See go.mod for the measured direct module selection.

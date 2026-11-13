# Ratelimit composed

Run `GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run .` and `go test -race .` with that environment. Replaces point only to ratelimit, client, retry, circuitbreaker four directories above. v0.0.0 is a local placeholder, not a tag test. Root `make check-ratelimit` validates module/package/graph/replace allowlists offline. Timed behavior is checked with synctest in tests, with no real precision sleeps, services or sockets.

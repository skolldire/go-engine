# Circuitbreaker minimal

Run `GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run .` and `go test -race .` with the same environment. Local replaces only target circuitbreaker four directories above. v0.0.0 is a local placeholder, not a real-tag validation. Root `make check-circuitbreaker` checks package/module/graph/replace allowlists and executes offline. Recovery timing uses synctest in tests, without real sleeps, sockets or services.

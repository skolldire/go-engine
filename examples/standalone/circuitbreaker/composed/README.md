# Circuitbreaker composed

Run `GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run .` and `go test -race .` with the same environment. Local replaces only target circuitbreaker, client, retry four directories above. v0.0.0 is a local placeholder, not a real-tag validation. Root `make check-circuitbreaker` checks the dependency boundary declared in modules.tsv and executes offline. Recovery timing uses synctest in tests, without real sleeps, sockets or services.

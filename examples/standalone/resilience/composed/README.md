# Resilience composed

Run `GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run .` and `go test -race .` with the same environment. Replaces point only to resilience and its five declared base modules, four directories above. v0.0.0 placeholders are not published tags. Both consumers resolve six library modules plus stdlib, including minimal with no active resources. Root `make check-resilience` enforces package/module/graph/replace allowlists and executes offline. Temporal tests use synctest; no services/sockets/framework/config.

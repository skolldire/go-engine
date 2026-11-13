# Resilience minimal

Run GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run . and go test -race . with the same environment. Replaces point only to resilience, client and retry, four directories above. D43 removes obsolete concrete-tool requirements/replaces: minimal resolves three library modules plus stdlib, not circuitbreaker/concurrency/ratelimit. v0.0.0 placeholders are not published tags. Composed still explicitly selects its concrete tools. Root make check-resilience enforces the catalog-derived boundary and executes offline. No services/sockets/framework/config.

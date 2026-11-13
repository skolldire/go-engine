# Retry minimal

Run `GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly go run .`. Local replaces point only to retry four directories above. Version v0.0.0 is a local placeholder, not a published tag. Root `make check-retry` verifies module/package/graph allowlists and runs both consumers offline. No sockets, framework or external services.

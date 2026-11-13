# config composed consumer

Go 1.27.4. GOWORK=off GOPROXY=off go run -mod=readonly .
Reads YAML base/profile via config/viper, explicit captured lookup via env.Resolver
and bindings, then decodes over concurrency.DefaultPoolConfig(). Application
validates, constructs, executes jobs and shuts down the real Pool. No engine/app.

Replaces: ../../../../config, ../../../../config/viper, ../../../../concurrency.
External dependencies are pinned in go.mod/go.sum and the composed allowlist.
Populate the cache with go mod download before running offline. No tags implied.

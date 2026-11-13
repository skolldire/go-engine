# config minimal consumer

Go 1.27.4. GOWORK=off GOPROXY=off go run -mod=readonly .
Only config/json/env and mapstructure/v2 v2.5.0; Viper is absent from the graph.
Explicit FS, captured lookup, env.Resolver, defaults/overrides/errors/cancellation.
Sole replace ../../../../config. No engine/app/provider. Populate cache with
go mod download before running offline. This validates a candidate, not a tag.

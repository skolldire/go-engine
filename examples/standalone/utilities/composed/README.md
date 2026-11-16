# utilities + explicit configuration assembly

The application reads SCOPE and calls profile.ParseScope; utilities never reads it. Known profiles select base.yaml then <profile>.yaml on an explicit in-memory fstest FS passed to config/viper. Config defaults are seeded and the override wins; unknown profiles fail before source construction. No cwd/file search or engine is involved.

Local replaces ../../../../{utilities,config,config/viper} point only at used modules. Run go run . and go test -race ./... with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly and a prepared cache. SCOPE must be unset/empty or end in a supported token; tests cover all four profiles independently of the process environment.

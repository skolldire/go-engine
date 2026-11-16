# health + explicit application logging

Health knows no logger. The application builds logging.New over bytes.Buffer and explicitly logs failed CheckResult.Err with slog. The serialized Report contains neither backend detail nor PanicError.Value/Stack; PanicError.Error is fixed. The trusted log intentionally contains the normal backend error, demonstrating the application's separate disclosure policy. It does not log panic payload/stack.

Local replaces ../../../../health and ../../../../logging point only at used modules. Run go run . and go test -race ./... with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. No router, engine, network or files are involved.

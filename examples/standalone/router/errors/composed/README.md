# Readiness errors mapped at an application boundary

Local ServeMux and ResponseRecorder assemble real health and logging with router/errors. Readiness failure maps explicitly to503 Problem Details; health preserves its private cause, and the caller deliberately logs causes in a trusted buffer. Neither the public response nor serialized Report exposes private cause/panic Value/Stack. The request ID comes from an application-owned context accessor, ignoring an untrusted request header.

This is a local application fixture at /example, not a router/operations implementation. Local replaces ../../../../../{router/errors,health,logging} point only at used modules. Run go run . and go test -race ./... with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. No engine, chi, SDK, driver or socket is involved.

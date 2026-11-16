# Independent health consumer

Only health and stdlib. Local replace ../../../../health is development-only. CheckerFunc, Live/Ready as probe, Check as diagnostic, original errors, JSON exclusion and registration freeze are exercised in memory. No HTTP endpoint, logger, SDK, engine or driver is needed.

Run GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run . and go test -race ./....

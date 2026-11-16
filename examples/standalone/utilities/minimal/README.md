# Independent utilities consumer

Only utilities and stdlib. Local replace ../../../../utilities is development-only. Explicit profile parsing, Unicode text, generic copies/pointers and stdlib cmp.Or are exercised without engine or implicit environment access.

Run GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly go run . and go test -race ./....

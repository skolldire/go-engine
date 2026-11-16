# auth/jwt/http minimal

Run `go run .` and `go test -race ./...` with GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly. No sockets, engine or services. Local replaces select only modules used, relative to this directory. The graph includes auth/jwt and its JWT-library dependency through the Claims API; a fake verifier does not construct a JWKS client. Composed signs only local verification fixtures, not a token issuer API.

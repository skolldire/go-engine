module example.com/httpclient-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/client v0.0.0
	github.com/skolldire/go-engine/client/logging v0.0.0
	github.com/skolldire/go-engine/httpclient v0.0.0
	github.com/skolldire/go-engine/logging v0.0.0
	github.com/skolldire/go-engine/resilience v0.0.0
	github.com/skolldire/go-engine/retry v0.0.0
)

replace github.com/skolldire/go-engine/client => ../../../../client

replace github.com/skolldire/go-engine/client/logging => ../../../../client/logging

replace github.com/skolldire/go-engine/httpclient => ../../../../httpclient

replace github.com/skolldire/go-engine/logging => ../../../../logging

replace github.com/skolldire/go-engine/resilience => ../../../../resilience

replace github.com/skolldire/go-engine/retry => ../../../../retry

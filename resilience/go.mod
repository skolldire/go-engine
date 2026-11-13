module github.com/skolldire/go-engine/resilience

go 1.27.4

require (
	github.com/skolldire/go-engine/circuitbreaker v0.0.0
	github.com/skolldire/go-engine/client v0.0.0
	github.com/skolldire/go-engine/concurrency v0.0.0
	github.com/skolldire/go-engine/ratelimit v0.0.0
	github.com/skolldire/go-engine/retry v0.0.0
)

replace github.com/skolldire/go-engine/client => ../client

replace github.com/skolldire/go-engine/retry => ../retry

replace github.com/skolldire/go-engine/ratelimit => ../ratelimit

replace github.com/skolldire/go-engine/concurrency => ../concurrency

replace github.com/skolldire/go-engine/circuitbreaker => ../circuitbreaker

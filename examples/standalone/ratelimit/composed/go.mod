module example.com/ratelimit-composed

go 1.27.4

require github.com/skolldire/go-engine/ratelimit v0.0.0

replace github.com/skolldire/go-engine/ratelimit => ../../../../ratelimit

require github.com/skolldire/go-engine/client v0.0.0

replace github.com/skolldire/go-engine/client => ../../../../client

require github.com/skolldire/go-engine/retry v0.0.0

replace github.com/skolldire/go-engine/retry => ../../../../retry

require github.com/skolldire/go-engine/circuitbreaker v0.0.0

replace github.com/skolldire/go-engine/circuitbreaker => ../../../../circuitbreaker

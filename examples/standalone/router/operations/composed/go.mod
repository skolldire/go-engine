module example.com/router-operations-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/health v0.0.0
	github.com/skolldire/go-engine/router/operations v0.0.0
)

replace github.com/skolldire/go-engine/router/operations => ../../../../../router/operations

replace github.com/skolldire/go-engine/health => ../../../../../health

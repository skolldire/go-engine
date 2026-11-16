module example.com/router-errors-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/health v0.0.0
	github.com/skolldire/go-engine/logging v0.0.0
	github.com/skolldire/go-engine/router/errors v0.0.0
)

replace github.com/skolldire/go-engine/router/errors => ../../../../../router/errors

replace github.com/skolldire/go-engine/health => ../../../../../health

replace github.com/skolldire/go-engine/logging => ../../../../../logging

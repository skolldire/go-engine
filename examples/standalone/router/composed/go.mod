module example.com/router-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/health v0.0.0
	github.com/skolldire/go-engine/logging v0.0.0
	github.com/skolldire/go-engine/router v0.0.0
	github.com/skolldire/go-engine/router/errors v0.0.0
	github.com/skolldire/go-engine/router/operations v0.0.0
)

replace github.com/skolldire/go-engine/router => ../../../../router

replace github.com/skolldire/go-engine/router/errors => ../../../../router/errors

replace github.com/skolldire/go-engine/router/operations => ../../../../router/operations

replace github.com/skolldire/go-engine/health => ../../../../health

replace github.com/skolldire/go-engine/logging => ../../../../logging

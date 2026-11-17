module example.com/presets-httpapi-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/health v0.0.0
	github.com/skolldire/go-engine/logging v0.0.0
	github.com/skolldire/go-engine/presets/httpapi v0.0.0
	github.com/skolldire/go-engine/router v0.0.0
	github.com/skolldire/go-engine/router/errors v0.0.0
)

require github.com/skolldire/go-engine/router/operations v0.0.0 // indirect

replace github.com/skolldire/go-engine/health => ../../../../../health

replace github.com/skolldire/go-engine/logging => ../../../../../logging

replace github.com/skolldire/go-engine/presets/httpapi => ../../../../../presets/httpapi

replace github.com/skolldire/go-engine/router => ../../../../../router

replace github.com/skolldire/go-engine/router/errors => ../../../../../router/errors

replace github.com/skolldire/go-engine/router/operations => ../../../../../router/operations

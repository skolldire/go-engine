module example.com/router-chi-composed

go 1.27.4

require (
	github.com/skolldire/go-engine/router v0.0.0
	github.com/skolldire/go-engine/router/chi v0.0.0
)

require github.com/go-chi/chi/v5 v5.5.0 // indirect

replace github.com/skolldire/go-engine/router/chi => ../../../../../router/chi

replace github.com/skolldire/go-engine/router => ../../../../../router

module example.com/auth-jwt-http-composed

go 1.27.4

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/skolldire/go-engine/auth/jwt v0.0.0
	github.com/skolldire/go-engine/auth/jwt/http v0.0.0
	github.com/skolldire/go-engine/health v0.0.0
	github.com/skolldire/go-engine/router v0.0.0
	github.com/skolldire/go-engine/router/errors v0.0.0
)

replace github.com/skolldire/go-engine/auth/jwt/http => ../../../../../../auth/jwt/http

replace github.com/skolldire/go-engine/auth/jwt => ../../../../../../auth/jwt

replace github.com/skolldire/go-engine/router => ../../../../../../router

replace github.com/skolldire/go-engine/router/errors => ../../../../../../router/errors

replace github.com/skolldire/go-engine/health => ../../../../../../health

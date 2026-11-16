module example.com/auth-jwt-http-minimal

go 1.27.4

require (
	github.com/skolldire/go-engine/auth/jwt v0.0.0
	github.com/skolldire/go-engine/auth/jwt/http v0.0.0
)

require github.com/golang-jwt/jwt/v5 v5.3.1 // indirect

replace github.com/skolldire/go-engine/auth/jwt/http => ../../../../../../auth/jwt/http

replace github.com/skolldire/go-engine/auth/jwt => ../../../../../../auth/jwt

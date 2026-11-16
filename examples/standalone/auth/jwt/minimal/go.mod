module example.com/auth-jwt-minimal

go 1.27.4

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/skolldire/go-engine/auth/jwt v0.0.0
)

replace github.com/skolldire/go-engine/auth/jwt => ../../../../../auth/jwt

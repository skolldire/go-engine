module example.com/httpclient-resty-minimal

go 1.27.4

require (
	github.com/skolldire/go-engine/httpclient v0.0.0
	github.com/skolldire/go-engine/httpclient/resty v0.0.0
)

require (
	github.com/go-resty/resty/v2 v2.17.2 // indirect
	golang.org/x/net v0.62.0 // indirect
)

replace github.com/skolldire/go-engine/httpclient => ../../../../../httpclient

replace github.com/skolldire/go-engine/httpclient/resty => ../../../../../httpclient/resty

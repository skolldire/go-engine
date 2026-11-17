module github.com/skolldire/go-engine/httpclient/resty

go 1.27.4

require (
	github.com/go-resty/resty/v2 v2.17.2
	github.com/skolldire/go-engine/httpclient v0.0.0
)

require golang.org/x/net v0.62.0 // indirect

replace github.com/skolldire/go-engine/httpclient => ..

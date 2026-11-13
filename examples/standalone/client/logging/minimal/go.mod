module example.com/client-logging-minimal

go 1.27.4

require (
 github.com/skolldire/go-engine/client v0.0.0
 github.com/skolldire/go-engine/client/logging v0.0.0
)

replace github.com/skolldire/go-engine/client => ../../../../../client
replace github.com/skolldire/go-engine/client/logging => ../../../../../client/logging

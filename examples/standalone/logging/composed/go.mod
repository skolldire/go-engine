module example.com/go-engine-logging-composed

go 1.27.4

require (
 github.com/skolldire/go-engine/logging v0.0.0
 github.com/skolldire/go-engine/client v0.0.0
)

replace github.com/skolldire/go-engine/logging => ../../../../logging
replace github.com/skolldire/go-engine/client => ../../../../client

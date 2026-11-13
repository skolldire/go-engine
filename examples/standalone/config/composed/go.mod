module example.com/config-composed

go 1.27.4

require (
 github.com/skolldire/go-engine/config v0.0.0
 github.com/skolldire/go-engine/concurrency v0.0.0
)

replace github.com/skolldire/go-engine/config => ../../../../config
replace github.com/skolldire/go-engine/concurrency => ../../../../concurrency

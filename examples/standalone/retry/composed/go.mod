module example.com/retry-composed

go 1.27.4

require github.com/skolldire/go-engine/retry v0.0.0

replace github.com/skolldire/go-engine/retry => ../../../../retry

require github.com/skolldire/go-engine/client v0.0.0

replace github.com/skolldire/go-engine/client => ../../../../client

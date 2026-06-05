module github.com/skolldire/go-engine/database/redis

go 1.26.3

require (
	github.com/redis/go-redis/v9 v9.20.0
	github.com/skolldire/go-engine v0.17.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/skolldire/go-engine v0.17.0 => ../..

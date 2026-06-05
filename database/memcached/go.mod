module github.com/skolldire/go-engine/database/memcached

go 1.26.3

require (
	github.com/bradfitz/gomemcache v0.0.0-20260422231931-4d751bb6e37c
	github.com/skolldire/go-engine v0.17.0
)

require (
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/skolldire/go-engine v0.17.0 => ../..

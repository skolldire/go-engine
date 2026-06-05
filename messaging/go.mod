module github.com/skolldire/go-engine/messaging

go 1.26.3

require (
	github.com/rabbitmq/amqp091-go v1.11.0
	github.com/segmentio/kafka-go v0.4.51
	github.com/skolldire/go-engine v0.17.0
	google.golang.org/grpc v1.81.1
)

require (
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/skolldire/go-engine v0.17.0 => ..

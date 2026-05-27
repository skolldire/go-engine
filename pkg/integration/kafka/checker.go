package kafka

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/skolldire/go-engine/pkg/health"
)

const defaultCheckerTimeout = 3 * time.Second

var _ health.Checker = (*KafkaChecker)(nil)

type KafkaChecker struct {
	brokers []string
	timeout time.Duration
}

func NewChecker(brokers []string) *KafkaChecker {
	return &KafkaChecker{
		brokers: brokers,
		timeout: defaultCheckerTimeout,
	}
}

func (k *KafkaChecker) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	dialer := &net.Dialer{}
	for _, broker := range k.brokers {
		conn, err := dialer.DialContext(checkCtx, "tcp", broker)
		if err == nil {
			_ = conn.Close()
			return nil
		}
	}

	return fmt.Errorf("kafka: no brokers reachable: [%s]", strings.Join(k.brokers, ", "))
}

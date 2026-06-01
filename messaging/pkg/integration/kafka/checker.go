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

// KafkaChecker implements health.Checker by attempting a TCP dial to each
// configured broker. It is satisfied by a successful connection to any one
// broker, making it resilient to partial broker failures in a cluster.
type KafkaChecker struct {
	brokers []string
	timeout time.Duration
}

// NewChecker creates a KafkaChecker for the given broker addresses.
// The check timeout defaults to 3 s and can be overridden for tests via the
// exported timeout field.
func NewChecker(brokers []string) *KafkaChecker {
	return &KafkaChecker{
		brokers: brokers,
		timeout: defaultCheckerTimeout,
	}
}

// Check dials each broker in turn. Returns nil as soon as one broker accepts
// the connection. Returns an error listing all brokers if none respond within
// the timeout. Compatible with health.Checker.
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

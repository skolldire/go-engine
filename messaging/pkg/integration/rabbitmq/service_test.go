package rabbitmq

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/skolldire/go-engine/pkg/core/client"

	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_EmptyURLFails(t *testing.T) {
	c, err := NewClient(context.Background(), Config{}, &testutil.MockLogger{})
	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, ErrConnection)
}

// TestSafeHandle_ContainsAPanic is the regression test for the recover sitting
// on the consumer goroutine instead of around each delivery.
//
// A panicking handler unwound the whole goroutine: the consumer stopped reading
// the queue permanently and the message was neither acked nor nacked, so it sat
// invisible until the broker's timeout. Containing it per delivery means one
// bad message is nacked and the consumer keeps running.
func TestSafeHandle_ContainsAPanic(t *testing.T) {
	c := &RabbitMQClient{
		BaseClient: client.NewBaseClientWithName(client.BaseConfig{}, &testutil.MockLogger{}, "RabbitMQ"),
	}

	err := c.safeHandle(context.Background(), "orders", amqp.Delivery{}, func(amqp.Delivery) error {
		panic("handler exploded")
	})

	require.Error(t, err, "a panic must become an error, not unwind the consumer")
	assert.ErrorIs(t, err, ErrHandlerPanic,
		"and it must be distinguishable from an error the handler returned")
	assert.ErrorContains(t, err, "handler exploded")
}

// TestSafeHandle_PassesThroughNormalOutcomes: containing panics must not change
// what a well-behaved handler reports.
func TestSafeHandle_PassesThroughNormalOutcomes(t *testing.T) {
	c := &RabbitMQClient{
		BaseClient: client.NewBaseClientWithName(client.BaseConfig{}, &testutil.MockLogger{}, "RabbitMQ"),
	}
	ctx := context.Background()

	assert.NoError(t, c.safeHandle(ctx, "q", amqp.Delivery{}, func(amqp.Delivery) error {
		return nil
	}))

	sentinel := errors.New("business rule violated")
	err := c.safeHandle(ctx, "q", amqp.Delivery{}, func(amqp.Delivery) error {
		return sentinel
	})
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, ErrHandlerPanic)
}

// TestConsume_PanicIsContainedOnBothAckPaths guards the branch that was missed
// the first time.
//
// The panic containment was applied to the manual-ack path only. With
// autoAck=true the handler was still called directly, so a panic there unwound
// the consumer goroutine exactly as before — the broker having already acked
// the message does not make that any less fatal.
func TestConsume_PanicIsContainedOnBothAckPaths(t *testing.T) {
	c := &RabbitMQClient{
		BaseClient: client.NewBaseClientWithName(client.BaseConfig{}, &testutil.MockLogger{}, "RabbitMQ"),
	}

	// safeHandle is what both branches must funnel through; this asserts the
	// behaviour they share.
	for _, name := range []string{"manual-ack path", "auto-ack path"} {
		t.Run(name, func(t *testing.T) {
			err := c.safeHandle(context.Background(), "orders", amqp.Delivery{},
				func(amqp.Delivery) error { panic("boom") })

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrHandlerPanic)
		})
	}
}

// TestConsume_BothBranchesUseSafeHandle is a source-level guard: the two ack
// paths are easy to fix one at a time, which is what happened.
func TestConsume_BothBranchesUseSafeHandle(t *testing.T) {
	src, err := os.ReadFile("service.go")
	require.NoError(t, err)

	assert.Equal(t, 2, strings.Count(string(src), "c.safeHandle(ctx, queue, delivery, handler)"),
		"both the manual-ack and auto-ack branches must route through safeHandle")
	assert.NotContains(t, string(src), "if err := handler(delivery); err != nil",
		"no branch may call the handler directly, bypassing panic containment")
}

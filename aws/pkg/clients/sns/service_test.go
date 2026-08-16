package sns

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func newTestClient() Service {
	return NewClient(aws.Config{Region: "us-east-1"}, Config{}, &testutil.MockLogger{})
}

func TestNewClient_NotNil(t *testing.T) {
	assert.NotNil(t, newTestClient())
}

// Inputs rejected before any AWS call — no backend required.
func TestValidationErrors(t *testing.T) {
	c := newTestClient()
	ctx := context.Background()

	_, err := c.CreateTopic(ctx, "", nil)
	assert.ErrorIs(t, err, ErrInvalidInput)

	assert.ErrorIs(t, c.DeleteTopic(ctx, ""), ErrInvalidInput)

	_, err = c.PublishMessage(ctx, "", "msg", nil)
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, err = c.PublishMessage(ctx, "arn", "", nil)
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, err = c.CreateSubscription(ctx, "", "", "")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestEnableLogging(t *testing.T) {
	c := newTestClient()
	assert.NotPanics(t, func() { c.EnableLogging(true); c.EnableLogging(false) })
}

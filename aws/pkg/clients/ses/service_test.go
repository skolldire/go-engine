package ses

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient() Service {
	return NewClient(aws.Config{Region: "us-east-1"}, Config{}, &testutil.MockLogger{})
}

func TestNewClient_NotNil(t *testing.T) {
	assert.NotNil(t, newTestClient())
}

// These inputs are rejected before any AWS call, so they run without a backend.
func TestSendEmail_InvalidInput(t *testing.T) {
	c := newTestClient()
	ctx := context.Background()

	_, err := c.SendEmail(ctx, EmailMessage{}) // no From, no To
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, err = c.SendEmail(ctx, EmailMessage{From: EmailAddress{Email: "a@b.com"}}) // no recipients
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestVerifyEmailAddress_InvalidInput(t *testing.T) {
	c := newTestClient()
	assert.ErrorIs(t, c.VerifyEmailAddress(context.Background(), ""), ErrInvalidInput)
	assert.ErrorIs(t, c.DeleteVerifiedEmailAddress(context.Background(), ""), ErrInvalidInput)
}

func TestEnableLogging(t *testing.T) {
	c := newTestClient()
	assert.NotPanics(t, func() { c.EnableLogging(true); c.EnableLogging(false) })
}

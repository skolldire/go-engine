package ssm

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

	_, err := c.GetParameter(ctx, "", false)
	assert.ErrorIs(t, err, ErrInvalidInput)

	_, err = c.GetParametersByPath(ctx, "", false, false)
	assert.ErrorIs(t, err, ErrInvalidInput)

	err = c.PutParameter(ctx, "", "", "String", "", false, nil)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestEnableLogging(t *testing.T) {
	c := newTestClient()
	assert.NotPanics(t, func() { c.EnableLogging(true); c.EnableLogging(false) })
}

package dynamo

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_NotNil(t *testing.T) {
	c := NewClient(context.Background(), aws.Config{Region: "us-east-1"}, Config{}, &testutil.MockLogger{})
	require.NotNil(t, c)
}

func TestTableName(t *testing.T) {
	assert.Equal(t, "users", (&DynamoClient{}).TableName("users"))
	assert.Equal(t, "prod-users", (&DynamoClient{tablePrefix: "prod"}).TableName("users"))
}

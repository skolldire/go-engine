package rabbitmq

import (
	"testing"

	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_EmptyURLFails(t *testing.T) {
	c, err := NewClient(Config{}, &testutil.MockLogger{})
	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, ErrConnection)
}

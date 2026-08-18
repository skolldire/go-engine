package memcached

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_NoServersFails(t *testing.T) {
	c, err := NewClient(context.Background(), Config{}, &testutil.MockLogger{})
	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, ErrConnection)
}

func TestKeyName(t *testing.T) {
	assert.Equal(t, "raw", (&MemcachedClient{}).KeyName("raw"))
	assert.Equal(t, "app:raw", (&MemcachedClient{prefix: "app"}).KeyName("raw"))
}

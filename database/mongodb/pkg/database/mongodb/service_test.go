package mongodb

import (
	"context"
	"testing"

	"github.com/skolldire/go-engine/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_Validation(t *testing.T) {
	log := &testutil.MockLogger{}
	tests := []struct {
		name string
		cfg  Config
	}{
		{"empty URI", Config{Database: "db"}},
		{"bad URI scheme", Config{URI: "http://localhost", Database: "db"}},
		{"empty database", Config{URI: "mongodb://localhost:27017"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(context.Background(), tt.cfg, log)
			require.Error(t, err)
			assert.Nil(t, c)
			assert.ErrorIs(t, err, ErrConnection)
		})
	}
}

func TestRedactMongoURI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no credentials", "mongodb://localhost:27017/db", "mongodb://localhost:27017/db"},
		{"with credentials", "mongodb://user:pass@localhost:27017/db", "mongodb://***:***@localhost:27017/db"},
		{"srv with credentials", "mongodb+srv://user:pass@cluster.example.com/db", "mongodb+srv://***:***@cluster.example.com/db"},
		{"unknown scheme untouched", "custom://user:pass@host", "custom://user:pass@host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactMongoURI(tt.in))
		})
	}
}

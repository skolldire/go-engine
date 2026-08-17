package gormsql

import (
	"context"
	"testing"
	"time"

	baseclient "github.com/skolldire/go-engine/pkg/core/client"
	"github.com/stretchr/testify/assert"
)

func TestEnsureContextWithTimeout(t *testing.T) {
	// The timeout behaviour now comes from the embedded BaseClient, so the
	// client must be built with one, exactly as the constructor does.
	dbc := &DBClient{
		BaseClient: baseclient.NewBaseClientWithName(baseclient.BaseConfig{}, nil, "SQL"),
	}

	// No deadline on the input → a timeout is applied.
	ctx, cancel := dbc.ContextWithTimeout(context.Background())
	defer cancel()
	_, hasDeadline := ctx.Deadline()
	assert.True(t, hasDeadline)

	// Existing deadline is preserved.
	parent, pcancel := context.WithTimeout(context.Background(), time.Minute)
	defer pcancel()
	derived, dcancel := dbc.ContextWithTimeout(parent)
	defer dcancel()
	dl, ok := derived.Deadline()
	assert.True(t, ok)
	pdl, _ := parent.Deadline()
	assert.Equal(t, pdl, dl)
}

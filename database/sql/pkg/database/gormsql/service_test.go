package gormsql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnsureContextWithTimeout(t *testing.T) {
	dbc := &DBClient{}

	// No deadline on the input → a timeout is applied.
	ctx, cancel := dbc.ensureContextWithTimeout(context.Background())
	defer cancel()
	_, hasDeadline := ctx.Deadline()
	assert.True(t, hasDeadline)

	// Existing deadline is preserved.
	parent, pcancel := context.WithTimeout(context.Background(), time.Minute)
	defer pcancel()
	derived, dcancel := dbc.ensureContextWithTimeout(parent)
	defer dcancel()
	dl, ok := derived.Deadline()
	assert.True(t, ok)
	pdl, _ := parent.Deadline()
	assert.Equal(t, pdl, dl)
}

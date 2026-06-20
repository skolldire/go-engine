package cognito

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newGroupsTestClient(t *testing.T) Service {
	t.Helper()
	cfg := Config{
		Region:        "us-east-1",
		UserPoolID:    "us-east-1_TestPool123",
		ClientID:      "test-client-id",
		EnableLogging: false,
	}
	client, err := NewClient(cfg, &mockLogger{})
	assert.NoError(t, err)
	assert.NotNil(t, client)
	return client
}

func TestClient_AddUserToGroup_Validation(t *testing.T) {
	client := newGroupsTestClient(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		group    string
	}{
		{name: "empty username", username: "", group: "administrador"},
		{name: "empty group", username: "user-1", group: ""},
		{name: "both empty", username: "", group: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AddUserToGroup(ctx, tt.username, tt.group)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrMissingRequiredField))
		})
	}
}

func TestClient_RemoveUserFromGroup_Validation(t *testing.T) {
	client := newGroupsTestClient(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		group    string
	}{
		{name: "empty username", username: "", group: "administrador"},
		{name: "empty group", username: "user-1", group: ""},
		{name: "both empty", username: "", group: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.RemoveUserFromGroup(ctx, tt.username, tt.group)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrMissingRequiredField))
		})
	}
}

func TestClient_ListGroupsForUser_Validation(t *testing.T) {
	client := newGroupsTestClient(t)
	ctx := context.Background()

	groups, err := client.ListGroupsForUser(ctx, "")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingRequiredField))
	assert.Nil(t, groups)
}

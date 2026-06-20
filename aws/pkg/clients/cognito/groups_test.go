package cognito

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCognitoAPI embeds cognitoAPI (nil) and overrides only AdminListGroupsForUser,
// which is enough to exercise the pagination/error/edge-case logic in ListGroupsForUser.
type stubCognitoAPI struct {
	cognitoAPI
	listResponses []*cognitoidentityprovider.AdminListGroupsForUserOutput
	listErr       error
	calls         int
}

func (s *stubCognitoAPI) AdminListGroupsForUser(_ context.Context, _ *cognitoidentityprovider.AdminListGroupsForUserInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminListGroupsForUserOutput, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	resp := s.listResponses[s.calls]
	s.calls++
	return resp, nil
}

// newGroupsStubClient builds a *Client wired to a stubbed Cognito API so the
// behavioral logic can be tested without real AWS calls.
func newGroupsStubClient(api cognitoAPI) *Client {
	return &Client{
		config:        Config{UserPoolID: "us-east-1_TestPool123"},
		cognitoClient: api,
		logger:        &mockLogger{},
		logging:       false,
	}
}

func groupPage(next string, names ...string) *cognitoidentityprovider.AdminListGroupsForUserOutput {
	out := &cognitoidentityprovider.AdminListGroupsForUserOutput{}
	for _, n := range names {
		out.Groups = append(out.Groups, types.GroupType{GroupName: aws.String(n)})
	}
	if next != "" {
		out.NextToken = aws.String(next)
	}
	return out
}

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

func TestClient_ListGroupsForUser_Paginated(t *testing.T) {
	api := &stubCognitoAPI{
		listResponses: []*cognitoidentityprovider.AdminListGroupsForUserOutput{
			groupPage("page-2", "administrador", "copropietario"),
			groupPage("", "operador"),
		},
	}
	c := newGroupsStubClient(api)

	groups, err := c.ListGroupsForUser(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"administrador", "copropietario", "operador"}, groups)
	assert.Equal(t, 2, api.calls, "should follow the NextToken to a second page")
}

func TestClient_ListGroupsForUser_Empty(t *testing.T) {
	api := &stubCognitoAPI{
		listResponses: []*cognitoidentityprovider.AdminListGroupsForUserOutput{
			groupPage(""), // no groups, no next token
		},
	}
	c := newGroupsStubClient(api)

	groups, err := c.ListGroupsForUser(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Empty(t, groups)
	assert.Equal(t, 1, api.calls)
}

func TestClient_ListGroupsForUser_ErrorMapped(t *testing.T) {
	api := &stubCognitoAPI{
		listErr: &types.UserNotFoundException{Message: aws.String("user does not exist")},
	}
	c := newGroupsStubClient(api)

	groups, err := c.ListGroupsForUser(context.Background(), "ghost")
	assert.Error(t, err)
	assert.Nil(t, groups)
	// handleCognitoError maps UserNotFoundException to ErrUserNotFound.
	assert.True(t, errors.Is(err, ErrUserNotFound), "expected ErrUserNotFound, got %v", err)
}

func TestClient_ListGroupsForUser_UnexpectedResponse(t *testing.T) {
	// A nil output with no error must surface as an error, not a silent success.
	api := &stubCognitoAPI{
		listResponses: []*cognitoidentityprovider.AdminListGroupsForUserOutput{nil},
	}
	c := newGroupsStubClient(api)

	groups, err := c.ListGroupsForUser(context.Background(), "user-1")
	assert.Error(t, err)
	assert.Nil(t, groups)
	assert.True(t, errors.Is(err, ErrUnexpectedResponse), "expected ErrUnexpectedResponse, got %v", err)
}

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
	"github.com/stretchr/testify/assert"
)

func TestSESAdapter_Do_InvalidOperation(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSESAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ses.invalid_operation",
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SES operation")
}

func TestSESAdapter_SendEmail_InvalidBody(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSESAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ses.send_email",
		Body:      []byte("invalid json"),
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON body")
}

func TestSESAdapter_SendEmail_MissingFrom(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSESAdapter(cfg, 0, RetryPolicy{})

	body, _ := json.Marshal(map[string]interface{}{
		"to": []map[string]interface{}{{"email": "to@example.com"}},
	})

	req := &cloud.Request{
		Operation: "ses.send_email",
		Body:      body,
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "from address is required")
}

func TestSESAdapter_VerifyEmailIdentity_InvalidPath(t *testing.T) {
	cfg := aws.Config{Region: "us-east-1"}
	adapter := newSESAdapter(cfg, 0, RetryPolicy{})

	req := &cloud.Request{
		Operation: "ses.verify_email_identity",
		Path:      "", // Empty path
	}

	resp, err := adapter.Do(context.Background(), req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email address is required")
}

func TestNormalizeSESError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		operation string
		wantNil   bool
		wantCode  string
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			operation: "ses.send_email",
			wantNil:   true,
		},
		{
			name:      "error normalized correctly",
			err:       errors.New("email not verified"),
			operation: "ses.send_email",
			wantNil:   false,
			wantCode:  "ses.send_email.error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSESError(tt.err, tt.operation)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantCode, result.Code)
				assert.Equal(t, tt.err.Error(), result.Message)
				assert.NotNil(t, result.Cause)
			}
		})
	}
}

package adapters

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeSQSError(t *testing.T) {
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
			operation: "sqs.send",
			wantNil:   true,
		},
		{
			name:      "error normalized correctly",
			err:       errors.New("queue not found"),
			operation: "sqs.send",
			wantNil:   false,
			wantCode:  "sqs.send.error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSQSError(tt.err, tt.operation)
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

func TestNormalizeSNSError(t *testing.T) {
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
			operation: "sns.publish",
			wantNil:   true,
		},
		{
			name:      "error normalized correctly",
			err:       errors.New("topic not found"),
			operation: "sns.publish",
			wantNil:   false,
			wantCode:  "sns.publish.error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSNSError(tt.err, tt.operation)
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

func TestNormalizeLambdaError(t *testing.T) {
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
			operation: "lambda.invoke",
			wantNil:   true,
		},
		{
			name:      "error normalized correctly",
			err:       errors.New("function error"),
			operation: "lambda.invoke",
			wantNil:   false,
			wantCode:  "lambda.invoke.error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeLambdaError(tt.err, tt.operation)
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

// Note: normalizeS3Error, normalizeSESError, and normalizeSSMError are tested
// in their respective adapter files (s3_test.go, ses_test.go, ssm_test.go)
// since they are private functions in those files

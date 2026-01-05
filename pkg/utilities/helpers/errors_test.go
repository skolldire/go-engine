package helpers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		msg     string
		wantNil bool
		wantMsg string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			msg:     "context",
			wantNil: true,
		},
		{
			name:    "wraps error with message",
			err:     errors.New("original error"),
			msg:     "failed to process",
			wantNil: false,
			wantMsg: "failed to process: original error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapError(tt.err, tt.msg)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.wantMsg)
				assert.ErrorIs(t, result, tt.err)
			}
		})
	}
}

func TestWrapErrorf(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		format  string
		args    []interface{}
		wantNil bool
		wantMsg string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			format:  "failed: %d",
			args:    []interface{}{123},
			wantNil: true,
		},
		{
			name:    "wraps error with formatted message",
			err:     errors.New("original error"),
			format:  "failed to process user %d",
			args:    []interface{}{123},
			wantNil: false,
			wantMsg: "failed to process user 123: original error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapErrorf(tt.err, tt.format, tt.args...)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.wantMsg)
				assert.ErrorIs(t, result, tt.err)
			}
		})
	}
}

func TestNewError(t *testing.T) {
	msg := "test error"
	err := NewError(msg)
	assert.NotNil(t, err)
	assert.Equal(t, msg, err.Error())
}

func TestNewErrorf(t *testing.T) {
	format := "error %d: %s"
	err := NewErrorf(format, 123, "test")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "error 123: test")
}

package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	assert.NotNil(t, v)
}

func TestSetGlobalValidator(t *testing.T) {
	v := NewValidator()
	SetGlobalValidator(v)
	retrieved := GetGlobalValidator()
	assert.Equal(t, v, retrieved)
}

func TestGetGlobalValidator(t *testing.T) {
	// Clear global validator first
	SetGlobalValidator(nil)

	// GetGlobalValidator should create a new one if nil
	v := GetGlobalValidator()
	assert.NotNil(t, v)
}

func TestRegisterCustomValidators(t *testing.T) {
	v := NewValidator()

	// Test not_empty validator
	type TestStruct struct {
		Field string `validate:"not_empty"`
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "hello", false},
		{"empty", "", true},
		{"whitespace", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := TestStruct{Field: tt.value}
			err := v.Struct(test)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_JSONTagName(t *testing.T) {
	v := NewValidator()

	type TestStruct struct {
		FieldName string `json:"field_name" validate:"required"`
		Ignored   string `json:"-" validate:"required"`
	}

	test := TestStruct{}
	err := v.Struct(test)
	assert.Error(t, err)

	// Check that field_name is used, not FieldName
	errs := err.(validator.ValidationErrors)
	found := false
	for _, e := range errs {
		if e.Field() == "field_name" {
			found = true
			break
		}
	}
	assert.True(t, found, "should use JSON tag name")
}

package app_profile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProfileByScope(t *testing.T) {
	tests := []struct {
		scope    string
		expected string
	}{
		{"dev-local", "local"},
		{"uat-test", "test"},
		{"prod", "prod"},
		{"staging-stage", "stage"},
		{"local", "local"}, // Caso sin prefijo
	}

	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			_ = os.Setenv("SCOPE", tt.scope)
			defer func() { _ = os.Unsetenv("SCOPE") }()
			result := GetProfileByScope()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsLocalProfile(t *testing.T) {
	_ = os.Setenv("SCOPE", "local")
	defer func() { _ = os.Unsetenv("SCOPE") }()

	assert.True(t, IsLocalProfile())

	_ = os.Setenv("SCOPE", "prod")
	assert.False(t, IsLocalProfile())
}

func TestIsTestProfile(t *testing.T) {
	_ = os.Setenv("SCOPE", "some-test")
	defer func() { _ = os.Unsetenv("SCOPE") }()

	assert.True(t, IsTestProfile())

	_ = os.Setenv("SCOPE", "prod")
	assert.False(t, IsTestProfile())
}

func TestIsProdProfile(t *testing.T) {
	_ = os.Setenv("SCOPE", "environment-prod")
	defer func() { _ = os.Unsetenv("SCOPE") }()

	assert.True(t, IsProdProfile())

	_ = os.Setenv("SCOPE", "stage")
	assert.False(t, IsProdProfile())
}

func TestIsStageProfile(t *testing.T) {
	_ = os.Setenv("SCOPE", "test-stage")
	defer func() { _ = os.Unsetenv("SCOPE") }()

	assert.True(t, IsStageProfile())

	_ = os.Setenv("SCOPE", "prod")
	assert.False(t, IsStageProfile())
}

func TestGetScopeValue(t *testing.T) {
	_ = os.Setenv("SCOPE", "custom-profile")
	defer func() { _ = os.Unsetenv("SCOPE") }()

	assert.Equal(t, "custom-profile", GetScopeValue())
	_ = os.Unsetenv("SCOPE")
	assert.Equal(t, "local", GetScopeValue())
}

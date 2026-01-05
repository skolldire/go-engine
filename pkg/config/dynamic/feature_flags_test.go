package dynamic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockLogger is now defined in mocks_test.go

func TestNewFeatureFlags(t *testing.T) {
	flags := map[string]interface{}{"flag1": true}
	ff := NewFeatureFlags(flags, nil)
	assert.NotNil(t, ff)
}

func TestNewFeatureFlags_Nil(t *testing.T) {
	ff := NewFeatureFlags(nil, nil)
	assert.NotNil(t, ff)
	result, exists := ff.Get("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, result)
}

func TestFeatureFlags_Get(t *testing.T) {
	flags := map[string]interface{}{
		"flag1": true,
		"flag2": "value",
	}
	ff := NewFeatureFlags(flags, nil)

	value, exists := ff.Get("flag1")
	assert.True(t, exists)
	assert.Equal(t, true, value)

	value, exists = ff.Get("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, value)
}

func TestFeatureFlags_GetBool(t *testing.T) {
	flags := map[string]interface{}{
		"bool_true":   true,
		"bool_false":  false,
		"string_true": "true",
		"string_1":    "1",
		"string_yes":  "yes",
		"string_no":   "no",
		"int":         1,
		"nonexistent": nil,
	}
	ff := NewFeatureFlags(flags, nil)

	assert.True(t, ff.GetBool("bool_true"))
	assert.False(t, ff.GetBool("bool_false"))
	assert.True(t, ff.GetBool("string_true"))
	assert.True(t, ff.GetBool("string_1"))
	assert.True(t, ff.GetBool("string_yes"))
	assert.False(t, ff.GetBool("string_no"))
	assert.False(t, ff.GetBool("int")) // Non-bool, non-string-true returns false
	assert.False(t, ff.GetBool("nonexistent"))
}

func TestFeatureFlags_GetString(t *testing.T) {
	flags := map[string]interface{}{
		"string": "value",
		"int":    42,
		"bool":   true,
	}
	ff := NewFeatureFlags(flags, nil)

	assert.Equal(t, "value", ff.GetString("string"))
	assert.Equal(t, "42", ff.GetString("int"))
	assert.Equal(t, "true", ff.GetString("bool"))
	assert.Equal(t, "", ff.GetString("nonexistent"))
}

func TestFeatureFlags_GetInt(t *testing.T) {
	flags := map[string]interface{}{
		"int":    42,
		"float":  3.14,
		"string": "123",
		"bool":   true,
	}
	ff := NewFeatureFlags(flags, nil)

	assert.Equal(t, 42, ff.GetInt("int"))
	assert.Equal(t, 3, ff.GetInt("float")) // Truncated
	assert.Equal(t, 123, ff.GetInt("string"))
	assert.Equal(t, 0, ff.GetInt("bool")) // Non-numeric returns 0
	assert.Equal(t, 0, ff.GetInt("nonexistent"))
}

func TestFeatureFlags_Set(t *testing.T) {
	mockLog := new(mockLogger)
	mockLog.On("Debug", mock.Anything, "feature flag updated", mock.Anything).Return()
	ff := NewFeatureFlags(map[string]interface{}{"key1": "value1"}, mockLog)
	ff.Set("key2", "value2")

	value, exists := ff.Get("key2")
	assert.True(t, exists)
	assert.Equal(t, "value2", value)
	mockLog.AssertExpectations(t)
}

func TestFeatureFlags_SetAll(t *testing.T) {
	mockLog := new(mockLogger)
	mockLog.On("Info", mock.Anything, "feature flags updated", mock.Anything).Return()
	ff := NewFeatureFlags(map[string]interface{}{"old": "value"}, mockLog)
	newFlags := map[string]interface{}{"new": "value"}
	ff.SetAll(newFlags)

	all := ff.GetAll()
	assert.Equal(t, newFlags, all)
	mockLog.AssertExpectations(t)
}

func TestFeatureFlags_SetAll_Nil(t *testing.T) {
	mockLog := new(mockLogger)
	mockLog.On("Info", mock.Anything, "feature flags updated", mock.Anything).Return()
	ff := NewFeatureFlags(map[string]interface{}{"old": "value"}, mockLog)
	ff.SetAll(nil)

	all := ff.GetAll()
	assert.NotNil(t, all)
	assert.Equal(t, 0, len(all))
	mockLog.AssertExpectations(t)
}

func TestFeatureFlags_GetAll(t *testing.T) {
	flags := map[string]interface{}{
		"flag1": true,
		"flag2": "value",
	}
	ff := NewFeatureFlags(flags, nil)
	all := ff.GetAll()
	assert.Equal(t, flags, all)
}

func TestFeatureFlags_IsEnabled(t *testing.T) {
	flags := map[string]interface{}{
		"enabled":  true,
		"disabled": false,
	}
	ff := NewFeatureFlags(flags, nil)

	assert.True(t, ff.IsEnabled("enabled"))
	assert.False(t, ff.IsEnabled("disabled"))
	assert.False(t, ff.IsEnabled("nonexistent"))
}

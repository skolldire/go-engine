package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeTypeAssert_Success(t *testing.T) {
	result := "test-string"
	value, err := SafeTypeAssert[string](result)

	assert.NoError(t, err)
	assert.Equal(t, "test-string", value)
}

func TestSafeTypeAssert_Int(t *testing.T) {
	result := 42
	value, err := SafeTypeAssert[int](result)

	assert.NoError(t, err)
	assert.Equal(t, 42, value)
}

func TestSafeTypeAssert_Struct(t *testing.T) {
	type TestStruct struct {
		Name string
		Age  int
	}

	result := TestStruct{Name: "John", Age: 30}
	value, err := SafeTypeAssert[TestStruct](result)

	assert.NoError(t, err)
	assert.Equal(t, "John", value.Name)
	assert.Equal(t, 30, value.Age)
}

func TestSafeTypeAssert_TypeMismatch(t *testing.T) {
	result := "string-value"
	value, err := SafeTypeAssert[int](result)

	assert.Error(t, err)
	assert.Equal(t, 0, value)
	assert.Contains(t, err.Error(), "unexpected response type")
}

func TestSafeTypeAssert_Nil(t *testing.T) {
	var result interface{} = nil
	value, err := SafeTypeAssert[string](result)

	assert.Error(t, err)
	assert.Equal(t, "", value)
}

func TestSafeTypeAssert_WrongType(t *testing.T) {
	result := 42
	value, err := SafeTypeAssert[string](result)

	assert.Error(t, err)
	assert.Equal(t, "", value)
	assert.Contains(t, err.Error(), "expected string")
}

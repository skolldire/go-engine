package validation

import (
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/skolldire/go-engine/pkg/utilities/helpers"
)

var (
	globalValidator *validator.Validate
	mu              sync.RWMutex
)

func NewValidator() *validator.Validate {
	v := validator.New()

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	registerCustomValidators(v)

	return v
}

func registerCustomValidators(v *validator.Validate) {
	// "not_empty" validator only applies to string fields
	v.RegisterValidation("not_empty", func(fl validator.FieldLevel) bool {
		// Check if field is a string type
		if fl.Field().Kind() != reflect.String {
			return false // Not a string, validation fails
		}
		return helpers.IsNotEmptyString(fl.Field().String())
	})
}

func SetGlobalValidator(v *validator.Validate) {
	mu.Lock()
	defer mu.Unlock()
	globalValidator = v
}

func GetGlobalValidator() *validator.Validate {
	mu.RLock()
	if globalValidator != nil {
		mu.RUnlock()
		return globalValidator
	}
	mu.RUnlock()
	
	// Double-checked locking
	mu.Lock()
	defer mu.Unlock()
	if globalValidator != nil {
		return globalValidator
	}
	globalValidator = NewValidator()
	return globalValidator
}

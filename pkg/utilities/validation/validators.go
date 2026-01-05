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

// NewValidator creates a *validator.Validate configured for this package.
// 
// The returned validator derives field names from a struct field's `json` tag
// (fields with `json:"-"` are omitted) and has the package's custom validations
// registered.
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

// registerCustomValidators registers the "not_empty" validation on v, which validates that a string field is not empty using helpers.IsNotEmptyString.
func registerCustomValidators(v *validator.Validate) {
	v.RegisterValidation("not_empty", func(fl validator.FieldLevel) bool {
		return helpers.IsNotEmptyString(fl.Field().String())
	})
}

// SetGlobalValidator sets the package-level validator used by GetGlobalValidator.
// It is safe for concurrent use. Passing nil clears the global validator.
func SetGlobalValidator(v *validator.Validate) {
	mu.Lock()
	defer mu.Unlock()
	globalValidator = v
}

// GetGlobalValidator returns the package-level validator instance if one has been set.
// Otherwise it returns a newly configured validator from NewValidator.
// It is safe for concurrent use.
func GetGlobalValidator() *validator.Validate {
	mu.RLock()
	defer mu.RUnlock()
	if globalValidator != nil {
		return globalValidator
	}
	return NewValidator()
}
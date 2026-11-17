// Package validator plugs go-playground/validator into Fiber's binder.
// Once registered as fiber.Config.StructValidator, every c.Bind().Body/Query call validates
// the target struct and returns *apperror.Error on failure.
package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/yourorg/music-api/internal/apperror"
)

// Validator implements fiber.StructValidator.
type Validator struct {
	v *validator.Validate
}

// New builds a validator that reports fields by their json/query name.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Report field names as the client sees them (json/query tag), not Go field names.
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		for _, tag := range []string{"json", "query", "params"} {
			name := strings.SplitN(f.Tag.Get(tag), ",", 2)[0]
			if name == "-" {
				return ""
			}
			if name != "" {
				return name
			}
		}
		return f.Name
	})
	return &Validator{v: v}
}

// Validate implements fiber.StructValidator.
func (val *Validator) Validate(out any) error {
	err := val.v.Struct(out)
	if err == nil {
		return nil
	}

	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return apperror.Internal(err)
	}

	details := make([]apperror.ErrorDetail, 0, len(verrs))
	for _, fe := range verrs {
		details = append(details, apperror.ErrorDetail{Field: fe.Field(), Message: message(fe)})
	}
	return apperror.Validation(details)
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return fmt.Sprintf("must be at least %s", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s", fe.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fe.Param())
	case "url", "http_url":
		return "must be a valid http(s) URL"
	case "uuid", "uuid4":
		return "must be a valid UUID"
	default:
		return fmt.Sprintf("failed rule %q", fe.Tag())
	}
}

package echox

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/go-playground/validator/v10/non-standard/validators"
	entranslations "github.com/go-playground/validator/v10/translations/en"
)

// EnumValid describes a value accepted by the enum validation tag.
type EnumValid interface {
	Valid() bool
}

// InvalidField describes one failed validation rule. Code is the validation
// tag and Message its English go-playground/validator translation.
type InvalidField struct {
	Path    string
	Code    string
	Message string
}

// ValidationError contains stable request-facing validation failures.
type ValidationError struct {
	fields []InvalidField
	cause  error
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "validation failed"
	}

	return fmt.Sprintf("validation failed for %d field(s)", len(e.fields))
}

// Unwrap returns the error produced by go-playground/validator.
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.cause
}

// InvalidFields returns a copy of the failed fields.
func (e *ValidationError) InvalidFields() []InvalidField {
	if e == nil {
		return nil
	}

	fields := make([]InvalidField, len(e.fields))
	copy(fields, e.fields)

	return fields
}

// Validator applies request validation tags.
type Validator struct {
	validate   *validator.Validate
	translator ut.Translator
}

// customTags are registered beyond the baked-in tags of go-playground/validator,
// which has no English translation for them, so echox describes them. enum is
// an echox extension; notblank keeps go-playground's non-standard
// implementation.
var customTags = []struct {
	tag         string
	validate    validator.Func
	translation string
}{
	{tag: "enum", validate: validateEnum, translation: "{0} must be one of the allowed values"},
	{tag: "notblank", validate: validators.NotBlank, translation: "{0} must not be blank"},
}

// MakeValidator creates an Echo-compatible validator with the English
// translations of go-playground/validator. It registers request tag field
// names and the enum and notblank tags with their translations.
func MakeValidator() (*Validator, error) {
	validate := validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(requestFieldName)

	english := en.New()
	translator, _ := ut.New(english, english).GetTranslator(english.Locale())
	if err := entranslations.RegisterDefaultTranslations(validate, translator); err != nil {
		return nil, fmt.Errorf("register English translations: %w", err)
	}

	for _, custom := range customTags {
		if err := validate.RegisterValidation(custom.tag, custom.validate); err != nil {
			return nil, fmt.Errorf("register %s validation: %w", custom.tag, err)
		}

		if err := validate.RegisterTranslation(
			custom.tag,
			translator,
			func(trans ut.Translator) error {
				return trans.Add(custom.tag, custom.translation, false)
			},
			translateField,
		); err != nil {
			return nil, fmt.Errorf("register %s translation: %w", custom.tag, err)
		}
	}

	return &Validator{validate: validate, translator: translator}, nil
}

// Validate implements echo.Validator.
func (v *Validator) Validate(value any) error {
	if v == nil || v.validate == nil {
		return errors.New("validator is nil")
	}

	if err := v.validate.Struct(value); err != nil {
		validationErrors, ok := errors.AsType[validator.ValidationErrors](err)
		if !ok {
			return fmt.Errorf("validate structure: %w", err)
		}

		return makeValidationError(reflect.TypeOf(value), validationErrors, v.translator)
	}

	return nil
}

// GoPlayground returns the underlying validator for application-specific
// validation registrations.
func (v *Validator) GoPlayground() *validator.Validate {
	if v == nil {
		return nil
	}

	return v.validate
}

// Translator returns the English translator for registering translations of
// application-specific tags with GoPlayground().RegisterTranslation.
func (v *Validator) Translator() ut.Translator {
	if v == nil {
		return nil
	}

	return v.translator
}

var requestFieldTags = [...]string{"json", "query", "param", "header"}

func requestFieldName(field reflect.StructField) string {
	for _, tag := range requestFieldTags {
		name, _, _ := strings.Cut(field.Tag.Get(tag), ",")
		if len(name) > 0 && name != "-" {
			return name
		}
	}

	return field.Name
}

func validateEnum(fl validator.FieldLevel) bool {
	value, ok := fl.Field().Interface().(EnumValid)

	return ok && value.Valid()
}

func translateField(trans ut.Translator, fe validator.FieldError) string {
	message, err := trans.T(fe.Tag(), fe.Field())
	if err != nil {
		return fe.Error()
	}

	return message
}

func makeValidationError(
	typ reflect.Type,
	errs validator.ValidationErrors,
	trans ut.Translator,
) *ValidationError {
	fields := make([]InvalidField, len(errs))
	for index, field := range errs {
		fields[index] = InvalidField{
			Path:    validationPath(typ, field),
			Code:    field.Tag(),
			Message: validationMessage(field, trans),
		}
	}

	return &ValidationError{fields: fields, cause: errs}
}

// validationMessage returns the translation of a failed tag. Without one,
// go-playground/validator falls back to its internal error text, which names
// Go types, so an untranslated application tag gets a neutral message.
func validationMessage(fe validator.FieldError, trans ut.Translator) string {
	if message := fe.Translate(trans); message != fe.Error() {
		return message
	}

	return fe.Field() + " is invalid"
}

func validationPath(typ reflect.Type, fe validator.FieldError) string {
	requestParts := strings.Split(fe.Namespace(), ".")
	structParts := strings.Split(fe.StructNamespace(), ".")
	if len(requestParts) < 2 || len(requestParts) != len(structParts) {
		return fe.Field()
	}

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	path := make([]string, 0, len(requestParts)-1)
	for index := 1; index < len(structParts); index++ {
		if typ.Kind() != reflect.Struct {
			return fe.Field()
		}

		fieldName, _, _ := strings.Cut(structParts[index], "[")
		structField, found := typ.FieldByName(fieldName)
		if !found {
			return fe.Field()
		}

		if !structField.Anonymous {
			path = append(path, requestParts[index])
		}

		typ = structField.Type
		for typ.Kind() == reflect.Pointer ||
			typ.Kind() == reflect.Slice ||
			typ.Kind() == reflect.Array {
			typ = typ.Elem()
		}
	}

	if len(path) == 0 {
		return fe.Field()
	}

	return indexReplacer.Replace(strings.Join(path, "."))
}

// indexReplacer turns "items[1]" into "items.1". A Replacer is safe for
// concurrent use, and building one per call dominated validation errors.
var indexReplacer = strings.NewReplacer("[", ".", "]", "")

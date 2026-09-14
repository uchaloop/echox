package echox_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/uchaloop/echox"
)

func makeValidator(t *testing.T) *echox.Validator {
	t.Helper()

	requestValidator, err := echox.MakeValidator()
	if err != nil {
		t.Fatalf("MakeValidator: %v", err)
	}

	return requestValidator
}

func TestValidatorUsesNativeTranslations(t *testing.T) {
	t.Parallel()

	err := makeValidator(t).Validate(&struct {
		Title  string            `json:"title" validate:"required"`
		Name   string            `json:"name" validate:"max=3"`
		Tags   []string          `json:"tags" validate:"min=2"`
		Status publicationStatus `json:"status" validate:"enum"`
	}{Name: "long", Tags: []string{"one"}, Status: "archived"})

	assertInvalidFields(t, err,
		echox.InvalidField{Path: "title", Code: "required", Message: "title is a required field"},
		echox.InvalidField{Path: "name", Code: "max", Message: "name must be a maximum of 3 characters in length"},
		echox.InvalidField{Path: "tags", Code: "min", Message: "tags must contain at least 2 items"},
		echox.InvalidField{Path: "status", Code: "enum", Message: "status must be one of the allowed values"},
	)
}

// PageQuery is embedded the way echox.ListInput embeds PageInput.
type PageQuery struct {
	Page int `query:"page" validate:"min=1"`
}

type author struct {
	Name string `json:"name" validate:"required"`
}

type requestPaths struct {
	PageQuery
	Author author   `json:"author"`
	Items  []author `json:"items" validate:"dive"`
}

// Paths use request names, skip embedded structs, and number slice items.
func TestValidatorReportsRequestPaths(t *testing.T) {
	t.Parallel()

	err := makeValidator(t).Validate(&requestPaths{Items: []author{{Name: "Ada"}, {}}})

	assertInvalidFields(t, err,
		echox.InvalidField{Path: "page", Code: "min", Message: "page must be 1 or greater"},
		echox.InvalidField{Path: "author.name", Code: "required", Message: "name is a required field"},
		echox.InvalidField{Path: "items.1.name", Code: "required", Message: "name is a required field"},
	)
}

// notblank is go-playground's non-standard implementation: separator control
// characters count as blank, and other kinds need a length or a non-zero value.
func TestValidatorNotBlankFollowsNativeImplementation(t *testing.T) {
	t.Parallel()

	requestValidator := makeValidator(t)
	err := requestValidator.Validate(&struct {
		Title string `json:"title" validate:"notblank"`
	}{Title: "   "})

	assertInvalidFields(t, err, echox.InvalidField{Path: "title", Code: "notblank", Message: "title must not be blank"})

	type (
		text struct {
			V string `validate:"notblank"`
		}
		list struct {
			V []string `validate:"notblank"`
		}
		number struct {
			V int `validate:"notblank"`
		}
	)

	for _, test := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "separators only", value: &text{V: "\x1c "}},
		{name: "text", value: &text{V: "text"}, valid: true},
		{name: "empty slice", value: &list{V: []string{}}},
		{name: "non-empty slice", value: &list{V: []string{"a"}}, valid: true},
		{name: "non-zero number", value: &number{V: 5}, valid: true},
	} {
		if valid := requestValidator.Validate(test.value) == nil; valid != test.valid {
			t.Errorf("%s: valid = %v, want %v", test.name, valid, test.valid)
		}
	}
}

func TestValidatorHidesInternalsOfUntranslatedTag(t *testing.T) {
	t.Parallel()

	requestValidator := makeValidator(t)
	if err := requestValidator.GoPlayground().RegisterValidation("even", func(fl validator.FieldLevel) bool {
		return fl.Field().Int()%2 == 0
	}); err != nil {
		t.Fatalf("register validation: %v", err)
	}

	err := requestValidator.Validate(&struct {
		Count int `json:"count" validate:"even"`
	}{Count: 3})

	assertInvalidFields(t, err, echox.InvalidField{Path: "count", Code: "even", Message: "count is invalid"})
}

func TestValidatorAcceptsApplicationTranslations(t *testing.T) {
	t.Parallel()

	requestValidator := makeValidator(t)
	validate, translator := requestValidator.GoPlayground(), requestValidator.Translator()
	if err := validate.RegisterValidation("even", func(fl validator.FieldLevel) bool {
		return fl.Field().Int()%2 == 0
	}); err != nil {
		t.Fatalf("register validation: %v", err)
	}

	if err := validate.RegisterTranslation("even", translator, func(trans ut.Translator) error {
		return trans.Add("even", "{0} must be even", false)
	}, func(trans ut.Translator, fe validator.FieldError) string {
		message, _ := trans.T("even", fe.Field())

		return message
	}); err != nil {
		t.Fatalf("register translation: %v", err)
	}

	err := requestValidator.Validate(&struct {
		Count int `json:"count" validate:"even"`
	}{Count: 3})

	assertInvalidFields(t, err, echox.InvalidField{Path: "count", Code: "even", Message: "count must be even"})
}

func assertInvalidFields(t *testing.T, err error, want ...echox.InvalidField) {
	t.Helper()

	validationErr, ok := errors.AsType[*echox.ValidationError](err)
	if !ok {
		t.Fatalf("error = %v, want *echox.ValidationError", err)
	}

	got := validationErr.InvalidFields()
	if !slices.Equal(got, want) {
		t.Fatalf("invalid fields = %+v, want %+v", got, want)
	}

	for _, field := range got {
		if strings.Contains(field.Message, "Key:") {
			t.Fatalf("message exposes validator internals: %q", field.Message)
		}
	}
}

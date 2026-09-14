package echox

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/httpx/problem"
)

// BindSource identifies the request source that could not be decoded.
type BindSource string

const (
	BindSourcePath   BindSource = "path"
	BindSourceQuery  BindSource = "query"
	BindSourceHeader BindSource = "header"
)

// sourceTags are the struct tags Echo binds each source from.
var sourceTags = map[BindSource]string{
	BindSourcePath:   "param",
	BindSourceQuery:  "query",
	BindSourceHeader: "header",
}

// BindError describes a failed Echo binding operation. Path is empty when the
// failed field is unknown.
type BindError struct {
	Source  BindSource
	Path    string
	Code    string
	Message string
	cause   error
}

func (e *BindError) Error() string {
	if e == nil {
		return "bind request values"
	}

	if e.cause == nil {
		return fmt.Sprintf("bind %s values", e.Source)
	}

	return fmt.Sprintf("bind %s values: %v", e.Source, e.cause)
}

// Unwrap returns the error produced by Echo's binder.
func (e *BindError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.cause
}

// BindPathAndValidate binds path parameters into T and validates *T through
// the validator configured on the Echo application.
func BindPathAndValidate[T any](ctx *echo.Context) (T, error) {
	return bindAndValidate[T](ctx, BindSourcePath, echo.BindPathValues)
}

// BindQueryAndValidate binds query parameters into T and validates *T through
// the validator configured on the Echo application. It does not bind path or
// body values.
func BindQueryAndValidate[T any](ctx *echo.Context) (T, error) {
	return bindAndValidate[T](ctx, BindSourceQuery, echo.BindQueryParams)
}

// BindHeadersAndValidate binds request headers into T and validates *T through
// the validator configured on the Echo application.
func BindHeadersAndValidate[T any](ctx *echo.Context) (T, error) {
	return bindAndValidate[T](ctx, BindSourceHeader, echo.BindHeaders)
}

func bindAndValidate[T any](
	ctx *echo.Context,
	source BindSource,
	bind func(*echo.Context, any) error,
) (T, error) {
	var value T

	if ctx == nil {
		return value, fmt.Errorf("context is nil")
	}

	if err := bind(ctx, &value); err != nil {
		return value, makeBindError(reflect.TypeFor[T](), source, err)
	}

	if err := ctx.Validate(&value); err != nil {
		return value, err
	}

	return value, nil
}

// makeBindError finds the field Echo failed to decode. Echo names it only in
// the text of the wrapped error, as "name: cause", so the name is trusted only
// when the bound type declares it for source. Otherwise Path stays empty.
func makeBindError(typ reflect.Type, source BindSource, err error) *BindError {
	bindErr := &BindError{Source: source, cause: err}

	cause := errors.Unwrap(err)
	if cause == nil {
		return bindErr
	}

	name, _, found := strings.Cut(cause.Error(), ": ")
	if !found {
		return bindErr
	}

	for fieldName, field := range taggedFields(typ, sourceTags[source]) {
		if fieldName == name {
			// Echo does not report which element of a slice failed.
			param := problem.TypeParam(name, elementType(field.Type))
			bindErr.Path, bindErr.Code, bindErr.Message = param.Path, param.Code, param.Message

			break
		}
	}

	return bindErr
}

// taggedFields yields the fields that bind from tag by name, including fields
// of embedded structs that have no tag of their own.
func taggedFields(typ reflect.Type, tag string) iter.Seq2[string, reflect.StructField] {
	return func(yield func(string, reflect.StructField) bool) {
		walkTaggedFields(typ, tag, yield)
	}
}

func walkTaggedFields(
	typ reflect.Type,
	tag string,
	yield func(string, reflect.StructField) bool,
) bool {
	typ = indirectType(typ)
	if typ.Kind() != reflect.Struct {
		return true
	}

	for index := range typ.NumField() {
		field := typ.Field(index)
		name, _, _ := strings.Cut(field.Tag.Get(tag), ",")
		if field.Anonymous && len(name) == 0 {
			if !walkTaggedFields(field.Type, tag, yield) {
				return false
			}

			continue
		}

		if len(name) == 0 || name == "-" {
			continue
		}

		if !yield(name, field) {
			return false
		}
	}

	return true
}

func indirectType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	return typ
}

func elementType(typ reflect.Type) reflect.Type {
	typ = indirectType(typ)
	if typ.Kind() == reflect.Slice {
		return indirectType(typ.Elem())
	}

	return typ
}

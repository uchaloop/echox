package echox

import (
	"fmt"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/httpx/request"
)

// DecodeJSON decodes one required JSON document from the Echo request.
// Its behavior and options are defined by httpx/request.DecodeJSON.
func DecodeJSON[T any](ctx *echo.Context, opts ...request.JSONOption) (T, error) {
	if ctx == nil {
		var value T

		return value, fmt.Errorf("context is nil")
	}

	return request.DecodeJSON[T](ctx.Request(), opts...)
}

// DecodeAndValidateJSON decodes JSON and validates *T through the validator
// configured on the Echo application.
func DecodeAndValidateJSON[T any](ctx *echo.Context, opts ...request.JSONOption) (T, error) {
	if ctx == nil {
		var value T

		return value, fmt.Errorf("context is nil")
	}

	return request.DecodeAndValidateJSON[T](ctx.Request(), ctx, opts...)
}

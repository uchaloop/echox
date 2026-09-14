package echox

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/httpx/request"
)

// Binder reads and validates one input from the request.
type Binder[T any] func(*echo.Context) (T, error)

// Handler starts a request-local sequence. Use a chain once, within its handler.
type Handler struct {
	ctx *echo.Context
	err error
}

// BoundHandler holds one input or the first bind error.
type BoundHandler[Input any] struct {
	handler Handler
	input   Input
}

// BoundHandler2 holds two ordered inputs or the first bind error.
type BoundHandler2[First, Second any] struct {
	handler Handler
	first   First
	second  Second
}

// MakeHandler starts an eager bind sequence. Terminals return its first error.
func MakeHandler(ctx *echo.Context) Handler {
	if ctx == nil {
		return Handler{err: fmt.Errorf("context is nil")}
	}

	return Handler{ctx: ctx}
}

// Bind reads one input. No binder runs after an earlier failure.
func (h Handler) Bind[T any](bind Binder[T]) BoundHandler[T] {
	var input T
	if h.err == nil {
		if bind == nil {
			h.err = fmt.Errorf("binder is nil")
		} else {
			input, h.err = bind(h.ctx)
		}
	}

	return BoundHandler[T]{handler: h, input: input}
}

// Bind appends a second input. No binder runs after an earlier failure.
func (h BoundHandler[First]) Bind[Second any](bind Binder[Second]) BoundHandler2[First, Second] {
	bound := h.handler.Bind(bind)

	return BoundHandler2[First, Second]{handler: bound.handler, first: h.input, second: bound.input}
}

// PositivePathID binds and validates the named request input.
func (h Handler) PositivePathID[ID positiveInteger]() BoundHandler[ID] {
	return h.Bind(BindPositivePathID[ID])
}

// Query binds and validates the named request input.
func (h Handler) Query[T any]() BoundHandler[T] { return h.Bind(BindQueryAndValidate[T]) }

// Headers binds and validates the named request input.
func (h Handler) Headers[T any]() BoundHandler[T] { return h.Bind(BindHeadersAndValidate[T]) }

// JSON decodes one JSON body and validates its tags.
func (h Handler) JSON[T any](opts ...request.JSONOption) BoundHandler[T] {
	return h.Bind(func(ctx *echo.Context) (T, error) { return DecodeAndValidateJSON[T](ctx, opts...) })
}

// PositivePathID binds and validates the named request input.
func (h BoundHandler[First]) PositivePathID[ID positiveInteger]() BoundHandler2[First, ID] {
	return h.Bind(BindPositivePathID[ID])
}

// Query binds and validates the named request input.
func (h BoundHandler[First]) Query[Second any]() BoundHandler2[First, Second] {
	return h.Bind(BindQueryAndValidate[Second])
}

// Headers binds and validates the named request input.
func (h BoundHandler[First]) Headers[Second any]() BoundHandler2[First, Second] {
	return h.Bind(BindHeadersAndValidate[Second])
}

// JSON decodes one JSON body and validates its tags.
func (h BoundHandler[First]) JSON[Second any](opts ...request.JSONOption) BoundHandler2[First, Second] {
	return h.Bind(func(ctx *echo.Context) (Second, error) { return DecodeAndValidateJSON[Second](ctx, opts...) })
}

// Data calls execute once, preserves its error, and writes the successful response.
func (h BoundHandler[Input]) Data[Result any](execute func(context.Context, Input) (Result, error)) error {
	if h.handler.err != nil {
		return h.handler.err
	}

	if execute == nil {
		return fmt.Errorf("execute function is nil")
	}

	result, err := execute(h.handler.ctx.Request().Context(), h.input)
	if err != nil {
		return err
	}

	return OK(h.handler.ctx, result)
}

// Created calls execute once, preserves its error, and writes the successful response.
func (h BoundHandler[Input]) Created[Result any](
	execute func(context.Context, Input) (Result, error),
	location func(Result) string,
) error {
	if h.handler.err != nil {
		return h.handler.err
	}

	if execute == nil {
		return fmt.Errorf("execute function is nil")
	}

	if location == nil {
		return fmt.Errorf("location function is nil")
	}

	result, err := execute(h.handler.ctx.Request().Context(), h.input)
	if err != nil {
		return err
	}

	return Created(h.handler.ctx, location(result), result)
}

// NoContent calls execute once, preserves its error, and writes the successful response.
func (h BoundHandler[Input]) NoContent(execute func(context.Context, Input) error) error {
	if h.handler.err != nil {
		return h.handler.err
	}

	if execute == nil {
		return fmt.Errorf("execute function is nil")
	}

	if err := execute(h.handler.ctx.Request().Context(), h.input); err != nil {
		return err
	}

	return h.handler.ctx.NoContent(http.StatusNoContent)
}

// Data calls execute once, preserves its error, and writes the successful response.
func (h BoundHandler2[First, Second]) Data[Result any](
	execute func(context.Context, First, Second) (Result, error),
) error {
	if h.handler.err != nil {
		return h.handler.err
	}

	if execute == nil {
		return fmt.Errorf("execute function is nil")
	}

	result, err := execute(h.handler.ctx.Request().Context(), h.first, h.second)
	if err != nil {
		return err
	}

	return OK(h.handler.ctx, result)
}

// Created calls execute once, preserves its error, and writes the successful response.
func (h BoundHandler2[First, Second]) Created[Result any](
	execute func(context.Context, First, Second) (Result, error),
	location func(Result) string,
) error {
	if h.handler.err != nil {
		return h.handler.err
	}

	if execute == nil {
		return fmt.Errorf("execute function is nil")
	}

	if location == nil {
		return fmt.Errorf("location function is nil")
	}

	result, err := execute(h.handler.ctx.Request().Context(), h.first, h.second)
	if err != nil {
		return err
	}

	return Created(h.handler.ctx, location(result), result)
}

// NoContent calls execute once, preserves its error, and writes the successful response.
func (h BoundHandler2[First, Second]) NoContent(execute func(context.Context, First, Second) error) error {
	if h.handler.err != nil {
		return h.handler.err
	}

	if execute == nil {
		return fmt.Errorf("execute function is nil")
	}

	if err := execute(h.handler.ctx.Request().Context(), h.first, h.second); err != nil {
		return err
	}

	return h.handler.ctx.NoContent(http.StatusNoContent)
}

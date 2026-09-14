package echox

import (
	"fmt"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/httpx/page"
	"github.com/uchaloop/httpx/response"
)

// Data writes value inside a {"data": ...} JSON envelope.
func Data(ctx *echo.Context, status int, value any) error {
	return Write(ctx, response.Data(status, value))
}

// OK writes a data envelope with status 200, including for a nil value.
func OK(ctx *echo.Context, value any) error {
	return Write(ctx, response.OK(value))
}

// Created writes value inside a data envelope with status 201 and sets the
// Location response header.
func Created(ctx *echo.Context, location string, value any) error {
	return Write(ctx, response.Created(location, value))
}

// Page writes a conventional paginated data envelope. A nil items slice is
// encoded as an empty JSON array.
func Page[Item any](ctx *echo.Context, items []Item, total uint64, current page.Page) error {
	return Write(ctx, response.Page(items, total, current))
}

// Write is the Echo backend for httpx responses. A body goes through the
// application's JSON serializer, which delays the status until it succeeds.
func Write(ctx *echo.Context, resp response.Response) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}

	if err := resp.Validate(); err != nil {
		return err
	}

	resp.SetHeaders(ctx.Response().Header())
	if len(resp.ContentType) == 0 {
		return ctx.NoContent(resp.Status)
	}

	return ctx.JSON(resp.Status, resp.Body)
}

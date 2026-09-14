package echox

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Ping handles a liveness request with an empty 204 response. The service
// remains responsible for registering it on GET /ping.
func Ping(ctx *echo.Context) error {
	return ctx.NoContent(http.StatusNoContent)
}

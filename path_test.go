package echox_test

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/problem"
)

func TestBindPositivePathID(t *testing.T) {
	t.Parallel()

	app := makeBindApp(t)
	app.GET("/articles/:id", func(ctx *echo.Context) error {
		id, err := echox.BindPositivePathID[int64](ctx)
		if err != nil {
			return err
		}

		return ctx.JSON(http.StatusOK, map[string]int64{"id": id})
	})

	api := apitest.Make(t, app)
	api.Get("/articles/42").Do().Status(http.StatusOK).JSONEqual(map[string]int{"id": 42})
	api.Get("/articles/0").Do().
		Status(http.StatusUnprocessableEntity).
		Header("Content-Type", problem.ContentType).
		JSONEqual(invalidInput("/articles/0", invalidParam("id", "min", "id must be 1 or greater")))

	api.Get("/articles/abc").Do().
		Status(http.StatusBadRequest).
		Header("Content-Type", problem.ContentType).
		JSONEqual(undecodableInput("/articles/abc", invalidParam("id", "type", "id must be an integer")))
}

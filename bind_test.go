package echox_test

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/problem"
)

type publicationStatus string

func (s publicationStatus) Valid() bool {
	return s == "draft" || s == "published"
}

type pathInput struct {
	ID uint64 `param:"id" validate:"required,min=1"`
}

type queryInput struct {
	Status *publicationStatus `query:"status" validate:"omitempty,enum"`
	Page   *uint64            `query:"page" validate:"omitempty,min=1"`
	Sort   []string           `query:"sort"`
}

type headerInput struct {
	RequestID string `header:"X-Request-ID" validate:"required"`
}

func TestBindHelpersUseOnlyTheirNamedSource(t *testing.T) {
	t.Parallel()

	app := makeBindApp(t)
	app.GET("/articles/:id", func(ctx *echo.Context) error {
		path, err := echox.BindPathAndValidate[pathInput](ctx)
		if err != nil {
			return err
		}

		query, err := echox.BindQueryAndValidate[queryInput](ctx)
		if err != nil {
			return err
		}

		headers, err := echox.BindHeadersAndValidate[headerInput](ctx)
		if err != nil {
			return err
		}

		return ctx.JSON(http.StatusOK, map[string]any{
			"id":        path.ID,
			"status":    query.Status,
			"page":      query.Page,
			"sort":      query.Sort,
			"requestId": headers.RequestID,
		})
	})

	apitest.Make(t, app).Get("/articles/42").
		Query("status", "published").
		Query("page", "3").
		Query("sort", "updatedAt:desc", "title").
		Header("X-Request-ID", "request-17").
		Do().
		Status(http.StatusOK).
		JSONEqual(map[string]any{
			"id":        42,
			"status":    "published",
			"page":      3,
			"sort":      []string{"updatedAt:desc", "title"},
			"requestId": "request-17",
		})
}

func TestBindDecodeErrorNamesTheField(t *testing.T) {
	t.Parallel()

	type input struct {
		Page  *uint64  `query:"page"`
		IDs   []uint64 `query:"ids"`
		Count int      `header:"X-Count"`
	}

	app := makeBindApp(t)
	app.GET("/query", func(ctx *echo.Context) error {
		_, err := echox.BindQueryAndValidate[input](ctx)

		return err
	})

	app.GET("/header", func(ctx *echo.Context) error {
		_, err := echox.BindHeadersAndValidate[input](ctx)

		return err
	})

	// Echo names the failed field only in its error text; these cases pin the
	// format it uses for each source. The path source is pinned in path_test.
	api := apitest.Make(t, app)
	api.Get("/query?page=abc").Do().
		Status(http.StatusBadRequest).
		Header("Content-Type", problem.ContentType).
		JSONEqual(undecodableInput("/query", invalidParam("page", "type", "page must be a non-negative integer")))

	api.Get("/query?ids=1&ids=x").Do().
		Status(http.StatusBadRequest).
		JSONEqual(undecodableInput("/query", invalidParam("ids", "type", "ids must be a non-negative integer")))

	api.Get("/header").Header("X-Count", "many").Do().
		Status(http.StatusBadRequest).
		JSONEqual(undecodableInput("/header", invalidParam("X-Count", "type", "X-Count must be an integer")))
}

func TestBindHeadersValidatesMissingHeader(t *testing.T) {
	t.Parallel()

	app := makeBindApp(t)
	app.GET("/articles", func(ctx *echo.Context) error {
		_, err := echox.BindHeadersAndValidate[headerInput](ctx)

		return err
	})

	apitest.Make(t, app).Get("/articles").Do().
		Status(http.StatusUnprocessableEntity).
		Header("Content-Type", problem.ContentType).
		JSONEqual(invalidInput("/articles", invalidParam("X-Request-ID", "required", "X-Request-ID is a required field")))
}

func makeBindApp(t *testing.T) *echo.Echo {
	t.Helper()

	mapper, err := problem.MakeMapper(
		problem.InputProblemRule(),
		echox.BindProblemRule(),
	)
	if err != nil {
		t.Fatalf("make mapper: %v", err)
	}

	requestValidator, err := echox.MakeValidator()
	if err != nil {
		t.Fatalf("make validator: %v", err)
	}

	app := echo.New()
	app.Validator = requestValidator
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)

	return app
}

// invalidInput is the 422 document for input that fails validation.
func invalidInput(instance string, errors ...map[string]any) map[string]any {
	return inputProblem(http.StatusUnprocessableEntity, "Request parameters are invalid", instance, errors)
}

// undecodableInput is the 400 document for input that cannot be decoded.
func undecodableInput(instance string, errors ...map[string]any) map[string]any {
	return inputProblem(http.StatusBadRequest, "Request parameters could not be decoded", instance, errors)
}

func inputProblem(status int, detail, instance string, errors []map[string]any) map[string]any {
	document := map[string]any{
		"type":     "about:blank",
		"title":    http.StatusText(status),
		"status":   status,
		"detail":   detail,
		"code":     "invalid_request",
		"instance": instance,
	}

	if len(errors) > 0 {
		document["errors"] = errors
	}

	return document
}

func invalidParam(path, code, message string) map[string]any {
	return map[string]any{"path": path, "code": code, "message": message}
}

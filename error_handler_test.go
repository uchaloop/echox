package echox_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/problem"
)

var errArticleNotFound = errors.New("article not found")

// internalProblem is the only document a client sees for an unexpected error.
func internalProblem(instance string) map[string]any {
	return map[string]any{
		"type":     "about:blank",
		"title":    "Internal Server Error",
		"status":   http.StatusInternalServerError,
		"code":     "internal_error",
		"instance": instance,
	}
}

func TestErrorHandlerMapsApplicationError(t *testing.T) {
	t.Parallel()

	mapper, err := problem.MakeMapper(problem.WhenIs(errArticleNotFound, problem.Template{
		Status: http.StatusNotFound,
		Code:   "article_not_found",
		Detail: "Article was not found.",
	}))
	if err != nil {
		t.Fatalf("make mapper: %v", err)
	}

	app := echo.New()
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
	app.GET("/articles/:id", func(*echo.Context) error {
		return errArticleNotFound
	})

	apitest.Make(t, app).Get("/articles/42").Do().
		Status(http.StatusNotFound).
		Header("Content-Type", problem.ContentType).
		JSONEqual(map[string]any{
			"type":     "about:blank",
			"title":    "Not Found",
			"status":   http.StatusNotFound,
			"code":     "article_not_found",
			"detail":   "Article was not found.",
			"instance": "/articles/42",
		})
}

func TestErrorHandlerMapsNativeEchoError(t *testing.T) {
	t.Parallel()

	mapper, err := problem.MakeMapper()
	if err != nil {
		t.Fatalf("make mapper: %v", err)
	}

	app := echo.New()
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)

	// A native Echo error has no stable code and no detail.
	apitest.Make(t, app).Get("/missing").Do().
		Status(http.StatusNotFound).
		Header("Content-Type", problem.ContentType).
		JSONEqual(map[string]any{
			"type":     "about:blank",
			"title":    "Not Found",
			"status":   http.StatusNotFound,
			"instance": "/missing",
		})
}

func TestErrorHandlerDoesNotExposeUnsafeHTTPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "internal", err: echo.NewHTTPError(http.StatusInternalServerError, "database password is secret")},
		{name: "non-standard status", err: echo.NewHTTPError(599, "database password is secret")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mapper, err := problem.MakeMapper()
			if err != nil {
				t.Fatalf("make mapper: %v", err)
			}

			app := echo.New()
			app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
			app.GET("/failure", func(*echo.Context) error {
				return test.err
			})

			apitest.Make(t, app).Get("/failure").Do().
				Status(http.StatusInternalServerError).
				Header("Content-Type", problem.ContentType).
				JSONEqual(internalProblem("/failure"))
		})
	}
}

func TestErrorHandlerDoesNotReplaceCommittedResponse(t *testing.T) {
	t.Parallel()

	mapper, err := problem.MakeMapper()
	if err != nil {
		t.Fatalf("make mapper: %v", err)
	}

	app := echo.New()
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
	app.GET("/committed", func(ctx *echo.Context) error {
		if err := ctx.JSON(http.StatusAccepted, map[string]bool{"accepted": true}); err != nil {
			return err
		}

		return errors.New("late failure")
	})

	apitest.Make(t, app).Get("/committed").Do().
		Status(http.StatusAccepted).
		JSONEqual(map[string]bool{"accepted": true})
}

func TestErrorHandlerOmitsBodyForHead(t *testing.T) {
	t.Parallel()

	mapper, err := problem.MakeMapper()
	if err != nil {
		t.Fatalf("make mapper: %v", err)
	}

	app := echo.New()
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
	app.HEAD("/articles/:id", func(*echo.Context) error {
		return echo.ErrNotFound
	})

	// net/http drops HEAD bodies on the wire, so only a recorder shows
	// whether the error handler wrote one. The headers match what GET gets.
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/articles/42", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != problem.ContentType {
		t.Errorf("Content-Type = %q, want %q", contentType, problem.ContentType)
	}

	if recorder.Body.Len() > 0 {
		t.Errorf("HEAD body = %q, want empty", recorder.Body.String())
	}
}

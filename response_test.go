package echox_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/page"
	"github.com/uchaloop/httpx/problem"
	"github.com/uchaloop/httpx/response"
)

// The envelopes themselves are httpx contracts; these cases check that the
// Echo backend writes status, headers and body of each response kind.
func TestWriteSendsHTTPXResponses(t *testing.T) {
	t.Parallel()

	currentPage, err := page.Make(page.Params{}, page.Config{DefaultSize: 20, MaxSize: 100})
	if err != nil {
		t.Fatalf("make page: %v", err)
	}

	app := echo.New()
	app.GET("/data", func(ctx *echo.Context) error { return echox.Data(ctx, http.StatusAccepted, map[string]int{"id": 42}) })
	app.POST("/created", func(ctx *echo.Context) error { return echox.Created(ctx, "/articles/42", map[string]int{"id": 42}) })
	app.GET("/page", func(ctx *echo.Context) error { return echox.Page[map[string]any](ctx, nil, 17, currentPage) })
	app.DELETE("/empty", func(ctx *echo.Context) error { return echox.Write(ctx, response.NoContent()) })

	api := apitest.Make(t, app)
	api.Get("/data").Do().
		Status(http.StatusAccepted).
		Header("Content-Type", "application/json").
		JSONEqual(map[string]any{"data": map[string]int{"id": 42}})

	api.Post("/created").Do().
		Status(http.StatusCreated).
		Header("Location", "/articles/42").
		JSONEqual(map[string]any{"data": map[string]int{"id": 42}})

	api.Get("/page").Do().
		Status(http.StatusOK).
		JSONEqual(map[string]any{"data": map[string]any{"items": []any{}, "page": 1, "size": 20, "total": 17}})

	api.Delete("/empty").Do().
		Status(http.StatusNoContent).
		HeaderAbsent("Content-Type").
		BodyEqual("")
}

// The Echo backend writes custom headers under the same canonical names as
// net/http, replacing what a middleware already set.
func TestWriteCanonicalizesCustomHeaders(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), recorder)
	c.Response().Header().Set("X-User", "middleware")
	answer := response.OK(map[string]int{"id": 42})
	answer.Header = http.Header{"x-user": {"42"}}

	if err := echox.Write(c, answer); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := recorder.Header().Values("X-User"); !slices.Equal(got, []string{"42"}) {
		t.Fatalf("X-User = %q, want the response's single value", got)
	}
}

func TestWriteRejectsInvalidResponseBeforeWriting(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), recorder)
	if err := echox.Created(c, "", map[string]int{"id": 42}); err == nil {
		t.Fatal("Created without a location succeeded")
	}

	if recorder.Code != http.StatusOK || recorder.Body.Len() > 0 || len(recorder.Header()) > 0 {
		t.Fatalf("response changed: status %d, header %v, body %q", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

// serializerSpy is an application's own JSON serializer.
type serializerSpy struct {
	echo.DefaultJSONSerializer

	calls atomic.Int64
}

func (s *serializerSpy) Serialize(ctx *echo.Context, target any, indent string) error {
	s.calls.Add(1)

	return s.DefaultJSONSerializer.Serialize(ctx, target, indent)
}

func TestResponsesUseApplicationJSONSerializer(t *testing.T) {
	t.Parallel()

	mapper, err := problem.MakeMapper()
	if err != nil {
		t.Fatalf("make mapper: %v", err)
	}

	serializer := &serializerSpy{}
	app := echo.New()
	app.JSONSerializer = serializer
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
	app.GET("/article", func(ctx *echo.Context) error {
		return echox.OK(ctx, map[string]int{"id": 42})
	})

	api := apitest.Make(t, app)
	api.Get("/article").Do().Status(http.StatusOK)
	api.Get("/missing").Do().Status(http.StatusNotFound).Header("Content-Type", problem.ContentType)
	if calls := serializer.calls.Load(); calls != 2 {
		t.Fatalf("serializer calls = %d, want the data and the problem response", calls)
	}
}

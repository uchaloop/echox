package echox_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
)

func TestHandlerStopsAtFirstFailedBinding(t *testing.T) {
	t.Parallel()

	cause := errors.New("bad input")
	t.Run("first input", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), recorder)
		err := echox.MakeHandler(c).
			Bind(func(*echo.Context) (int, error) { return 0, cause }).
			Bind(func(*echo.Context) (string, error) { t.Fatal("second binder called"); return "", nil }).
			Data(func(context.Context, int, string) (int, error) { t.Fatal("service called"); return 0, nil })
		if err != cause || recorder.Body.Len() > 0 {
			t.Fatalf("error = %v, body = %s; want the original error and no response", err, recorder.Body.String())
		}
	})

	t.Run("second input", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), recorder)
		err := echox.MakeHandler(c).
			Bind(func(*echo.Context) (int, error) { return 4, nil }).
			Bind(func(*echo.Context) (string, error) { return "", cause }).
			Created(func(context.Context, int, string) (int, error) { t.Fatal("service called"); return 0, nil },
				func(int) string { t.Fatal("location called"); return "/items/4" })
		if err != cause || recorder.Body.Len() > 0 {
			t.Fatalf("error = %v, body = %s; want the original error and no response", err, recorder.Body.String())
		}
	})
}

func TestHandlerPreservesServiceErrorAndRequestContext(t *testing.T) {
	t.Parallel()

	cause := errors.New("domain failure")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx), recorder)
	err := echox.MakeHandler(c).
		Bind(func(*echo.Context) (int, error) { return 8, nil }).
		Created(func(got context.Context, id int) (int, error) {
			if got != ctx || got.Err() != context.Canceled || id != 8 {
				t.Fatal("request context or value lost")
			}

			return 0, cause
		}, func(int) string { t.Fatal("location called on failure"); return "" })
	if err != cause || recorder.Body.Len() > 0 {
		t.Fatalf("error = %v, body = %s", err, recorder.Body.String())
	}
}

func TestHandlerBindsQueryAndHeadersInOrder(t *testing.T) {
	t.Parallel()

	app, err := echox.Make(echox.Config{})
	if err != nil {
		t.Fatal(err)
	}

	type query struct {
		ID int `query:"id" validate:"min=1"`
	}

	type headers struct {
		Token string `header:"X-Token" validate:"required"`
	}

	app.GET("/", func(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).Query[query]().Headers[headers]().
			Data(func(_ context.Context, q query, h headers) (string, error) {
				// The handler runs in a server goroutine, where Fatal is not allowed.
				if q.ID != 7 || h.Token != "abc" {
					t.Error("inputs lost or reversed")
				}

				return h.Token, nil
			})
	})

	apitest.Make(t, app).Get("/?id=7").Header("X-Token", "abc").Do().
		Status(http.StatusOK).
		JSONEqual(map[string]string{"data": "abc"})
}

func TestHandlerPathFailureSkipsJSONBody(t *testing.T) {
	t.Parallel()

	app, err := echox.Make(echox.Config{})
	if err != nil {
		t.Fatal(err)
	}

	app.PATCH("/:id", func(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).PositivePathID[int64]().JSON[struct{}]().
			NoContent(func(context.Context, int64, struct{}) error { t.Fatal("service called"); return nil })
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/0", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want path validation before invalid JSON", recorder.Code)
	}
}

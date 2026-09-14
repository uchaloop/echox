package echox_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/page"
	"github.com/uchaloop/httpx/problem"
	"github.com/uchaloop/httpx/sortby"
)

type paginatedQuery struct {
	Filter string `query:"filter"`
}

type listSortField string

const (
	listSortByID    listSortField = "id"
	listSortByTitle listSortField = "title"
)

func (f *listSortField) FromString(value string) error {
	parsed := listSortField(value)
	if parsed != listSortByID && parsed != listSortByTitle {
		return fmt.Errorf("unsupported field %q", value)
	}

	*f = parsed

	return nil
}

type listQueryInput struct {
	Filter string `query:"filter"`
}

func makeRequests(t *testing.T, size uint64) *echox.Requests {
	t.Helper()

	requests, err := echox.MakeRequests(echox.RequestConfig{
		Page: page.Config{DefaultSize: size, MaxSize: 100, MaxOffset: 100},
	})
	if err != nil {
		t.Fatal(err)
	}

	return requests
}

func TestPageQueryBindsAndUsesProvidedPolicy(t *testing.T) {
	t.Parallel()

	app := makeBindApp(t)
	app.GET("/articles", func(ctx *echo.Context) error {
		query, err := makeRequests(t, 25).BindPageQuery[paginatedQuery](ctx)
		if err != nil {
			return err
		}

		return ctx.JSON(http.StatusOK, map[string]any{
			"filter": query.Filters.Filter,
			"page":   query.Page.Number(),
			"size":   query.Page.Size(),
			"offset": query.Page.Offset(),
		})
	})

	apitest.Make(t, app).Get("/articles?filter=recent&page=3&size=20").Do().
		Status(http.StatusOK).
		JSONEqual(map[string]any{"filter": "recent", "page": 3, "size": 20, "offset": 40})
}

// Each API version keeps its own page policy, as example-api's v1 and v2 do.
func TestRequestsKeepsPoliciesIndependent(t *testing.T) {
	t.Parallel()

	app := makeBindApp(t)
	for _, size := range []uint64{10, 20} {
		requests := makeRequests(t, size)
		app.GET(fmt.Sprintf("/%d", size), func(ctx *echo.Context) error {
			input, err := requests.BindPageQuery[paginatedQuery](ctx)
			if err != nil {
				return err
			}

			return ctx.JSON(http.StatusOK, input.Page.Size())
		})
	}

	api := apitest.Make(t, app)
	api.Get("/10").Do().Status(http.StatusOK).JSONEqual(10)
	api.Get("/20").Do().Status(http.StatusOK).JSONEqual(20)
}

func TestListQueryBindsPageAndRepeatedSortValues(t *testing.T) {
	t.Parallel()

	app := makeBindApp(t)
	app.GET("/articles", func(ctx *echo.Context) error {
		query, err := makeRequests(t, 10).BindListQuery[listQueryInput, listSortField, *listSortField](ctx)
		if err != nil {
			return err
		}

		return ctx.JSON(http.StatusOK, map[string]any{
			"filter":     query.Filters.Filter,
			"page":       query.Page.Number(),
			"firstSort":  query.Sort[0].Field,
			"descending": query.Sort[0].Direction == sortby.Descending,
			"secondSort": query.Sort[1].Field,
		})
	})

	apitest.Make(t, app).Get("/articles?filter=recent&page=2&sort=id:desc&sort=title").Do().
		Status(http.StatusOK).
		JSONEqual(map[string]any{
			"filter":     "recent",
			"page":       2,
			"firstSort":  "id",
			"descending": true,
			"secondSort": "title",
		})
}

func TestRequestsRejectsInvalidPolicy(t *testing.T) {
	t.Parallel()

	for _, config := range []page.Config{{}, {DefaultSize: 20, MaxSize: 10}} {
		if _, err := echox.MakeRequests(echox.RequestConfig{Page: config}); err == nil {
			t.Fatalf("accepted policy %+v", config)
		}
	}
}

func TestRequestsRejectsInvalidControlsAndFilters(t *testing.T) {
	t.Parallel()

	type filters struct {
		Name string `query:"name" validate:"omitempty,max=3"`
	}

	app := makeBindApp(t)
	requests := makeRequests(t, 10)
	app.GET("/", func(ctx *echo.Context) error {
		_, err := requests.BindListQuery[filters, listSortField, *listSortField](ctx)
		return err
	})

	tests := []struct {
		query    string
		status   int
		expected map[string]any
	}{
		{"page=0", 422, invalidInput("/", invalidParam("page", "min", "page must be 1 or greater"))},
		{"size=0", 422, invalidInput("/", invalidParam("size", "min", "size must be 1 or greater"))},
		{"size=101", 422, invalidInput("/", invalidParam("size", "max", "size must be 100 or less"))},
		{"page=12", 422, invalidInput("/", invalidParam("page", "max", "page must be 11 or less"))},
		{"name=long", 422, invalidInput("/", invalidParam("name", "max", "name must be a maximum of 3 characters in length"))},
		{"sort=id:sideways", 422, invalidInput("/", invalidParam("sort.0", "enum", "sort[0] must be one of the allowed values"))},
		{"sort=id&sort=secret:desc", 422, invalidInput("/", invalidParam("sort.1", "enum", "sort[1] must be one of the allowed values"))},
	}

	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			apitest.Make(t, app).Get("/?"+test.query).Do().
				Status(test.status).
				Header("Content-Type", problem.ContentType).
				JSONEqual(test.expected)
		})
	}
}

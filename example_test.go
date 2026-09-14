package echox_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/page"
	"github.com/uchaloop/httpx/problem"
	"github.com/uchaloop/httpx/response"
)

var errNotFound = errors.New("article not found")

type articleStatus string

// Valid makes the status accepted by the enum validation tag.
func (s articleStatus) Valid() bool {
	return s == "draft" || s == "published"
}

type article struct {
	ID     int64         `json:"id"`
	Title  string        `json:"title"`
	Status articleStatus `json:"status"`
}

type createArticle struct {
	Title  string        `json:"title" validate:"notblank,max=200"`
	Status articleStatus `json:"status" validate:"required,enum"`
}

// articleService stands in for the application's service.
type articleService struct{}

func (articleService) Get(_ context.Context, id int64) (article, error) {
	if id != 42 {
		return article{}, fmt.Errorf("get article %d: %w", id, errNotFound)
	}

	return article{ID: 42, Title: "HTTP boundaries", Status: "published"}, nil
}

func (articleService) Create(_ context.Context, input createArticle) (article, error) {
	return article{ID: 42, Title: input.Title, Status: input.Status}, nil
}

type articleSort string

const (
	sortByTitle     articleSort = "title"
	sortByUpdatedAt articleSort = "updatedAt"
)

// FromString lets BindListQuery parse a sort field; the compiler requires it.
func (s *articleSort) FromString(value string) error {
	switch parsed := articleSort(value); parsed {
	case sortByTitle, sortByUpdatedAt:
		*s = parsed

		return nil
	default:
		return fmt.Errorf("unsupported sort field %q", value)
	}
}

type articleFilters struct {
	Status *articleStatus `query:"status" validate:"omitempty,enum"`
}

func ExampleMake() {
	app, err := echox.Make(echox.Config{
		ProblemRules: []problem.Rule{
			problem.WhenIs(errNotFound, problem.Template{
				Status: http.StatusNotFound,
				Code:   "article_not_found",
				Detail: "Article was not found",
			}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	service := articleService{}
	app.GET("/articles/:id", func(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).PositivePathID[int64]().Data(service.Get)
	})

	for _, target := range []string{"/articles/42", "/articles/7", "/ping"} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	// Output:
	// 200 {"data":{"id":42,"title":"HTTP boundaries","status":"published"}}
	// 404 {"type":"about:blank","title":"Not Found","status":404,"detail":"Article was not found","instance":"/articles/7","code":"article_not_found"}
	// 204
}

func ExampleConfig_bodyLimit() {
	app, err := echox.Make(echox.Config{BodyLimit: 64})
	if err != nil {
		log.Fatal(err)
	}

	service := articleService{}
	app.POST("/articles", func(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).JSON[createArticle]().Data(service.Create)
	})

	body := `{"title":"` + strings.Repeat("a", 100) + `","status":"draft"}`
	r := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, r)
	fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))
	// Output:
	// 413 {"type":"about:blank","title":"Request Entity Too Large","status":413,"instance":"/articles"}
}

func ExampleMakeHandler() {
	app, err := echox.Make(echox.Config{})
	if err != nil {
		log.Fatal(err)
	}

	service := articleService{}
	app.POST("/articles", func(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).
			JSON[createArticle]().
			Created(service.Create, func(created article) string {
				return fmt.Sprintf("/articles/%d", created.ID)
			})
	})

	for _, body := range []string{
		`{"title":"HTTP boundaries","status":"draft"}`,
		`{"title":" ","status":"archived"}`,
	} {
		r := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, r)
		fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))

		if location := recorder.Header().Get("Location"); len(location) > 0 {
			fmt.Println("Location:", location)
		}
	}
	// Output:
	// 201 {"data":{"id":42,"title":"HTTP boundaries","status":"draft"}}
	// Location: /articles/42
	// 422 {"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"Request parameters are invalid","instance":"/articles","code":"invalid_request","errors":[{"path":"title","code":"notblank","message":"title must not be blank"},{"path":"status","code":"enum","message":"status must be one of the allowed values"}]}
}

func ExampleHandler_PositivePathID() {
	app, err := echox.Make(echox.Config{})
	if err != nil {
		log.Fatal(err)
	}

	service := articleService{}
	app.GET("/articles/:id", func(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).PositivePathID[int64]().Data(service.Get)
	})

	for _, target := range []string{"/articles/42", "/articles/0", "/articles/abc"} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	// Output:
	// 200 {"data":{"id":42,"title":"HTTP boundaries","status":"published"}}
	// 422 {"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"Request parameters are invalid","instance":"/articles/0","code":"invalid_request","errors":[{"path":"id","code":"min","message":"id must be 1 or greater"}]}
	// 400 {"type":"about:blank","title":"Bad Request","status":400,"detail":"Request parameters could not be decoded","instance":"/articles/abc","code":"invalid_request","errors":[{"path":"id","code":"type","message":"id must be an integer"}]}
}

func ExampleBindQueryAndValidate() {
	type searchQuery struct {
		Text string   `query:"q" json:"q" validate:"required"`
		Tags []string `query:"tag" json:"tags"`
	}

	app, err := echox.Make(echox.Config{})
	if err != nil {
		log.Fatal(err)
	}

	app.GET("/search", func(ctx *echo.Context) error {
		query, err := echox.BindQueryAndValidate[searchQuery](ctx)
		if err != nil {
			return err
		}

		return echox.OK(ctx, query)
	})

	for _, target := range []string{
		"/search?q=http&tag=go&tag=api",
		"/search?q=http&q=api&tag=go", // a scalar field takes the first value
		"/search?tag=go",
	} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	// Output:
	// 200 {"data":{"q":"http","tags":["go","api"]}}
	// 200 {"data":{"q":"http","tags":["go"]}}
	// 422 {"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"Request parameters are invalid","instance":"/search","code":"invalid_request","errors":[{"path":"q","code":"required","message":"q is a required field"}]}
}

func ExampleRequests_BindListQuery() {
	app, err := echox.Make(echox.Config{})
	if err != nil {
		log.Fatal(err)
	}

	requests, err := echox.MakeRequests(echox.RequestConfig{
		Page: page.Config{DefaultSize: 20, MaxSize: 100},
	})
	if err != nil {
		log.Fatal(err)
	}

	app.GET("/articles", func(ctx *echo.Context) error {
		query, err := requests.BindListQuery[articleFilters, articleSort](ctx)
		if err != nil {
			return err
		}

		fmt.Println("status:", *query.Filters.Status)
		fmt.Println("sort:", query.Sort)
		fmt.Println("page:", query.Page.Number(), "size:", query.Page.Size())

		articles := []article{{ID: 42, Title: "HTTP boundaries", Status: "draft"}}

		return echox.Page(ctx, articles, 21, query.Page)
	})

	for _, target := range []string{
		"/articles?status=draft&sort=updatedAt:desc&sort=title&page=2",
		"/articles?sort=author",
		"/articles?page=x",
	} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	// Output:
	// status: draft
	// sort: [{updatedAt desc} {title asc}]
	// page: 2 size: 20
	// 200 {"data":{"items":[{"id":42,"title":"HTTP boundaries","status":"draft"}],"total":21,"page":2,"size":20}}
	// 422 {"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"Request parameters are invalid","instance":"/articles","code":"invalid_request","errors":[{"path":"sort.0","code":"enum","message":"sort[0] must be one of the allowed values"}]}
	// 400 {"type":"about:blank","title":"Bad Request","status":400,"detail":"Request parameters could not be decoded","instance":"/articles","code":"invalid_request","errors":[{"path":"page","code":"type","message":"page must be a non-negative integer"}]}
}

func ExampleValidator_Translator() {
	type createTag struct {
		Slug string `json:"slug" validate:"slug"`
	}

	requestValidator, err := echox.MakeValidator()
	if err != nil {
		log.Fatal(err)
	}

	slug := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	playground := requestValidator.GoPlayground()

	err = playground.RegisterValidation("slug", func(fl validator.FieldLevel) bool {
		return slug.MatchString(fl.Field().String())
	})
	if err != nil {
		log.Fatal(err)
	}

	err = playground.RegisterTranslation(
		"slug",
		requestValidator.Translator(),
		func(trans ut.Translator) error {
			return trans.Add("slug", "{0} must be a lowercase slug", false)
		},
		func(trans ut.Translator, fe validator.FieldError) string {
			message, _ := trans.T("slug", fe.Field())

			return message
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	app, err := echox.Make(echox.Config{Validator: requestValidator})
	if err != nil {
		log.Fatal(err)
	}

	app.POST("/tags", func(ctx *echo.Context) error {
		input, err := echox.DecodeAndValidateJSON[createTag](ctx)
		if err != nil {
			return err
		}

		return echox.Created(ctx, "/tags/"+input.Slug, input)
	})

	r := httptest.NewRequest(http.MethodPost, "/tags", strings.NewReader(`{"slug":"Go Tips"}`))
	r.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, r)
	fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))
	// Output:
	// 422 {"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"Request parameters are invalid","instance":"/tags","code":"invalid_request","errors":[{"path":"slug","code":"slug","message":"slug must be a lowercase slug"}]}
}

func ExampleWrite() {
	app, err := echox.Make(echox.Config{})
	if err != nil {
		log.Fatal(err)
	}

	app.GET("/profile", func(ctx *echo.Context) error {
		answer := response.OK(map[string]string{"name": "Ada"})
		answer.Header = http.Header{"x-user": {"7"}}

		return echox.Write(ctx, answer)
	})

	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/profile", nil))
	fmt.Println(recorder.Code, "X-User:", recorder.Header().Get("X-User"))
	fmt.Println(strings.TrimSpace(recorder.Body.String()))
	// Output:
	// 200 X-User: 7
	// {"data":{"name":"Ada"}}
}

func ExampleMakeErrorHandler() {
	mapper, err := echox.MakeProblemMapper(problem.WhenIs(errNotFound, problem.Template{
		Status: http.StatusNotFound,
		Code:   "article_not_found",
	}))
	if err != nil {
		log.Fatal(err)
	}

	// An application composed by hand takes only the error handler.
	app := echo.New()
	app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
	app.GET("/articles/:id", func(*echo.Context) error {
		return fmt.Errorf("get article: %w", errNotFound)
	})

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(method, "/articles/42", nil))
		fmt.Println(recorder.Code, strings.TrimSpace(recorder.Body.String()))

		if allow := recorder.Header().Get("Allow"); len(allow) > 0 {
			fmt.Println("Allow:", allow)
		}
	}
	// Output:
	// 404 {"type":"about:blank","title":"Not Found","status":404,"instance":"/articles/42","code":"article_not_found"}
	// 405 {"type":"about:blank","title":"Method Not Allowed","status":405,"instance":"/articles/42"}
	// Allow: OPTIONS, GET
}

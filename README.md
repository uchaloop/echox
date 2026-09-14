<p align="center">
  <a href="https://github.com/uchaloop/echox/actions/workflows/ci.yml"><img src="https://github.com/uchaloop/echox/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/uchaloop/echox"><img src="https://pkg.go.dev/badge/github.com/uchaloop/echox.svg" alt="Go Reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/uchaloop/echox" alt="License: MIT"></a>
</p>

# echox

JSON HTTP APIs on [Echo v5](https://github.com/labstack/echo) with the building
blocks of [httpx](https://github.com/uchaloop/httpx): responses as values,
errors as [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) Problem Details,
strict JSON bodies, and validated pagination and sorting. Echo stays the router,
and everything it provides stays available.

> echox is an unofficial extension of Echo. It is not affiliated with or
> endorsed by LabStack.

- **Native first** - Echo's binder, serializer, `Recover`, `BodyLimit` and
  `RequestLogger`; go-playground/validator's tags and English translations.
  echox adds only what has no native counterpart.
- **Handlers read like the contract** - a chain binds each input and calls the
  service method, and the compiler checks one against the other.
- **One error format** - binding, validation, JSON, Echo's own 404, 405 and
  413, panics and unknown errors all answer as Problem Details, and nothing
  internal leaks.
- **One source per binder** - path, query, headers and body are bound
  separately, so a query parameter never fills a body field.
- **The service's logger** - a `*slog.Logger` of your choice, and one record
  for every 5xx response.

```bash
go get github.com/uchaloop/echox
```

echox requires Go 1.27: handler chains use generic methods.

## Quick start

```go
func main() {
	app, err := echox.Make(echox.Config{
		Logger: slog.Default(),
		ProblemRules: []problem.Rule{
			problem.WhenIs(article.ErrNotFound, problem.Template{
				Status: http.StatusNotFound,
				Code:   "article_not_found",
				Detail: "Article was not found",
			}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	articles := &ArticleHandler{service: article.MakeService(store)}
	app.GET("/articles/:id", articles.Get)
	app.POST("/articles", articles.Create)

	log.Fatal(http.ListenAndServe(":8080", app))
}

type ArticleHandler struct {
	service *article.Service
}

func (h *ArticleHandler) Get(ctx *echo.Context) error {
	return echox.MakeHandler(ctx).PositivePathID[int64]().Data(h.service.Get)
}

func (h *ArticleHandler) Create(ctx *echo.Context) error {
	return echox.MakeHandler(ctx).JSON[article.CreateParams]().Created(
		h.service.Create,
		func(created article.Article) string { return fmt.Sprintf("/articles/%d", created.ID) },
	)
}
```

The service knows nothing about HTTP:

```go
func (s *Service) Get(ctx context.Context, id int64) (Article, error)
func (s *Service) Create(ctx context.Context, params CreateParams) (Article, error)

type CreateParams struct {
	Title  string `json:"title" validate:"notblank,max=200"`
	Status Status `json:"status" validate:"required,enum"`
}
```

What a client gets:

```text
GET /articles/42     200 {"data":{"id":42,"title":"HTTP boundaries","status":"published"}}
GET /articles/7      404 {"type":"about:blank","title":"Not Found","status":404,"detail":"Article was not found","instance":"/articles/7","code":"article_not_found"}
GET /articles/0      422 ... "errors":[{"path":"id","code":"min","message":"id must be 1 or greater"}]
GET /articles/abc    400 ... "errors":[{"path":"id","code":"type","message":"id must be an integer"}]
GET /ping            204
```

```http
POST /articles
Content-Type: application/json

{"title": " ", "status": "archived"}
```

```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/problem+json

{
  "type": "about:blank",
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "Request parameters are invalid",
  "instance": "/articles",
  "code": "invalid_request",
  "errors": [
    {"path": "title", "code": "notblank", "message": "title must not be blank"},
    {"path": "status", "code": "enum", "message": "status must be one of the allowed values"}
  ]
}
```

## The application

`echox.Make` creates an Echo application, or completes one passed in
`Config.Echo`:

| Part                  | Default                                      | Config                                          |
|-----------------------|----------------------------------------------|-------------------------------------------------|
| Logger                | `slog.Default()` for a new application       | `Logger`                                        |
| Validator             | `MakeValidator()`                            | `Validator`                                     |
| Error handler         | `MakeErrorHandler` with the standard rules   | `ProblemRules`, `ProblemMapper`, `HTTPErrorHandler` |
| Log of 5xx responses  | always                                       | through the logger                              |
| Echo's `Recover`      | always                                       |                                                 |
| Echo's `BodyLimit`    | none, as in Echo                             | `BodyLimit`                                     |
| Your middleware       |                                              | `Middleware`, installed inside all of the above |
| `GET /ping`           | 204                                          |                                                 |

A 5xx response is logged once, with the status the client received, through the
request's logger:

```text
level=ERROR msg="HTTP request failed" method=GET path=/articles/42 status=500 error="query article: connection refused"
```

A middleware may give each request a logger of its own, and the record uses it:

```go
echox.Config{
	Logger: logger,
	Middleware: []echo.MiddlewareFunc{
		func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(ctx *echo.Context) error {
				ctx.SetLogger(ctx.Logger().With("request_id", ctx.Request().Header.Get("X-Request-Id")))

				return next(ctx)
			}
		},
	},
}
```

A body over `BodyLimit` is answered with 413. A panic is recovered, answered
with a 500 `internal_error` problem and logged.

## Handler chains

`MakeHandler` starts a chain. Each step binds and validates one input; a
terminal calls the service with the request context and writes the answer:

```go
func (h *ArticleHandler) Update(ctx *echo.Context) error {
	return echox.MakeHandler(ctx).
		PositivePathID[int64]().
		JSON[article.Patch]().
		Data(h.service.Update) // func(context.Context, int64, article.Patch) (article.Article, error)
}

func (h *ArticleHandler) Delete(ctx *echo.Context) error {
	return echox.MakeHandler(ctx).PositivePathID[int64]().NoContent(h.service.Delete)
}
```

| Step                   | Binds                                                          |
|------------------------|----------------------------------------------------------------|
| `PositivePathID[T]()`  | the `:id` path parameter as a positive integer                 |
| `Query[T]()`           | query parameters into `T` by `query` tags                      |
| `Headers[T]()`         | request headers into `T` by `header` tags                      |
| `JSON[T](options...)`  | one strict JSON body into `T` by `json` tags                   |
| `Bind(binder)`         | anything else, with a `func(*echo.Context) (T, error)`         |

| Terminal                     | Answers                                 |
|------------------------------|-----------------------------------------|
| `Data(execute)`              | 200 `{"data": ...}`                     |
| `Created(execute, location)` | 201 `{"data": ...}` with `Location`     |
| `NoContent(execute)`         | 204                                     |

A chain holds one or two inputs. The first failure skips the remaining steps
and the service and goes to the error handler unchanged. For anything else -
a list, another response shape, several service calls - write the handler with
the functions below.

## Binding and validation

```go
input, err := echox.DecodeAndValidateJSON[article.CreateParams](ctx)
query, err := echox.BindQueryAndValidate[SearchQuery](ctx)
headers, err := echox.BindHeadersAndValidate[ClientHeaders](ctx)
path, err := echox.BindPathAndValidate[SlugPath](ctx)
id, err := echox.BindPositivePathID[int64](ctx)
```

Each function binds one source with Echo's binder and validates the result with
the application's validator. JSON bodies are decoded by httpx: a JSON media
type, no unknown fields, exactly one document.

| Failure                                         | Status | `code`            | `errors[].code` |
|-------------------------------------------------|--------|-------------------|-----------------|
| A value that does not decode into its field     | 400    | `invalid_request` | `type`          |
| A validation tag                                | 422    | `invalid_request` | the tag name    |
| Malformed JSON, an unknown field                | 400    | `invalid_json`    |                 |
| A body that is not JSON                         | 415    |                   |                 |

```text
GET /search?q=http&tag=go&tag=api   200 {"data":{"q":"http","tags":["go","api"]}}
GET /search?q=http&q=api&tag=go     200 {"data":{"q":"http","tags":["go"]}}
GET /search?tag=go                  422 ... {"path":"q","code":"required","message":"q is a required field"}
```

A parameter sent more than once fills a slice field with every value; a scalar
field gets the first one, as Echo binds it.

Fields are named after their `json`, `query`, `param` or `header` tag, so an
error names what the client sent. Messages are go-playground/validator's English
translations. echox adds two tags: `enum` for a type with a `Valid() bool`
method, and `notblank` from go-playground's non-standard validators.

Your own tags are registered on the validator, with a translation:

```go
requestValidator, err := echox.MakeValidator()

requestValidator.GoPlayground().RegisterValidation("slug", validateSlug)
requestValidator.GoPlayground().RegisterTranslation("slug", requestValidator.Translator(),
	func(translator ut.Translator) error {
		return translator.Add("slug", "{0} must be a lowercase slug", false)
	},
	func(translator ut.Translator, field validator.FieldError) string {
		message, _ := translator.T("slug", field.Field())

		return message
	},
)

app, err := echox.Make(echox.Config{Validator: requestValidator})
// {"path":"slug","code":"slug","message":"slug must be a lowercase slug"}
```

A tag without a translation gets the message `{field} is invalid` rather than
Go type names.

## Lists

`Requests` holds the pagination policy of an API version or an endpoint:

```go
requests, err := echox.MakeRequests(echox.RequestConfig{
	Page: page.Config{DefaultSize: 20, MaxSize: 100},
})

func (h *ArticleHandler) List(ctx *echo.Context) error {
	query, err := h.requests.BindListQuery[ArticleFilters, article.SortField](ctx)
	if err != nil {
		return err
	}

	result, err := h.service.List(ctx.Request().Context(), article.ListParams{
		Status: query.Filters.Status,
		Sort: query.Sort.Make(func(field article.SortField, direction sortby.Direction) article.Sort {
			return article.Sort{Field: field, Descending: direction == sortby.Descending}
		}),
		Page: query.Page.Number(),
		Size: query.Page.Size(),
	})
	if err != nil {
		return err
	}

	return echox.Page(ctx, result.Articles, result.Total, query.Page)
}

type ArticleFilters struct {
	Status *article.Status `query:"status" validate:"omitempty,enum"`
}
```

`page`, `size` and repeated `sort` parameters are reserved:

```text
GET /articles?status=draft&sort=updatedAt:desc&sort=title&page=2
200 {"data":{"items":[...],"total":21,"page":2,"size":20}}

GET /articles?sort=author
422 ... {"path":"sort.0","code":"enum","message":"sort[0] must be one of the allowed values"}

GET /articles?page=x
400 ... {"path":"page","code":"type","message":"page must be a non-negative integer"}
```

The sort field is a domain type that parses itself, without importing echox. The
compiler rejects a type without `FromString` on its pointer:

```go
func (f *SortField) FromString(value string) error {
	parsed := SortField(value)
	if !parsed.Valid() {
		return fmt.Errorf("unsupported sort field %q", value)
	}

	*f = parsed

	return nil
}
```

`BindPageQuery` is the same without sorting.

## Responses

```go
echox.OK(ctx, article)                      // 200 {"data": article}
echox.Created(ctx, "/articles/42", article) // 201 {"data": article}, Location
echox.Data(ctx, http.StatusAccepted, job)   // any status, {"data": job}
echox.Page(ctx, items, total, query.Page)   // 200 {"data": {"items", "total", "page", "size"}}
ctx.NoContent(http.StatusNoContent)         // Echo's own
```

Bodies go through Echo's JSON serializer. `echox.Write` writes any httpx
`response.Response`, for example one with a custom header:

```go
answer := response.OK(profile)
answer.Header = http.Header{"x-user": {userID}} // sent as X-User

return echox.Write(ctx, answer)
```

## Errors

`MakeErrorHandler` answers every error as Problem Details. `Make` builds its
mapper from `ProblemRules`, followed by the standard rules: JSON, pagination
and sorting from httpx, binding and validation from echox, and errors made with
`problem.MakeError`.

Errors Echo raises itself keep their status: 404 for an unknown route, 405 with
`Allow`, 413 over the body limit. An error nothing classifies becomes a 500 that
shows nothing - including an `echo.HTTPError` with status 500 and a message:

```json
{"type":"about:blank","title":"Internal Server Error","status":500,"instance":"/articles/42","code":"internal_error"}
```

A response already written is never replaced, and a HEAD request gets the
problem's headers without its body. An application composed by hand can take
the error handler alone:

```go
mapper, err := echox.MakeProblemMapper(rules...)

app := echo.New()
app.HTTPErrorHandler = echox.MakeErrorHandler(mapper)
```

## Routes

echox registers no route besides `GET /ping`. Register yours with Echo itself;
a group per API version keeps the whole contract in one table:

```go
v1 := app.Group("/v1", authenticate)
v1.POST("/articles", articles.Create)
v1.GET("/articles", articles.List)
v1.GET("/articles/:id", articles.Get)
```

## Testing

`*echo.Echo` is an `http.Handler`, so the application is tested with
[httpx/apitest](https://pkg.go.dev/github.com/uchaloop/httpx/apitest) through a
real HTTP client and an in-memory server:

```go
func TestGetArticle(t *testing.T) {
	t.Parallel()

	api := apitest.Make(t, app)

	api.Get("/articles/42").Do().
		Status(http.StatusOK).
		JSONEqual(map[string]any{
			"data": map[string]any{"id": 42, "title": "HTTP boundaries", "status": "published"},
		})
}
```

## Documentation

Every type and function, with runnable examples:
**[pkg.go.dev/github.com/uchaloop/echox](https://pkg.go.dev/github.com/uchaloop/echox)**.

## Acknowledgements

I am grateful to the authors of [Echo](https://github.com/labstack/echo) and of
[go-playground/validator](https://github.com/go-playground/validator). echox
stands on their work and keeps to their conventions.

## License

[MIT](LICENSE)

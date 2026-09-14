/*
Package echox builds JSON HTTP APIs on Echo v5 from the building blocks of
httpx: responses as values, errors as RFC 9457 Problem Details, strict JSON
bodies, and validated pagination and sorting. It is an unofficial extension of
Echo, not affiliated with LabStack. Echo stays the router, and everything Echo
provides stays available.

# Application

[Make] creates an Echo application, or completes one passed in Config.Echo:

	app, err := echox.Make(echox.Config{
		Logger: logger,
		ProblemRules: []problem.Rule{
			problem.WhenIs(article.ErrNotFound, problem.Template{
				Status: http.StatusNotFound,
				Code:   "article_not_found",
			}),
		},
		BodyLimit: 1 << 20,
	})

It installs:

  - the service's *slog.Logger as the Echo logger, or slog.Default for a new
    application, so that nothing is written by a logger of Echo's own;
  - a [Validator] with the English translations of go-playground/validator;
  - the error handler of [MakeErrorHandler], with the application's rules
    followed by the standard ones;
  - a log record for every response with a 5xx status, written through the
    request's logger, which a middleware may replace with Context.SetLogger;
  - Echo's Recover middleware, so a panic becomes a logged 500 problem;
  - Echo's BodyLimit middleware when Config.BodyLimit is set; as in Echo,
    there is no limit by default;
  - Config.Middleware, inside all of the above;
  - GET /ping, answered by [Ping] with 204.

Routes stay the application's. Each part can be replaced through [Config], or
used on its own in an application composed by hand.

# Handlers

[MakeHandler] starts a typed chain. Each step binds and validates one input,
and a terminal calls the service and writes the answer:

	func (h *Handler) Get(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).PositivePathID[int64]().Data(h.service.Get)
	}

	func (h *Handler) Update(ctx *echo.Context) error {
		return echox.MakeHandler(ctx).
			PositivePathID[int64]().
			JSON[article.Patch]().
			Data(h.service.Update)
	}

The steps are [Handler.PositivePathID], [Handler.Query], [Handler.Headers],
[Handler.JSON], and [Handler.Bind] for a [Binder] of the application. A chain
holds one or two inputs, and the compiler checks them against the signature of
the service method. The terminals are [BoundHandler.Data] with 200,
[BoundHandler.Created] with 201 and a Location, and [BoundHandler.NoContent]
with 204. The first failure skips the remaining steps and the service, and the
terminal returns it to the error handler unchanged. The service receives the
request context and values of its own types, never Echo types.

For anything else, such as a list endpoint, another response shape or several
service calls, a handler uses the bind and response functions directly.

# Binding

Each function binds one source with Echo's binder and validates the result:
[BindPathAndValidate] reads param tags, [BindQueryAndValidate] query tags,
[BindHeadersAndValidate] header tags, and [DecodeAndValidateJSON] a strict JSON
body. No function mixes sources, so a query parameter never fills a body field.
[BindPositivePathID] binds the :id path parameter as a positive integer.

A value that does not decode into its field is a [*BindError] with code type,
answered with status 400, such as "id must be an integer". A parameter sent
more than once fills a slice field with every value, and a scalar field gets
the first one, as Echo binds it.

# Validation

[MakeValidator] creates the go-playground validator that Make installs. Fields
are named after their json, query, param or header tag, so a failure names the
value the client sent. A failure is a [*ValidationError], answered with status
422: the code is the validation tag and the message is its English
translation.

	Title string `json:"title" validate:"required,max=200"`
	// {"path":"title","code":"required","message":"title is a required field"}

Two tags are added: enum, for a value whose type implements [EnumValid], and
notblank from go-playground's non-standard validators. The application registers
its own tags and their translations through [Validator.GoPlayground] and
[Validator.Translator]. A tag without a translation gets the message
"{field} is invalid".

# Pagination and sorting

[Requests] holds the pagination policy of an API version or an endpoint.
[Requests.BindPageQuery] binds the filters of an endpoint with the page and size
query parameters, and [Requests.BindListQuery] also parses the repeated sort
parameter into a typed order:

	// GET /articles?status=draft&sort=updatedAt:desc&page=2&size=20
	query, err := h.requests.BindListQuery[articleFilters, article.SortField](ctx)

The sort field type implements [EnumStringable] on its pointer, so the compiler
rejects a type that cannot parse itself. The page, size and sort parameters are
reserved and must not be declared as filters.

# Responses

[OK], [Data], [Created] and [Page] write the envelopes of httpx/response, and
[Write] writes any response.Response. The body goes through Echo's
JSONSerializer, and headers are written under canonical names.

# Errors

[MakeErrorHandler] answers every error as Problem Details. [MakeProblemMapper]
puts the application's rules first, then the input rule of httpx/problem,
[BindProblemRule] for binding and validation, and the rule for errors made
with problem.MakeError.

An error Echo raises itself keeps its status: 404 for an unknown route, 405 with
Allow, 413 over the body limit. An error that nothing classifies becomes a 500
that shows nothing, including an echo.HTTPError with status 500 and a message.
A response already written is never replaced, and a HEAD request gets the
headers of the problem without its body.

# Routes

echox registers no route besides GET /ping. The application registers its
routes with Echo itself, for example with one Group per API version, so the
whole contract reads as one table.
*/
package echox

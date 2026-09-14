package echox_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/problem"
	"github.com/uchaloop/httpx/request"
)

type conventionalInput struct {
	Title string `json:"title" validate:"required"`
}

func TestMakeConfiguresConventionalApplication(t *testing.T) {
	t.Parallel()

	app, err := echox.Make(echox.Config{})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}

	app.POST("/articles", func(ctx *echo.Context) error {
		_, err := echox.DecodeAndValidateJSON[conventionalInput](ctx)

		return err
	})

	api := apitest.Make(t, app)
	api.Get("/ping").Do().Status(http.StatusNoContent).BodyEqual("")
	api.Post("/articles").JSON(map[string]any{}).Do().
		Status(http.StatusUnprocessableEntity).
		Header("Content-Type", problem.ContentType).
		JSONEqual(invalidInput("/articles", invalidParam("title", "required", "title is a required field")))
}

func TestMakeChoosesLogger(t *testing.T) {
	t.Parallel()

	created, err := echox.Make(echox.Config{})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}

	if created.Logger != slog.Default() {
		t.Error("a new application does not use slog.Default")
	}

	own := slog.New(slog.DiscardHandler)
	supplied, err := echox.Make(echox.Config{Echo: echo.NewWithConfig(echo.Config{Logger: own})})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}

	if supplied.Logger != own {
		t.Error("Make replaced the logger of a supplied application")
	}

	service := slog.New(slog.DiscardHandler)
	replaced, err := echox.Make(echox.Config{Echo: echo.NewWithConfig(echo.Config{Logger: own}), Logger: service})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}

	if replaced.Logger != service {
		t.Error("Make ignored Config.Logger")
	}
}

func TestMakeLogsOnlyServerErrors(t *testing.T) {
	t.Parallel()

	errInvalid := errors.New("invalid article")
	var output syncBuffer
	app, err := echox.Make(echox.Config{
		Logger: slog.New(slog.NewJSONHandler(&output, nil)),
		ProblemRules: []problem.Rule{problem.WhenIs(errInvalid, problem.Template{
			Status: http.StatusBadRequest,
			Code:   "invalid_article",
		})},
	})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}

	app.GET("/invalid", func(*echo.Context) error { return errInvalid })
	app.GET("/internal", func(*echo.Context) error { return errors.New("database unavailable") })
	app.GET("/panic", func(*echo.Context) error { panic("failure") })

	api := apitest.Make(t, app)
	api.Get("/invalid").Do().Status(http.StatusBadRequest)
	api.Get("/internal").Do().Status(http.StatusInternalServerError).JSONEqual(internalProblem("/internal"))
	api.Get("/panic").Do().Status(http.StatusInternalServerError).JSONEqual(internalProblem("/panic"))

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want only the two server errors:\n%s", len(lines), output.String())
	}

	assertServerErrorLog(t, lines[0], "/internal", "database unavailable")
	assertServerErrorLog(t, lines[1], "/panic", "failure")
}

func TestMakeLimitsRequestBody(t *testing.T) {
	t.Parallel()

	makeApp := func(limit int64) *echo.Echo {
		app, err := echox.Make(echox.Config{BodyLimit: limit})
		if err != nil {
			t.Fatalf("Make: %v", err)
		}

		app.POST("/articles", func(ctx *echo.Context) error {
			if _, err := echox.DecodeJSON[map[string]any](ctx); err != nil {
				return err
			}

			return ctx.NoContent(http.StatusNoContent)
		})

		return app
	}

	tooLarge := map[string]any{
		"type": "about:blank", "title": "Request Entity Too Large", "status": 413, "instance": "/articles",
	}

	api := apitest.Make(t, makeApp(16))
	api.Post("/articles").JSON(map[string]string{"a": "b"}).Do().Status(http.StatusNoContent)
	api.Post("/articles").JSON(map[string]string{"title": "a long title"}).Do().
		Status(http.StatusRequestEntityTooLarge).
		JSONEqual(tooLarge)

	// Without Content-Length the limit is enforced while the body is read.
	api.Post("/articles").Body("application/json", io.MultiReader(strings.NewReader(`{"title":"a long title"}`))).Do().
		Status(http.StatusRequestEntityTooLarge).
		JSONEqual(tooLarge)

	// Zero sets no limit, as in Echo itself.
	apitest.Make(t, makeApp(0)).Post("/articles").JSON(map[string]string{"title": strings.Repeat("x", 1<<20)}).Do().
		Status(http.StatusNoContent)

	if _, err := echox.Make(echox.Config{BodyLimit: -1}); err == nil {
		t.Error("Make accepted a negative body limit")
	}
}

func TestMakeRejectsMapperWithRulesBeforeChangingEcho(t *testing.T) {
	t.Parallel()

	mapper, err := echox.MakeProblemMapper()
	if err != nil {
		t.Fatal(err)
	}

	app := echo.New()
	if _, err := echox.Make(echox.Config{
		Echo: app, ProblemMapper: mapper,
		ProblemRules: []problem.Rule{problem.ErrorRule()},
	}); err == nil {
		t.Fatal("accepted ambiguous configuration")
	}

	if app.Validator != nil {
		t.Fatal("modified Echo before rejecting configuration")
	}
}

func TestMakeProblemRulesPrecedeStandardRules(t *testing.T) {
	t.Parallel()

	// A service rule for an error the standard input rule also knows must win.
	app, err := echox.Make(echox.Config{ProblemRules: []problem.Rule{
		problem.WhenAs(func(*request.DecodeError) problem.Template {
			return problem.Template{Status: http.StatusBadRequest, Code: "body_rejected"}
		}),
	}})
	if err != nil {
		t.Fatal(err)
	}

	app.POST("/", func(ctx *echo.Context) error {
		_, err := echox.DecodeJSON[map[string]any](ctx)

		return err
	})

	apitest.Make(t, app).Post("/").Body("application/json", strings.NewReader("{")).Do().
		Status(http.StatusBadRequest).
		JSONEqual(map[string]any{
			"type": "about:blank", "title": "Bad Request", "status": 400, "code": "body_rejected", "instance": "/",
		})
}

func TestMakeAcceptsCustomEchoAndErrorHandler(t *testing.T) {
	t.Parallel()

	app := echo.New()
	configured, err := echox.Make(echox.Config{
		Echo: app,
		HTTPErrorHandler: func(ctx *echo.Context, err error) {
			_ = ctx.JSON(http.StatusTeapot, map[string]string{"error": err.Error()})
		},
	})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}

	configured.GET("/failure", func(*echo.Context) error {
		return errors.New("failure")
	})

	if configured != app {
		t.Fatal("Make returned another Echo application")
	}

	apitest.Make(t, configured).Get("/failure").Do().
		Status(http.StatusTeapot).
		JSONEqual(map[string]string{"error": "failure"})
}

// syncBuffer collects log output written by server goroutines.
type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}

func assertServerErrorLog(t *testing.T, line, path, cause string) {
	t.Helper()

	var record struct {
		Level  string `json:"level"`
		Msg    string `json:"msg"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode log line %q: %v", line, err)
	}

	if record.Level != "ERROR" || record.Msg != "HTTP request failed" || record.Method != http.MethodGet ||
		record.Path != path || record.Status != http.StatusInternalServerError || !strings.Contains(record.Error, cause) {
		t.Errorf("log record = %+v, want an ERROR for GET %s with status 500 and cause %q", record, path, cause)
	}
}

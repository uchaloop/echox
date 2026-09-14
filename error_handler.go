package echox

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/httpx/problem"
)

// MakeProblemMapper creates a mapper with application rules followed by the
// standard httpx input, Echo binding, and explicit transport-error rules.
func MakeProblemMapper(rules ...problem.Rule) (*problem.Mapper, error) {
	standardRules := []problem.Rule{
		problem.InputProblemRule(),
		BindProblemRule(),
		problem.ErrorRule(),
	}

	configuredRules := make([]problem.Rule, 0, len(rules)+len(standardRules))
	configuredRules = append(configuredRules, rules...)
	configuredRules = append(configuredRules, standardRules...)

	return problem.MakeMapper(configuredRules...)
}

// MakeErrorHandler adapts a problem.Mapper to Echo's centralized error
// handler. Native HTTP status errors keep their status; application errors are
// classified by m. Unknown errors remain safe internal problems.
func MakeErrorHandler(m *problem.Mapper) echo.HTTPErrorHandler {
	return func(ctx *echo.Context, err error) {
		if ctx == nil || responseCommitted(ctx) {
			return
		}

		mapped := mapError(ctx.Request(), m, err)
		if ctx.Request().Method == http.MethodHead {
			// HEAD carries the headers a GET would get, such as Accept on 415,
			// but never the body.
			mapped.Response().SetHeaders(ctx.Response().Header())
			if writeErr := ctx.NoContent(mapped.Status); writeErr != nil {
				ctx.Logger().Error("write problem response", "error", writeErr)
			}

			return
		}

		if writeErr := Write(ctx, mapped.Response()); writeErr != nil {
			ctx.Logger().Error("write problem response", "error", writeErr)
		}
	}
}

func responseCommitted(ctx *echo.Context) bool {
	response, _ := echo.UnwrapResponse(ctx.Response())

	return response != nil && response.Committed
}

func mapError(r *http.Request, m *problem.Mapper, err error) problem.Problem {
	if mapped, matched := m.MapKnown(r, err); matched {
		return mapped
	}

	if status := echo.StatusCode(err); status >= http.StatusBadRequest &&
		status <= 599 &&
		status != http.StatusInternalServerError &&
		len(http.StatusText(status)) > 0 {
		return problem.MakeProblem(r, problem.Template{Status: status})
	}

	return m.Map(r, err)
}

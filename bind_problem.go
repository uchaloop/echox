package echox

import (
	"errors"
	"net/http"

	"github.com/uchaloop/httpx/problem"
)

// BindProblemRule maps Echo binding and tag validation errors to RFC 9457
// templates. Application rules may be placed before it to override the default
// public text.
func BindProblemRule() problem.Rule {
	return problem.When(func(err error) (problem.Template, bool) {
		if validationErr, ok := errors.AsType[*ValidationError](err); ok {
			return problem.InvalidRequest(
				http.StatusUnprocessableEntity,
				makeInvalidParams(validationErr.fields)...,
			), true
		}

		if bindErr, ok := errors.AsType[*BindError](err); ok {
			var invalidParams []problem.InvalidParam
			if len(bindErr.Path) > 0 {
				invalidParams = []problem.InvalidParam{{
					Path:    bindErr.Path,
					Code:    bindErr.Code,
					Message: bindErr.Message,
				}}
			}

			return problem.InvalidRequest(http.StatusBadRequest, invalidParams...), true
		}

		return problem.Template{}, false
	})
}

func makeInvalidParams(fields []InvalidField) []problem.InvalidParam {
	params := make([]problem.InvalidParam, len(fields))
	for index, field := range fields {
		params[index] = problem.InvalidParam{
			Path:    field.Path,
			Code:    field.Code,
			Message: field.Message,
		}
	}

	return params
}

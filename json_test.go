package echox_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/echox"
	"github.com/uchaloop/httpx/apitest"
	"github.com/uchaloop/httpx/request"
)

type articleInput struct {
	Title string `json:"title"`
}

type validatorFake struct {
	err error
}

func (v *validatorFake) Validate(any) error {
	return v.err
}

// DecodeAndValidateJSON validates through the validator configured on Echo
// and keeps its error in the chain.
func TestDecodeAndValidateJSONUsesEchoValidator(t *testing.T) {
	t.Parallel()

	valid := echo.New()
	valid.Validator = &validatorFake{}
	valid.POST("/articles", func(ctx *echo.Context) error {
		input, err := echox.DecodeAndValidateJSON[articleInput](ctx)
		if err != nil {
			return err
		}

		return ctx.JSON(http.StatusCreated, input)
	})

	apitest.Make(t, valid).Post("/articles").JSON(articleInput{Title: "Architecture"}).Do().
		Status(http.StatusCreated).
		JSONEqual(articleInput{Title: "Architecture"})

	validationErr := errors.New("title is required")
	returned := make(chan error, 1)
	invalid := echo.New()
	invalid.Validator = &validatorFake{err: validationErr}
	invalid.POST("/articles", func(ctx *echo.Context) error {
		_, err := echox.DecodeAndValidateJSON[articleInput](ctx)
		returned <- err

		return err
	})

	apitest.Make(t, invalid).Post("/articles").JSON(articleInput{}).Do().Status(http.StatusInternalServerError)
	if err := <-returned; !errors.Is(err, validationErr) {
		t.Fatalf("error = %v, want validation error in chain", err)
	}
}

func TestDecodeJSONForwardsOptions(t *testing.T) {
	t.Parallel()

	app := echo.New()
	app.POST("/articles", func(ctx *echo.Context) error {
		input, err := echox.DecodeJSON[articleInput](ctx, request.AllowUnknownFields())
		if err != nil {
			return err
		}

		return ctx.JSON(http.StatusOK, input)
	})

	apitest.Make(t, app).Post("/articles").
		Body("application/json", strings.NewReader(`{"title":"Architecture","unknown":true}`)).
		Do().
		Status(http.StatusOK).
		JSONEqual(articleInput{Title: "Architecture"})
}

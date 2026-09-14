package echox

import (
	"fmt"

	"github.com/labstack/echo/v5"
	"github.com/uchaloop/httpx/page"
	"github.com/uchaloop/httpx/sortby"
)

// EnumStringable describes an enum that can parse and assign a string value.
type EnumStringable interface {
	FromString(string) error
}

// RequestConfig defines pagination policy for a version or endpoint.
type RequestConfig struct {
	Page page.Config
}

// Requests holds immutable policy. It can be shared across concurrent requests.
type Requests struct {
	pageConfig page.Config
}

// MakeRequests validates policy before serving requests.
func MakeRequests(cfg RequestConfig) (*Requests, error) {
	if _, err := page.Make(page.Params{}, cfg.Page); err != nil {
		return nil, err
	}

	return &Requests{pageConfig: cfg.Page}, nil
}

// PageInput separates application filters from validated HTTP pagination.
type PageInput[Filters any] struct {
	Filters Filters
	Page    page.Page
}

// ListInput adds ordered, typed sort terms to a paginated input.
type ListInput[Filters, Field any] struct {
	PageInput[Filters]
	Sort sortby.Order[Field]
}

// controlQuery holds the query parameters reserved for pagination and sort,
// bound in one pass. The fields are flat: Echo does not bind an embedded
// struct of an unexported type. BindPageQuery ignores sort, as it ignores any
// parameter it does not declare.
type controlQuery struct {
	Page *uint64  `query:"page" validate:"omitempty,min=1"`
	Size *uint64  `query:"size" validate:"omitempty,min=1"`
	Sort []string `query:"sort"`
}

// BindPageQuery binds filter tags and pagination from query parameters only.
// Filters must be a struct using query and validate tags. Page and size are
// reserved for pagination and must not be declared as application filters.
func (r *Requests) BindPageQuery[Filters any](ctx *echo.Context) (PageInput[Filters], error) {
	if r == nil {
		return PageInput[Filters]{}, fmt.Errorf("requests is nil")
	}

	filters, err := BindQueryAndValidate[Filters](ctx)
	if err != nil {
		return PageInput[Filters]{}, err
	}

	query, err := BindQueryAndValidate[controlQuery](ctx)
	if err != nil {
		return PageInput[Filters]{}, err
	}

	return makePageInput(r.pageConfig, filters, query)
}

// BindListQuery also parses repeated sort terms. Sort is reserved for ordering.
func (r *Requests) BindListQuery[Filters, Field any, FieldPtr interface {
	*Field
	EnumStringable
}](ctx *echo.Context) (ListInput[Filters, Field], error) {
	if r == nil {
		return ListInput[Filters, Field]{}, fmt.Errorf("requests is nil")
	}

	filters, err := BindQueryAndValidate[Filters](ctx)
	if err != nil {
		return ListInput[Filters, Field]{}, err
	}

	query, err := BindQueryAndValidate[controlQuery](ctx)
	if err != nil {
		return ListInput[Filters, Field]{}, err
	}

	input, err := makePageInput(r.pageConfig, filters, query)
	if err != nil {
		return ListInput[Filters, Field]{}, err
	}

	order, err := sortby.Parse(query.Sort, func(value string) (Field, error) {
		var field Field
		if err := FieldPtr(&field).FromString(value); err != nil {
			return field, err
		}

		return field, nil
	})
	if err != nil {
		return ListInput[Filters, Field]{}, err
	}

	return ListInput[Filters, Field]{PageInput: input, Sort: order}, nil
}

func makePageInput[Filters any](cfg page.Config, filters Filters, query controlQuery) (PageInput[Filters], error) {
	current, err := page.Make(page.Params{Number: query.Page, Size: query.Size}, cfg)
	if err != nil {
		return PageInput[Filters]{}, err
	}

	return PageInput[Filters]{Filters: filters, Page: current}, nil
}

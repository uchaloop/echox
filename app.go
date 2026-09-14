package echox

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/uchaloop/httpx/problem"
)

// Config defines the conventional Echo composition applied by Make.
type Config struct {
	// Echo is an optional application instance. Make creates one when it is nil.
	Echo *echo.Echo
	// Logger replaces the application logger. When nil, Make keeps the logger
	// of a supplied Echo instance and gives a new one slog.Default. A service
	// middleware may still set a request-scoped logger with Context.SetLogger.
	Logger *slog.Logger
	// ProblemMapper classifies application errors. When nil, Make constructs
	// a mapper from ProblemRules followed by the standard rules.
	ProblemMapper *problem.Mapper
	// ProblemRules precede the standard rules. Cannot be combined with ProblemMapper.
	ProblemRules []problem.Rule
	// Validator replaces the standard echox validator when it is not nil.
	Validator echo.Validator
	// HTTPErrorHandler replaces the RFC 9457 handler when it is not nil.
	HTTPErrorHandler echo.HTTPErrorHandler
	// BodyLimit is the largest accepted request body in bytes; a larger body
	// is answered with 413. Zero sets no limit, as Echo sets none by default.
	BodyLimit int64
	// Middleware is installed inside server error logging, panic recovery and
	// the body limit.
	Middleware []echo.MiddlewareFunc
}

// Make creates or completes a conventional Echo application. It configures
// the logger, validation, centralized error handling, server error logging,
// panic recovery, and GET /ping. The service remains responsible for all
// application routes.
func Make(cfg Config) (*echo.Echo, error) {
	if cfg.ProblemMapper != nil && len(cfg.ProblemRules) > 0 {
		return nil, fmt.Errorf("problem mapper and problem rules cannot be combined")
	}

	if cfg.BodyLimit < 0 {
		return nil, fmt.Errorf("body limit must not be negative, got %d", cfg.BodyLimit)
	}

	application := cfg.Echo
	if application == nil {
		application = echo.New()
		application.Logger = slog.Default()
	}

	if cfg.Logger != nil {
		application.Logger = cfg.Logger
	}

	mapper := cfg.ProblemMapper
	if mapper == nil {
		var err error
		mapper, err = MakeProblemMapper(cfg.ProblemRules...)
		if err != nil {
			return nil, err
		}
	}

	validator := cfg.Validator
	if validator == nil {
		var err error
		validator, err = MakeValidator()
		if err != nil {
			return nil, err
		}
	}

	application.Validator = validator

	if cfg.HTTPErrorHandler != nil {
		application.HTTPErrorHandler = cfg.HTTPErrorHandler
	} else {
		application.HTTPErrorHandler = MakeErrorHandler(mapper)
	}

	// Error logging wraps recovery, so a recovered panic is logged as well.
	application.Use(
		logServerErrors(),
		middleware.RecoverWithConfig(middleware.RecoverConfig{DisableStackAll: true}),
	)

	if cfg.BodyLimit > 0 {
		application.Use(middleware.BodyLimit(cfg.BodyLimit))
	}

	application.Use(cfg.Middleware...)
	application.GET("/ping", Ping)

	return application, nil
}

// logServerErrors hands an error to the error handler first, so it logs the
// status the client received, and logs only 5xx responses. It writes through
// the request logger, which a service middleware may replace.
func logServerErrors() echo.MiddlewareFunc {
	logRequests := requestErrorLogger()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		handle := logRequests(next)

		return func(ctx *echo.Context) error {
			// RequestLogger returns the error it has already handed to the error
			// handler; Echo would then call the handler a second time, and a
			// handler that does not check Committed would write twice.
			if err := handle(ctx); err != nil && !responseCommitted(ctx) {
				return err
			}

			return nil
		}
	}
}

func requestErrorLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError: true,
		LogStatus:   true,
		LogMethod:   true,
		LogURIPath:  true,
		LogValuesFunc: func(ctx *echo.Context, values middleware.RequestLoggerValues) error {
			if values.Error != nil && values.Status >= http.StatusInternalServerError {
				ctx.Logger().ErrorContext(
					ctx.Request().Context(),
					"HTTP request failed",
					"method", values.Method,
					"path", values.URIPath,
					"status", values.Status,
					"error", values.Error,
				)
			}

			return nil
		},
	})
}

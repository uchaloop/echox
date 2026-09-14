package echox

import "github.com/labstack/echo/v5"

type positiveInteger interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// The router guarantees that id is present, so zero is a value below the
// minimum rather than a missing value.
type positivePathID[ID positiveInteger] struct {
	ID ID `param:"id" validate:"min=1"`
}

// BindPositivePathID binds the conventional id path parameter and validates
// that it is a positive integer.
func BindPositivePathID[ID positiveInteger](ctx *echo.Context) (ID, error) {
	path, err := BindPathAndValidate[positivePathID[ID]](ctx)

	return path.ID, err
}

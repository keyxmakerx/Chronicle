package notes

import "github.com/labstack/echo/v5"

// pathValuesOf pairs route parameter names with their values, in order, for
// tests that build an Echo context by hand.
func pathValuesOf(names, values []string) echo.PathValues {
	pv := make(echo.PathValues, len(names))
	for i, n := range names {
		pv[i] = echo.PathValue{Name: n, Value: values[i]}
	}
	return pv
}

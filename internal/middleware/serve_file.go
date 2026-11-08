package middleware

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v5"
)

// ServeFile sends the file at an operating-system path. Echo's c.File opens
// names inside its own filesystem, which is rooted at the working directory,
// so a media or backup directory elsewhere would answer 404. Callers resolve
// and check the path first; this only opens it.
func ServeFile(c *echo.Context, path string) error {
	return c.FileFS(filepath.Base(path), os.DirFS(filepath.Dir(path)))
}

// ServeAttachment sends the file at path as a download named name. The name
// is Go-quoted, so a quote or newline in it cannot end the header value.
func ServeAttachment(c *echo.Context, path, name string) error {
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", name))
	return ServeFile(c, path)
}

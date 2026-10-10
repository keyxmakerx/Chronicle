package media

import (
	"strings"
	"testing"
)

// The handlers are written into a double-quoted onclick/onchange attribute
// without escaping, so one stray double quote ends the attribute and dumps
// the rest of the script onto the page as text.
func TestPageFileHandlersHaveNoDoubleQuotes(t *testing.T) {
	tests := map[string]string{
		"pick": pfPickJS, "upload": pfUploadJS, "ask": pfAskJS,
		"keep": pfKeepJS, "remove": pfRemoveJS, "gmToggle": pfGMToggleJS,
	}
	for name, js := range tests {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(js, `"`) {
				t.Errorf("%s handler contains a double quote", name)
			}
		})
	}
}

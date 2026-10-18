package systems

import "fmt"

// propString extracts a string representation of a property value
// from a ReferenceItem's Properties map. Returns empty string if
// the key is not present.
func propString(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	val, ok := props[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

// propText is propString for the reference browser, which shows plain text:
// {@category term} markup becomes the term it names. The Rules book keeps
// the markup because it turns it into term links.
func propText(props map[string]any, key string) string {
	return flattenRefMarkup(propString(props, key))
}

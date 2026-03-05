package modules

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

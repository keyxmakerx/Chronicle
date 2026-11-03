package transformations

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Limits keep a layer small: a transformation names a handful of fields, and
// every page view sends its layers.
const (
	MaxBodyBytes     = 64 << 10
	MaxLayersPerPage = 20
	MaxChanges       = 40
	MaxValueBytes    = 4 << 10
	MaxNameChars     = 80
	MaxSourceChars   = 120
	MaxNoteChars     = 1000
	MaxLabelChars    = 120
	MaxFieldChars    = 64
	MaxIDChars       = 64
)

// validToken is an id from a URL: letters, digits, '-' and '_' only.
func validToken(s string) bool {
	if s == "" || len(s) > MaxIDChars {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// validField is a sheet field key: lower-case letters, digits and '_',
// starting with a letter, which is how every game system names them.
func validField(s string) bool {
	if s == "" || len(s) > MaxFieldChars || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// validSource is a preset name such as "drawsteel:revenant".
func validSource(s string) bool {
	if len(s) > MaxSourceChars {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == ':' || r == '.') {
			return false
		}
	}
	return true
}

// plainText trims s and refuses control characters other than newlines in
// a note, so stored text is always safe to show as text.
func plainText(s string, max int, multiline bool) (string, bool) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return "", false
	}
	for _, r := range s {
		if r < 0x20 && !(multiline && (r == '\n' || r == '\t')) || r == 0x7f {
			return "", false
		}
	}
	return s, true
}

// valueKind reports what JSON value v is: "string", "number", "object", or
// "" for anything else (null, a list, a boolean, nothing).
func valueKind(v json.RawMessage) string {
	t := bytes.TrimSpace(v)
	if len(t) == 0 {
		return ""
	}
	switch t[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return "number"
	}
	return ""
}

// normalise checks a layer and returns it trimmed, with each value
// re-encoded compactly so nothing but valid JSON is stored.
func normalise(in AddInput) (AddInput, error) {
	name, ok := plainText(in.Name, MaxNameChars, false)
	if !ok || name == "" {
		return AddInput{}, errInvalid("a layer needs a name of up to 80 characters")
	}
	if !validSource(in.Source) {
		return AddInput{}, errInvalid("the preset name is not valid")
	}
	note, ok := plainText(in.Note, MaxNoteChars, true)
	if !ok {
		return AddInput{}, errInvalid("the note is too long")
	}
	if len(in.Changes) == 0 || len(in.Changes) > MaxChanges {
		return AddInput{}, errInvalid("a layer changes between 1 and 40 fields")
	}
	out := AddInput{Name: name, Source: in.Source, Note: note, Changes: make([]Change, 0, len(in.Changes))}
	for _, c := range in.Changes {
		if !validField(c.Field) {
			return AddInput{}, errInvalid("a change names a field that is not valid")
		}
		label, ok := plainText(c.Label, MaxLabelChars, false)
		if !ok {
			return AddInput{}, errInvalid("a change's label is too long")
		}
		if len(c.Value) > MaxValueBytes {
			return AddInput{}, errInvalid("a change's value is too large")
		}
		kind := valueKind(c.Value)
		switch c.Op {
		case OpSet, OpAppend:
			if kind == "" {
				return AddInput{}, errInvalid("set and append take text, a number or an object")
			}
		case OpAdd:
			if kind != "number" {
				return AddInput{}, errInvalid("add takes a number")
			}
		case OpRemove:
			if kind != "string" {
				return AddInput{}, errInvalid("remove takes the name of the entry")
			}
		default:
			return AddInput{}, errInvalid("a change's op is set, add, append or remove")
		}
		var v any
		if err := json.Unmarshal(c.Value, &v); err != nil {
			return AddInput{}, errInvalid("a change's value is not valid JSON")
		}
		compact, err := json.Marshal(v)
		if err != nil {
			return AddInput{}, errInvalid("a change's value is not valid JSON")
		}
		out.Changes = append(out.Changes, Change{Field: c.Field, Op: c.Op, Value: compact, Label: label})
	}
	return out, nil
}

package importer

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// CheckFields turns a page's `fields:` map into the values its category's
// sheet stores, checked against that category's own field list. The entity
// service saves any map it is given, so this is the only gate between an
// AI's guess and the page's sheet: an unknown key or a wrong type is a
// plain-words problem, never saved junk.
//
// A null value clears that field. Lists and maps go into text fields as
// JSON text, the shape a system's sheet keeps structured data in (a stat
// block's abilities, say).
func CheckFields(defs []entities.FieldDefinition, in map[string]any) (map[string]any, []string) {
	if len(in) == 0 {
		return nil, nil
	}
	byKey := make(map[string]entities.FieldDefinition, len(defs))
	for _, d := range defs {
		byKey[strings.ToLower(d.Key)] = d
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(in))
	var problems []string
	for _, k := range keys {
		def, ok := byKey[strings.ToLower(strings.TrimSpace(k))]
		if !ok {
			problems = append(problems, fmt.Sprintf("%q is not one of this category's fields.", k))
			continue
		}
		v, problem := fieldValue(def, in[k])
		if problem != "" {
			problems = append(problems, fmt.Sprintf("%s: %s", def.Label, problem))
			continue
		}
		out[def.Key] = v
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return out, nil
}

// fieldValue checks one value against its field's type, returning what to
// store. The types are the sheet form's input types.
func fieldValue(def entities.FieldDefinition, v any) (any, string) {
	if v == nil {
		return nil, ""
	}
	switch def.Type {
	case "number":
		switch n := v.(type) {
		case int:
			return n, ""
		case int64:
			return n, ""
		case float64:
			if math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, "needs a number."
			}
			return n, ""
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				if f == math.Trunc(f) && math.Abs(f) < 1e15 {
					return int(f), ""
				}
				return f, ""
			}
		}
		return nil, fmt.Sprintf("needs a number, not %s.", shown(v))
	case "checkbox":
		switch b := v.(type) {
		case bool:
			return b, ""
		case string:
			switch strings.ToLower(strings.TrimSpace(b)) {
			case "yes", "true":
				return true, ""
			case "no", "false":
				return false, ""
			}
		}
		return nil, fmt.Sprintf("needs yes or no, not %s.", shown(v))
	case "select":
		s, ok := scalarText(v)
		if !ok {
			return nil, "needs one of: " + strings.Join(def.Options, ", ") + "."
		}
		if len(def.Options) == 0 {
			return s, ""
		}
		for _, o := range def.Options {
			if strings.EqualFold(o, s) {
				return o, ""
			}
		}
		return nil, fmt.Sprintf("%q is not one of: %s.", s, strings.Join(def.Options, ", "))
	case "url":
		s, ok := scalarText(v)
		if !ok {
			return nil, "needs a web address."
		}
		u, err := url.Parse(s)
		if s != "" && (err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "") {
			return nil, fmt.Sprintf("%q is not an http or https address.", s)
		}
		return s, ""
	default: // text, textarea, and anything older
		if s, ok := scalarText(v); ok {
			return s, ""
		}
		b, err := json.Marshal(jsonSafe(v))
		if err != nil {
			return nil, "could not be read as text or a list."
		}
		return string(b), ""
	}
}

// scalarText is a plain value as text; lists and maps are not scalars.
func scalarText(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t), true
	case bool:
		if t {
			return "yes", true
		}
		return "no", true
	case int, int64, float64:
		return fmt.Sprint(t), true
	}
	return "", false
}

// jsonSafe converts YAML's map[any]any (older decoders) into string-keyed
// maps so the value marshals; yaml.v3 already yields map[string]any.
func jsonSafe(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[fmt.Sprint(k)] = jsonSafe(x)
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = jsonSafe(x)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = jsonSafe(x)
		}
		return out
	}
	return v
}

// shown is a value as a problem message quotes it.
func shown(v any) string {
	if s, ok := scalarText(v); ok {
		return strconv.Quote(s)
	}
	return "a list"
}

// FieldsSummary is the review row's line for a page's checked fields:
// each field's label and value, short, in the category's own order.
func FieldsSummary(defs []entities.FieldDefinition, checked map[string]any) string {
	var parts []string
	for _, d := range defs {
		v, ok := checked[d.Key]
		if !ok {
			continue
		}
		switch {
		case v == nil:
			parts = append(parts, d.Label+" cleared")
		default:
			s := fmt.Sprint(v)
			if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") {
				var list []any
				if json.Unmarshal([]byte(s), &list) == nil {
					s = fmt.Sprintf("%d %s", len(list), pluralWord(len(list), "entry", "entries"))
				} else {
					s = "set"
				}
			} else if r := []rune(s); len(r) > 40 {
				s = string(r[:37]) + "..."
			}
			parts = append(parts, d.Label+" "+s)
		}
	}
	return strings.Join(parts, ", ")
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

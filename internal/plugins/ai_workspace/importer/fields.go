package importer

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

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
	case entities.FieldTypeLink:
		// A page's name; the committer turns it into the page's id once every
		// page in the paste is saved (LinkFields).
		s, ok := scalarText(v)
		if !ok || s == "" {
			return nil, "needs the name of a page."
		}
		return s, ""
	case entities.FieldTypeCalendarDate:
		// year-month-day with the month by number; the server checks the day
		// is on the campaign's calendar when the page is saved.
		var s string
		switch t := v.(type) {
		case time.Time:
			s = fmt.Sprintf("%d-%d-%d", t.Year(), int(t.Month()), t.Day())
		default:
			s, _ = scalarText(v)
		}
		d, ok := entities.ParseFieldDate(s)
		if !ok {
			return nil, fmt.Sprintf("needs a date as year-month-day with the month's number, like 1043-3-22, not %s.", shown(v))
		}
		return d.String(), ""
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
		if v == nil {
			parts = append(parts, d.Label+" cleared")
			continue
		}
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
	return strings.Join(parts, ", ")
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// LinkFields turns the page names a sheet's link fields were given into
// those pages' ids, resolved like @[Name] links: pages this paste saved
// first, then pages of the campaign the operator can see. Whether the page
// is of the field's category is the entity service's check, at the write.
func LinkFields(defs []entities.FieldDefinition, checked map[string]any, links *PageLinks) []string {
	var problems []string
	for _, d := range defs {
		if d.Type != entities.FieldTypeLink {
			continue
		}
		name, ok := checked[d.Key].(string)
		if !ok || name == "" {
			continue
		}
		if links == nil {
			problems = append(problems, fmt.Sprintf("%s: pages can't be looked up here.", d.Label))
			continue
		}
		l, found := links.Resolve(name)
		if !found {
			problems = append(problems, fmt.Sprintf("%s: there is no page called %q (a page this paste makes must come before the page linking to it).", d.Label, name))
			continue
		}
		checked[d.Key] = l.ID
	}
	return problems
}

package importer

import (
	"encoding/json"
	"errors"
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

// maxSheetFormula caps the review screen's formula values; a creature's
// whole sheet is far smaller.
const maxSheetFormula = 64 << 10

// UseSheetFormula replaces fields the AI wrote with the values the review
// screen's game-system check worked out, for the numbers the director chose
// "Use the formula" on. raw is that screen's JSON object of key → value. Only
// a key the AI wrote is replaced (matched ignoring case, kept under the AI's
// spelling), so this never adds a field (UseDeclaredFormula adds the ones a
// system declares); the committer then checks
// the result like any other `fields:`. Input that is too large or unreadable
// changes nothing and is reported, so a director who pressed the button is
// never left thinking the formula was applied.
func UseSheetFormula(fields map[string]any, raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fields, nil
	}
	if len(raw) > maxSheetFormula {
		return fields, errors.New("the game system's numbers for this page are too large to apply, so nothing was changed")
	}
	var formula map[string]any
	if err := json.Unmarshal([]byte(raw), &formula); err != nil {
		return fields, errors.New("the game system's numbers for this page could not be read, so nothing was changed")
	}
	if len(fields) == 0 {
		return fields, nil
	}
	byKey := make(map[string]string, len(fields))
	for k := range fields {
		byKey[strings.ToLower(strings.TrimSpace(k))] = k
	}
	for k, v := range formula {
		if orig, ok := byKey[strings.ToLower(strings.TrimSpace(k))]; ok {
			fields[orig] = v
		}
	}
	return fields, nil
}

// UseDeclaredFormula adds to fields the review screen's formula values for
// numbers the AI left out, but only for a key the category defines and the
// game system marked as a formula figure (FieldDefinition.Formula). Keys the
// AI wrote are UseSheetFormula's, and any other key is dropped, so the review
// can never invent a field. It returns a new map and leaves fields as it was;
// the committer checks the result like any other `fields:`. Unreadable input
// adds nothing (UseSheetFormula has already reported it).
func UseDeclaredFormula(defs []entities.FieldDefinition, fields map[string]any, raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxSheetFormula {
		return fields
	}
	var formula map[string]any
	if err := json.Unmarshal([]byte(raw), &formula); err != nil {
		return fields
	}
	declared := make(map[string]string)
	for _, d := range defs {
		if d.Formula {
			declared[strings.ToLower(strings.TrimSpace(d.Key))] = d.Key
		}
	}
	written := make(map[string]bool, len(fields))
	for k := range fields {
		written[strings.ToLower(strings.TrimSpace(k))] = true
	}
	out := make(map[string]any, len(fields)+len(formula))
	for k, v := range fields {
		out[k] = v
	}
	for k, v := range formula {
		lk := strings.ToLower(strings.TrimSpace(k))
		if key, ok := declared[lk]; ok && !written[lk] {
			out[key] = v
		}
	}
	return out
}

// fieldValue checks one value against its field's type, returning what to
// store. The types are the sheet form's input types.
func fieldValue(def entities.FieldDefinition, v any) (any, string) {
	if v == nil {
		return nil, ""
	}
	switch def.Type {
	case "number":
		n, ok := numberOf(v)
		if !ok {
			return nil, fmt.Sprintf("needs a number, not %s.", shown(v))
		}
		if p := rangeProblem(def, n); p != "" {
			return nil, p
		}
		if i, isInt := v.(int); isInt {
			return i, ""
		}
		if i, isInt := v.(int64); isInt {
			return i, ""
		}
		if n == math.Trunc(n) && math.Abs(n) < 1e15 {
			return int(n), ""
		}
		return n, ""
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
	case entities.FieldTypeClimate:
		// Climate ids are lower-case words joined by hyphens; the page save
		// refuses one the calendar doesn't list, naming the ones it does.
		s, ok := scalarText(v)
		if !ok {
			return nil, "needs a climate id, such as desert or cold-coast."
		}
		return strings.Join(strings.Fields(strings.ToLower(s)), "-"), ""
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
		text := ""
		if s, ok := scalarText(v); ok {
			text = s
		} else {
			b, err := json.Marshal(jsonSafe(v))
			if err != nil {
				return nil, "could not be read as text or a list."
			}
			text = string(b)
		}
		// A list field's text is read by the system's widgets as JSON, so text
		// that is not a list would save and then draw as empty.
		if def.JSONList && text != "" {
			var list []any
			if json.Unmarshal([]byte(text), &list) != nil {
				return nil, "needs a list of entries, not plain text."
			}
		}
		return text, ""
	}
}

// numberOf reads a number from the shapes YAML and the paste hand over,
// refusing NaN and infinity so a bad figure never reaches a sheet.
func numberOf(v any) (float64, bool) {
	var f float64
	switch n := v.(type) {
	case int:
		f = float64(n)
	case int64:
		f = float64(n)
	case float64:
		f = n
	case string:
		p, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		f = p
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// rangeProblem is the plain-words problem when n is outside the bounds the
// system declared for the field, or "" when it is inside or unbounded.
func rangeProblem(def entities.FieldDefinition, n float64) string {
	switch {
	case def.Min != nil && def.Max != nil && (n < *def.Min || n > *def.Max):
		return fmt.Sprintf("needs a number from %s to %s, not %s.", num(*def.Min), num(*def.Max), num(n))
	case def.Min != nil && n < *def.Min:
		return fmt.Sprintf("needs a number of at least %s, not %s.", num(*def.Min), num(n))
	case def.Max != nil && n > *def.Max:
		return fmt.Sprintf("needs a number of at most %s, not %s.", num(*def.Max), num(n))
	}
	return ""
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

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

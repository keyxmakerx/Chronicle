package rolltables

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The conditions mirror the client engine's `when` (checkWhen in
// chronicle_gen.js): a line only rolls on days that meet all of them. The
// service stores the validated shape so a saved table behaves on a calendar
// day exactly as the builder's test rolls did.
const (
	maxWhenList  = 40
	maxWhenName  = 60
	maxWhenDepth = 3
	maxMoonDays  = 30
)

var (
	whenMoonPhases = map[string]bool{
		"full": true, "new": true, "waxing": true, "waning": true,
		"first-quarter": true, "last-quarter": true, "crescent": true, "gibbous": true,
	}
	// season, month and weekday take names or, for the last two, numbers.
	whenNameLists   = []string{"season", "weather", "climate"}
	whenNumberLists = []string{"month", "weekday"}
)

// normalizeWhen validates one line's conditions and returns them in compact
// form. A null or empty object means "any day" and is dropped (nil).
func normalizeWhen(raw json.RawMessage, label string) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, errInvalid(label + ": \"when\" must be a set of conditions like {\"season\":[\"winter\"]}")
	}
	clean, err := cleanWhen(m, label, 0)
	if err != nil {
		return nil, err
	}
	if len(clean) == 0 {
		return nil, nil
	}
	out, err := json.Marshal(clean)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	return out, nil
}

func cleanWhen(m map[string]json.RawMessage, label string, depth int) (map[string]any, error) {
	out := make(map[string]any, len(m))
	bad := func(msg string) error { return errInvalid(label + ": " + msg) }
	for k, v := range m {
		switch {
		case contains(whenNameLists, k):
			list, err := whenList(v, false)
			if err != nil {
				return nil, bad("\"" + k + "\" must be a short list of names")
			}
			out[k] = list
		case contains(whenNumberLists, k):
			list, err := whenList(v, true)
			if err != nil {
				return nil, bad("\"" + k + "\" must be a short list of names or numbers")
			}
			out[k] = list
		case k == "rest" || k == "festival":
			var b bool
			if err := json.Unmarshal(v, &b); err != nil {
				return nil, bad("\"" + k + "\" must be true or false")
			}
			out[k] = b
		case k == "chance":
			var f float64
			if err := json.Unmarshal(v, &f); err != nil || f < 0 || f > 1 {
				return nil, bad("\"chance\" must be a number from 0 to 1")
			}
			out[k] = f
		case k == "moon":
			moon, err := cleanMoon(v)
			if err != nil {
				return nil, bad(err.Error())
			}
			out[k] = moon
		case k == "not":
			if depth+1 >= maxWhenDepth {
				return nil, bad("\"not\" is nested too deeply")
			}
			var inner map[string]json.RawMessage
			if err := json.Unmarshal(v, &inner); err != nil {
				return nil, bad("\"not\" must be a set of conditions")
			}
			c, err := cleanWhen(inner, label, depth+1)
			if err != nil {
				return nil, err
			}
			out[k] = c
		default:
			return nil, bad("\"" + k + "\" is not a condition Chronicle knows (season, month, weekday, moon, weather, climate, rest, festival, chance, not)")
		}
	}
	return out, nil
}

// whenList reads a bounded list of non-empty names, or of names and whole
// numbers when numbers is set.
func whenList(raw json.RawMessage, numbers bool) ([]any, error) {
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil || len(items) > maxWhenList {
		return nil, fmt.Errorf("bad list")
	}
	out := make([]any, 0, len(items))
	for _, it := range items {
		switch x := it.(type) {
		case string:
			x = strings.TrimSpace(x)
			if x == "" || utf8.RuneCountInString(x) > maxWhenName {
				return nil, fmt.Errorf("bad name")
			}
			out = append(out, x)
		case float64:
			if !numbers || x != float64(int(x)) || x < 1 || x > 1000 {
				return nil, fmt.Errorf("bad number")
			}
			out = append(out, int(x))
		default:
			return nil, fmt.Errorf("bad item")
		}
	}
	return out, nil
}

func cleanMoon(raw json.RawMessage) (map[string]any, error) {
	var in struct {
		Name   string   `json:"name"`
		Phase  string   `json:"phase"`
		Within *float64 `json:"within"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("\"moon\" must look like {\"name\":\"any\",\"phase\":\"full\",\"within\":1}")
	}
	out := map[string]any{}
	if n := strings.TrimSpace(in.Name); n != "" {
		if utf8.RuneCountInString(n) > maxWhenName {
			return nil, fmt.Errorf("the moon's name is too long")
		}
		out["name"] = n
	}
	if in.Phase != "" {
		if !whenMoonPhases[in.Phase] {
			return nil, fmt.Errorf("\"%s\" is not a moon phase", in.Phase)
		}
		out["phase"] = in.Phase
	}
	if in.Within != nil {
		if *in.Within < 0 || *in.Within > maxMoonDays {
			return nil, fmt.Errorf("the moon's \"within\" must be 0 to %d days", maxMoonDays)
		}
		out["within"] = *in.Within
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

package fieldrules

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Gives is what a rule gives for the followed field's value: the first row
// that matches, or "" when none does or the value is blank. Pure, so the
// matching rules are pinned by table tests.
func (r *Rule) Gives(value any) string {
	text := valueText(value)
	if text == "" {
		return ""
	}
	num, isNum := parseNumber(text)
	for _, row := range r.Rows {
		if row.From != nil || row.To != nil {
			if !isNum {
				continue
			}
			if row.From != nil && num < *row.From {
				continue
			}
			if row.To != nil && num > *row.To {
				continue
			}
			return row.Gives
		}
		if strings.EqualFold(strings.TrimSpace(row.Is), text) {
			return row.Gives
		}
		if n, ok := parseNumber(row.Is); ok && isNum && n == num {
			return row.Gives
		}
	}
	return ""
}

// valueText is a stored field value as the text a row is matched against.
func valueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func parseNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// ChooseWords picks, for each word, the local name a page sees: the nearest
// place in places (the page itself first, then up its parents) that names it,
// else the era. Words kept DM only count only for the Director. The result is
// keyed by the lower-cased usual word.
func ChooseWords(words []LocalWord, places []string, eraID int, director bool) map[string]string {
	rank := make(map[string]int, len(places))
	for i, p := range places {
		if _, seen := rank[p]; !seen {
			rank[p] = i
		}
	}
	type pick struct {
		local string
		at    int // place rank; len(places) for an era
	}
	best := map[string]pick{}
	for _, w := range words {
		if w.DMOnly && !director {
			continue
		}
		at := -1
		switch {
		case w.PlaceID != "":
			if r, ok := rank[w.PlaceID]; ok {
				at = r
			}
		case w.EraID != 0 && w.EraID == eraID:
			at = len(places)
		}
		if at < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(w.Word))
		if cur, ok := best[key]; !ok || at < cur.at {
			best[key] = pick{local: w.Local, at: at}
		}
	}
	out := make(map[string]string, len(best))
	for k, p := range best {
		out[k] = p.local
	}
	return out
}

// Rename returns the local name for text when one applies. A plural made by
// adding "s" ("Marks") finds the singular word too, since prices are often
// written that way; the local word is used as the owner wrote it.
func Rename(words map[string]string, text string) (string, bool) {
	if len(words) == 0 {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(text))
	if key == "" {
		return "", false
	}
	if l, ok := words[key]; ok {
		return l, true
	}
	if strings.HasSuffix(key, "s") {
		if l, ok := words[strings.TrimSuffix(key, "s")]; ok {
			return l, true
		}
	}
	return "", false
}

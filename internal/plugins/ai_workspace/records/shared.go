package records

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// KindCalendar is the calendar block's `kind:`, the calendar's own slug,
// which also names the prompt's Calendar content choice. The calendar's AI
// part (calendar/calendarai) defines the block; the engine only names it.
const KindCalendar = calendar.PluginSlug

// CalendarKinds are the kinds the prompt's Calendar content choice
// explains: the calendar itself and everything written onto it.
var CalendarKinds = []string{KindCalendar, "event", "weather", "generator"}

// CalendarLookups are the lookups the prompt's Calendar content choice
// answers up front: the calendar itself (or how to set one up) and its
// kinds of weather. The calendar's AI part answers them.
func CalendarLookups() []Record {
	return []Record{
		{Kind: KindLookup, Fields: map[string]any{"what": KindCalendar}},
		{Kind: KindLookup, Fields: map[string]any{"what": "weather-kinds"}},
	}
}

// plural is "1 moon" or "3 moons".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// listItems reads a list key whose entries are maps (or, where allowed,
// bare names) as Records with lower-cased keys.
func listItems(r Record, key string, names bool) ([]Record, error) {
	raw := r.List(key)
	out := make([]Record, 0, len(raw))
	for i, v := range raw {
		switch t := v.(type) {
		case map[string]any:
			f := make(map[string]any, len(t))
			for k, val := range t {
				f[strings.ToLower(strings.TrimSpace(k))] = val
			}
			out = append(out, Record{Name: strings.TrimSpace(fmt.Sprint(f["name"])), Fields: f})
		case string:
			if !names {
				return nil, badRequestf("%s: entry %d needs keys such as name", key, i+1)
			}
			out = append(out, Record{Name: strings.TrimSpace(t), Fields: map[string]any{"name": t}})
		default:
			return nil, badRequestf("%s: entry %d is not readable", key, i+1)
		}
		if f := out[len(out)-1].Fields["name"]; f == nil || strings.TrimSpace(fmt.Sprint(f)) == "" {
			return nil, badRequestf("%s: entry %d needs a name", key, i+1)
		}
	}
	return out, nil
}

func lowerMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return out
}

// decodeMeta reads a relation's JSON metadata; bad JSON reads as empty.
func decodeMeta(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

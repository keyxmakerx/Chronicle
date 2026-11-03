package calendarai

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

const camp = "camp-1"

var owner = Actor{UserID: "owner", Role: 3}
var player = Actor{UserID: "pl", Role: 1}

// rec is a record as the engine hands it over, its body rendered by the
// engine's own Text (no paste links).
func rec(kind, action, name string, fields map[string]any, body string) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return Record{Kind: kind, Action: action, Name: name, Fields: fields, Body: body, Text: records.BodyText(nil)}
}

func lookup(what string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["what"] = what
	r := Record{Kind: "lookup", Fields: fields}
	if n, ok := fields["name"].(string); ok {
		r.Name = n
	}
	return r
}

// answer is one lookup's result as the engine shows it.
type answer struct{ Chip, Text, Error string }

// Answer answers lookups the way the engine does: by `what:` (or an
// alias), with a refusal's plain wording.
func (l *Lookups) Answer(ctx context.Context, campaignID string, a Actor, recs []Record) []answer {
	var out []answer
	for _, r := range recs {
		what := strings.ToLower(r.Str("what"))
		var ans answer
		ans.Error = "not something Chronicle can look up"
		for _, p := range l.All() {
			in := p.LookupInfo()
			if in.What != what && !contains(in.Aliases, what) {
				continue
			}
			chip, text, err := p.Answer(ctx, campaignID, a, r)
			ans = answer{Chip: chip, Text: text}
			if err != nil {
				ans.Text, ans.Error = "", planError(err)
			}
		}
		out = append(out, ans)
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// fakeTables is the campaign's rolling tables by name.
type fakeTables struct {
	ids   map[string]string // name -> id
	added map[string][]string
}

func (f *fakeTables) FindTable(_ context.Context, _, name string) (string, error) {
	for n, id := range f.ids {
		if strings.EqualFold(n, name) || id == name {
			return id, nil
		}
	}
	return "", nil
}

func (f *fakeTables) AddNames(_ context.Context, _ string, _ Actor, table string, names []string) error {
	if f.added == nil {
		f.added = map[string][]string{}
	}
	f.added[table] = append(f.added[table], names...)
	return nil
}

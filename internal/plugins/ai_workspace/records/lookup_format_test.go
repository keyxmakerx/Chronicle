package records

import (
	"context"
	"strings"
	"testing"
)

func TestLookups_Format(t *testing.T) {
	l := &Lookups{Kinds: NewRegistry(TableKind{}, PinKind{})}
	tests := []struct {
		name, block, want, err string
	}{
		{"a kind's doc", "table", "A rolling table, matched by `name`", ""},
		{"normalised", " Pin ", "## Format: kind: pin", ""},
		{"unknown lists the kinds", "dragon", "", "the kinds are pin, table"},
		{"missing", "", "", "it needs `block:`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatFormat, map[string]any{"block": tt.block})})[0]
			if tt.err != "" {
				if !strings.Contains(ans.Error, tt.err) {
					t.Fatalf("error %q, want %q", ans.Error, tt.err)
				}
				return
			}
			if ans.Error != "" || !strings.Contains(ans.Text, tt.want) {
				t.Fatalf("answer %+v lacks %q", ans, tt.want)
			}
		})
	}
	if got := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatFormat, map[string]any{"block": "table"})})[0]; got.Error == "" {
		t.Error("a server without the registry must say formats can't be looked up")
	}
}

// Not answered lines never end in a doubled full stop.
func TestMarkdown_NoDoubleStop(t *testing.T) {
	md := Markdown([]LookupAnswer{{Chip: "X", Error: "choose one on the Calendars page."}})
	if strings.Contains(md, "..") {
		t.Errorf("%q", md)
	}
}

func TestRegistry_DocsFor(t *testing.T) {
	reg := NewRegistry(PinKind{}, TableKind{}, NoteKind{})
	got := reg.DocsFor("table", "pin", "dragon")
	if !strings.Contains(got, "### kind: table") || !strings.Contains(got, "### kind: pin") || strings.Contains(got, "kind: note") {
		t.Errorf("docs: %s", got)
	}
	if strings.Index(got, "kind: table") > strings.Index(got, "kind: pin") {
		t.Error("docs must follow the order asked for")
	}
}

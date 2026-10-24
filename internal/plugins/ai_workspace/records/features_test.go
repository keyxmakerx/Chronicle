package records

import (
	"strings"
	"testing"
)

func TestRegistry_FindsUnderscoredKind(t *testing.T) {
	r := NewRegistry(RollNightKind{})
	for _, name := range []string{"roll_night", "roll-night", "Roll_Night"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("Get(%q) found nothing", name)
		}
	}
}

func TestOffFeatures_For(t *testing.T) {
	off := OffFeatures{KindCalendar: "Calendar", "event": "Calendar", "roll-night": "Calendar", "map": "Maps"}
	cases := []struct {
		rec  Record
		want string
	}{
		{rec("event", ActionCreate, "Fair", nil, ""), "Calendar"},
		{rec("roll_night", ActionCreate, "x", nil, ""), "Calendar"},
		{rec("MAP", ActionCreate, "Keep", nil, ""), "Maps"},
		{rec("table", ActionCreate, "Loot", nil, ""), ""},
		{rec("generator", ActionCreate, "", map[string]any{"generator": "weather"}, ""), "Calendar"},
		{rec("generator", ActionCreate, "", map[string]any{"generator": "names"}, ""), ""},
	}
	for _, c := range cases {
		if got := off.For(c.rec); got != c.want {
			t.Errorf("For(%s %v) = %q, want %q", c.rec.Kind, c.rec.Fields, got, c.want)
		}
	}
	if got := OffFeatures(nil).For(rec("event", ActionCreate, "x", nil, "")); got != "" {
		t.Errorf("nil gate refused a record: %q", got)
	}
}

func TestRegistry_WithoutAndNote(t *testing.T) {
	r := NewRegistry(EventKind{}, TableKind{}, PinKind{}, GeneratorKind{})
	off := OffFeatures{"event": "Calendar", KindCalendar: "Calendar", "pin": "Maps"}
	caps := r.Without(off).Capabilities()
	for _, gone := range []string{"kind: event", "kind: pin"} {
		if strings.Contains(caps, gone) {
			t.Errorf("switched-off %s still offered:\n%s", gone, caps)
		}
	}
	for _, kept := range []string{"kind: table", "kind: generator"} {
		if !strings.Contains(caps, kept) {
			t.Errorf("%s missing:\n%s", kept, caps)
		}
	}
	if r.Without(nil) != r {
		t.Error("nothing switched off should keep the registry")
	}
	note := off.Note()
	for _, want := range []string{"Calendar (`calendar`, `event`, `generator` runs other than names)", "Maps (`pin`)", "Game & features"} {
		if !strings.Contains(note, want) {
			t.Errorf("note missing %q:\n%s", want, note)
		}
	}
	if (OffFeatures{}).Note() != "" {
		t.Error("note written with nothing switched off")
	}
}

package records

import (
	"context"
	"strings"
	"testing"
)

type fakeGallery struct {
	list  []GalleryRef
	total int
	seen  [4]string
}

func (f *fakeGallery) Browse(_ context.Context, _ string, _ Actor, kind, text string, all bool) ([]GalleryRef, int, error) {
	f.seen = [4]string{kind, text, map[bool]string{true: "all", false: "cur"}[all]}
	return f.list, f.total, nil
}

func TestGalleryLookup(t *testing.T) {
	refs := []GalleryRef{{Name: "Moonsilver Ring", Note: "A ward.", Kind: "crafting-circle", SystemID: "drawsteel", Maker: "Sunniva", Summary: "ring: 7", Lacks: []string{"No moonsilver."}}}
	tests := []struct {
		name   string
		total  int
		fields map[string]any
		actor  Actor
		want   []string
		not    []string
		err    bool
	}{
		{"all of them", 1, nil, designOwner, []string{"## Shared designs (1)", "Moonsilver Ring", "by Sunniva", "Lacks here: No moonsilver."}, []string{" of "}, false},
		{"truncated says N of M", 40, nil, designOwner, []string{"## Shared designs (1 of 40)", "Only the newest 1"}, nil, false},
		{"filters pass through", 1, map[string]any{"kind": "Rooms", "q": "moon", "all": "yes"}, designOwner, nil, nil, false},
		{"a player is refused", 1, nil, Actor{UserID: "u2", Role: 1}, nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, _ := designKind(true)
			f := &fakeGallery{list: refs, total: tc.total}
			chip, text, err := GalleryLookup{Svc: f, Owners: k.Owners}.Answer(context.Background(), "c1", tc.actor, Record{Fields: tc.fields})
			if (err != nil) != tc.err {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				return
			}
			if chip != "Shared designs" {
				t.Errorf("chip = %q", chip)
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("answer lacks %q:\n%s", w, text)
				}
			}
			for _, w := range tc.not {
				if strings.Contains(text, w) {
					t.Errorf("answer has %q:\n%s", w, text)
				}
			}
			if tc.fields != nil && (f.seen[0] != "rooms" || f.seen[1] != "moon" || f.seen[2] != "all") {
				t.Errorf("filters = %v", f.seen)
			}
		})
	}
	if info := (GalleryLookup{}).LookupInfo(); info.What != "gallery" || !info.Tool.Owner {
		t.Errorf("info = %+v", info)
	}
}

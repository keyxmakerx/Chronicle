package systems

import (
	"context"
	"strings"
	"testing"
)

// A detail the Director lists in director_only reaches no player view: not
// the list, the pick list, a lookup by name, nor the list itself; Directors
// read it all.
func TestSystemEntryService_HiddenDetails(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	ctx := context.Background()
	props := map[string]any{"benefit": "Gain a surge.", "drawback": "Your shadow rises.", EntryHiddenKey: []any{"drawback"}}
	if _, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "The Red Tide", Properties: props}); err != nil {
		t.Fatal(err)
	}
	ch, _ := svc.Choices(ctx, "c1", "ancestry")
	pl, _ := svc.List(ctx, "c1", "ancestry", playerActor)
	pf, _ := svc.FindByName(ctx, "c1", "ancestry", "the red tide", playerActor)
	dl, _ := svc.List(ctx, "c1", "ancestry", dirActor)
	for name, p := range map[string]map[string]any{"choice": ch[0].Properties, "player list": pl[0].Properties, "player find": pf.Properties} {
		if _, ok := p["drawback"]; ok {
			t.Errorf("%s shows the drawback: %v", name, p)
		}
		if _, ok := p[EntryHiddenKey]; ok {
			t.Errorf("%s shows the hidden list: %v", name, p)
		}
		if p["benefit"] != "Gain a surge." {
			t.Errorf("%s lost the benefit: %v", name, p)
		}
	}
	if dl[0].Properties["drawback"] != "Your shadow rises." || dl[0].Properties[EntryHiddenKey] == nil {
		t.Errorf("director list: %v", dl[0].Properties)
	}
	if dc := dirChoice(t, svc); dc[0].Properties["drawback"] == nil {
		t.Errorf("director choice: %v", dc[0].Properties)
	}
}

// The list may only name details the entry has, so a typo never leaves a
// detail open that was meant to be hidden.
func TestValidateHiddenList(t *testing.T) {
	cases := []struct {
		name string
		p    map[string]any
		want string
	}{
		{"names a detail", map[string]any{"drawback": "x", EntryHiddenKey: []any{"drawback"}}, ""},
		{"no list", map[string]any{"drawback": "x"}, ""},
		{"typo", map[string]any{"drawback": "x", EntryHiddenKey: []any{"drawbak"}}, "does not have"},
		{"not a list", map[string]any{"drawback": "x", EntryHiddenKey: "drawback"}, "must be a list"},
		{"names itself", map[string]any{EntryHiddenKey: []any{EntryHiddenKey}}, "must list detail names"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEntryProperties(tc.p)
			if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

// A player's save never sees the hidden details, so they come back as
// stored, and a player cannot hide anything.
func TestKeepHidden(t *testing.T) {
	stored := map[string]any{"benefit": "a", "drawback": "secret", EntryHiddenKey: []any{"drawback"}}
	got := keepHidden(map[string]any{"benefit": "b"}, stored)
	if got["benefit"] != "b" || got["drawback"] != "secret" || got[EntryHiddenKey] == nil {
		t.Fatalf("kept: %v", got)
	}
	got = keepHidden(map[string]any{"benefit": "b", EntryHiddenKey: []any{"benefit"}}, map[string]any{})
	if _, ok := got[EntryHiddenKey]; ok {
		t.Fatalf("a player hid a detail: %v", got)
	}
}

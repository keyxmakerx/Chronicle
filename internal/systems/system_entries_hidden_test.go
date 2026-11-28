package systems

import (
	"context"
	"fmt"
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

// A GM-only part inside a detail's text (a signature trait, a list of
// abilities) reaches no player view, at any depth; the rest of the text does,
// and Directors read it all.
func TestSystemEntryService_SecretsInDetailsHiddenFromPlayers(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	ctx := context.Background()
	secret := `<p>Quiet. <span data-secret="true">DIRSECRET</span></p>`
	props := map[string]any{
		"speed":            5,
		"signature_traits": []any{map[string]any{"name": "Hidden", "description": secret}},
		"note":             secret,
		"plain":            "No <markup> & no secret",
	}
	if _, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Shade", Properties: props}); err != nil {
		t.Fatal(err)
	}
	ch, _ := svc.Choices(ctx, "c1", "ancestry")
	pl, _ := svc.List(ctx, "c1", "ancestry", playerActor)
	pf, _ := svc.FindByName(ctx, "c1", "ancestry", "shade", playerActor)
	dl, _ := svc.List(ctx, "c1", "ancestry", dirActor)
	text := func(p map[string]any) string { return fmt.Sprint(p) }
	for name, p := range map[string]map[string]any{"choice": ch[0].Properties, "player list": pl[0].Properties, "player find": pf.Properties} {
		if strings.Contains(text(p), "DIRSECRET") {
			t.Errorf("%s shows the GM-only part: %v", name, p)
		}
		if !strings.Contains(text(p), "Quiet.") || !strings.Contains(text(p), "Hidden") {
			t.Errorf("%s lost the rest of the text: %v", name, p)
		}
		if p["speed"] != 5 {
			t.Errorf("%s changed a number: %v", name, p["speed"])
		}
	}
	if !strings.Contains(text(dl[0].Properties), "DIRSECRET") {
		t.Errorf("director list lost the GM-only part: %v", dl[0].Properties)
	}
	if !strings.Contains(text(dirChoice(t, svc)[0].Properties), "DIRSECRET") {
		t.Error("director choice lost the GM-only part")
	}
	// The entry as stored is untouched by a player's read.
	if !strings.Contains(text(props), "DIRSECRET") {
		t.Error("PlayerProperties changed the stored details")
	}
}

func TestPlayerProperties_StripsAtAnyDepth(t *testing.T) {
	p := map[string]any{
		"a": []string{`x <span data-secret="true">S1</span>`, "plain"},
		"b": map[string]any{"c": []any{`<span data-secret="true">S2</span>y`}},
		"n": nil,
	}
	got := fmt.Sprint(PlayerProperties(p))
	if strings.Contains(got, "S1") || strings.Contains(got, "S2") {
		t.Errorf("secret kept: %s", got)
	}
	if !strings.Contains(got, "plain") || !strings.Contains(got, "y") {
		t.Errorf("text lost: %s", got)
	}
	if PlayerProperties(nil) != nil {
		t.Error("nil in, nil out")
	}
}

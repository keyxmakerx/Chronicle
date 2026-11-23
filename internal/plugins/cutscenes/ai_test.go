package cutscenes

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	aDirector = aiplug.Actor{UserID: "dir", Role: permissions.RoleOwner}
	aPlayer   = aiplug.Actor{UserID: "ana", Role: permissions.RolePlayer}
	aOwnKey   = aiplug.Actor{UserID: "ana", Role: permissions.RolePlayer, ReadsOwnNotes: true}
)

func newAIRig(on bool) (*rig, *AI) {
	r := newRig()
	return r, NewAI(r.svc, func(context.Context, string) bool { return on })
}

func rec(action, name string, fields map[string]any) aiplug.Record {
	f := map[string]any{"kind": KindName, "action": action, "name": name}
	for k, v := range fields {
		f[k] = v
	}
	return aiplug.Record{Kind: KindName, Action: action, Name: name, Fields: f}
}

// The eye scene as an AI writes it: YAML maps arrive as map[any]any.
func eyeFields() map[string]any {
	return map[string]any{
		"background": map[any]any{"kind": "fall-asleep", "seconds": 7},
		"overlays":   []any{map[any]any{"kind": "eye", "at": 7.5, "seconds": 3.5, "color": "#3dff7a"}},
		"text":       []any{map[any]any{"say": "Something has noticed you.", "at": 11, "seconds": 5, "style": "whisper"}},
		"ending":     map[any]any{"kind": "tap"},
	}
}

func TestAIPlan(t *testing.T) {
	r, ai := newAIRig(true)
	existing := r.scene(t, "Existing")
	k := ai.Kinds()[0]
	tests := []struct {
		name    string
		actor   aiplug.Actor
		on      bool
		r       aiplug.Record
		wantErr string
		want    []string
	}{
		{name: "create reads every layer in words", actor: aDirector, on: true, r: rec("create", "The Eye Opens", eyeFields()),
			want: []string{`makes the cut scene "The Eye Opens"`, "Background: falling asleep over 7 s", "On top: green eye at 7.5 s", `Words: "Something has noticed you." at 11 s for 5 s, whisper`, "Ending: closes when they tap"}},
		{name: "the editor's section names are read too", actor: aDirector, on: true, r: rec("create", "Aliased", map[string]any{"on_top": []any{map[string]any{"kind": "embers"}}, "words": []any{map[string]any{"say": "hi"}}, "background": "black"}),
			want: []string{"Background: black at once", "On top: orange embers at 0 s", `Words: "hi" at 0 s for 5 s`}},
		{name: "a number out of range is clamped and said", actor: aDirector, on: true, r: rec("create", "Clamped", map[string]any{"overlays": []any{map[string]any{"kind": "eye", "at": 500}}}),
			want: []string{"On top: green eye at 120 s"}},
		{name: "looks are read and named in words", actor: aDirector, on: true, r: rec("create", "Looks", map[string]any{
			"background": map[any]any{"kind": "fall-asleep", "seconds": 7, "look": "eyelids"},
			"overlays":   []any{map[any]any{"kind": "eye", "at": 1, "look": "rift"}, map[any]any{"kind": "eye", "params": map[any]any{"look": "almond"}}}}),
			want: []string{`Background: falling asleep over 7 s, look "Heavy eyelids"`, `look "Burning seam"`, `look "Lids part"`}},
		{name: "no look names the default", actor: aDirector, on: true, r: rec("create", "Default look", eyeFields()),
			want: []string{`look "The page drifts off" (the default)`, `look "Sparks gather" (the default)`}},
		{name: "an unknown look is refused by name", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"background": map[string]any{"kind": "fall-asleep", "look": "swirl"}}),
			wantErr: `"swirl" is not a look of fall-asleep; it is one of drift, eyelids, ink`},
		{name: "an eye look from the wrong list is refused", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"overlays": []any{map[string]any{"kind": "eye", "look": "ink"}}}),
			wantErr: `"ink" is not a look of eye`},
		{name: "a look on a background without looks is refused", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"background": map[string]any{"kind": "black", "look": "drift"}}),
			wantErr: `"black" has no looks`},
		{name: "a look on embers is refused", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"overlays": []any{map[string]any{"kind": "embers", "look": "gather"}}}),
			wantErr: `"embers" has no looks`},
		{name: "add-on off", actor: aDirector, on: false, r: rec("create", "X", eyeFields()), wantErr: "switched off"},
		{name: "a player's key", actor: aPlayer, on: true, r: rec("create", "X", eyeFields()), wantErr: "only the Director"},
		{name: "a member's own key", actor: aOwnKey, on: true, r: rec("create", "X", eyeFields()), wantErr: "only the Director"},
		{name: "no name", actor: aDirector, on: true, r: rec("create", "", eyeFields()), wantErr: "needs a name"},
		{name: "showing it is refused", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"show_to": []any{"Ana"}}), wantErr: "the AI never shows one"},
		{name: "a rewatch time is refused", actor: aDirector, on: true, r: rec("update", "Existing", map[string]any{"rewatch": "12h"}), wantErr: "the AI never shows one"},
		{name: "players are refused", actor: aDirector, on: true, r: rec("update", "Existing", map[string]any{"players": "all"}), wantErr: "the AI never shows one"},
		{name: "an unknown key is refused by name", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"music": "drums"}), wantErr: "`music` is not a key"},
		{name: "an unknown nested key is refused by name", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"overlays": []any{map[string]any{"kind": "eye", "size": 3}}}), wantErr: `no key "size"`},
		{name: "an unknown kind is refused", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"overlays": []any{map[string]any{"kind": "dragon"}}}), wantErr: `"dragon" is not one of eye, embers`},
		{name: "a bad colour is refused", actor: aDirector, on: true, r: rec("create", "X", map[string]any{"overlays": []any{map[string]any{"kind": "eye", "color": "green"}}}), wantErr: "#rrggbb"},
		{name: "create over an existing name", actor: aDirector, on: true, r: rec("create", "existing", eyeFields()), wantErr: "already exists"},
		{name: "update a missing scene", actor: aDirector, on: true, r: rec("update", "Missing", eyeFields()), wantErr: "no cut scene called"},
		{name: "update with nothing", actor: aDirector, on: true, r: rec("update", "Existing", nil), wantErr: "nothing to change"},
		{name: "update says only what changes", actor: aDirector, on: true, r: rec("update", "Existing", map[string]any{"ending": map[string]any{"kind": "auto", "after": 4}, "rename_to": "Renamed"}),
			want: []string{`changes the cut scene "Existing"`, `renames it to "Renamed"`, "Ending: closes by itself 4 s after the last layer"}},
		{name: "delete takes the name only", actor: aDirector, on: true, r: rec("delete", "Existing", map[string]any{"text": []any{}}), wantErr: "nothing else"},
		{name: "delete warns", actor: aDirector, on: true, r: rec("delete", "Existing", nil), want: []string{`deletes the cut scene "Existing" for good`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewAI(r.svc, func(context.Context, string) bool { return tt.on }).Kinds()[0]
			p := k.Plan(ctx, "camp-1", tt.actor, tt.r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q, want %q (summary %q)", p.Error, tt.wantErr, p.Summary)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan error %q", p.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(p.Summary, w) {
					t.Errorf("summary %q lacks %q", p.Summary, w)
				}
			}
		})
	}
	_ = k
	_ = existing
}

func TestAIApply(t *testing.T) {
	r, ai := newAIRig(true)
	k := ai.Kinds()[0]
	if err := k.Apply(ctx, "camp-1", aDirector, rec("create", "The Eye Opens", eyeFields())); err != nil {
		t.Fatal(err)
	}
	list, _ := r.svc.List(ctx, "camp-1", vDir)
	if len(list) != 1 || list[0].Overlays[0].Params.Color != "#3dff7a" || list[0].Text[0].Style != StyleWhisper {
		t.Fatalf("stored %+v", list)
	}
	if err := k.Apply(ctx, "camp-1", aDirector, rec("update", "The Eye Opens", map[string]any{
		"background": map[string]any{"kind": "fall-asleep", "seconds": 7, "look": "ink"},
		"overlays":   []any{map[string]any{"kind": "eye", "at": 7.5, "seconds": 3.5, "color": "#3dff7a", "look": "almond"}}})); err != nil {
		t.Fatal(err)
	}
	list, _ = r.svc.List(ctx, "camp-1", vDir)
	if list[0].Background.Params.Look != "ink" || list[0].Overlays[0].Params.Look != "almond" {
		t.Fatalf("looks not stored: %+v", list[0])
	}
	if err := k.Apply(ctx, "camp-1", aDirector, rec("update", "the eye opens", map[string]any{"text": []any{}})); err != nil {
		t.Fatal(err)
	}
	list, _ = r.svc.List(ctx, "camp-1", vDir)
	if len(list[0].Text) != 0 || len(list[0].Overlays) != 1 {
		t.Errorf("update replaced more than the text: %+v", list[0])
	}
	// A player's key never writes, even straight to Apply.
	if err := k.Apply(ctx, "camp-1", aPlayer, rec("create", "Sneaky", eyeFields())); err == nil {
		t.Error("a player's apply went through")
	}
	// Delete needs the reviewed state.
	del := rec("delete", "The Eye Opens", nil)
	if err := k.Apply(ctx, "camp-1", aDirector, del); err == nil {
		t.Error("a delete without its reviewed state went through")
	}
	del.Seen = k.Plan(ctx, "camp-1", aDirector, del).Seen
	if err := k.Apply(ctx, "camp-1", aDirector, del); err != nil {
		t.Fatal(err)
	}
	if list, _ := r.svc.List(ctx, "camp-1", vDir); len(list) != 0 {
		t.Errorf("not deleted")
	}
	// The AI has no way to show a scene: nothing it applied went out live.
	if len(r.pub.shown) != 0 {
		t.Errorf("an AI write went out live")
	}
}

func TestAIDeleteRefusesAChangedScene(t *testing.T) {
	r, ai := newAIRig(true)
	k := ai.Kinds()[0]
	sc := r.scene(t, "One")
	del := rec("delete", "One", nil)
	del.Seen = k.Plan(ctx, "camp-1", aDirector, del).Seen
	edit := *sc
	edit.Text = []TextLine{{Say: "changed"}}
	if _, err := r.svc.Save(ctx, "camp-1", sc.ID, vDir, edit); err != nil {
		t.Fatal(err)
	}
	if err := k.Apply(ctx, "camp-1", aDirector, del); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Errorf("delete of a changed scene: %v", err)
	}
}

func TestAILookup(t *testing.T) {
	r, ai := newAIRig(true)
	r.scene(t, "The Eye Opens")
	l := ai.Lookup()
	info := l.LookupInfo()
	if info.What != "cut-scenes" || info.Tool.Name != "cut_scenes" || !info.Tool.Owner {
		t.Errorf("info %+v", info)
	}
	_, text, err := l.Answer(ctx, "camp-1", aDirector, aiplug.Record{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### The Eye Opens", "Background: falling asleep over 7 s", "kind: cut-scene", "`fall-asleep`", "`eye`", "`whisper`", "`auto`",
		"`drift` (The page drifts off, the default):", "`eyelids` (Heavy eyelids):", "`ink` (Ink in water):", "`gather` (Sparks gather, the default):", "`almond` (Lids part):", "`rift` (Burning seam):"} {
		if !strings.Contains(text, want) {
			t.Errorf("answer lacks %q", want)
		}
	}
	for _, a := range []aiplug.Actor{aPlayer, aOwnKey} {
		if _, _, err := l.Answer(ctx, "camp-1", a, aiplug.Record{}); err == nil {
			t.Errorf("%+v read the scenes", a)
		}
	}
	_, offAI := newAIRig(false)
	if _, _, err := offAI.Lookup().Answer(ctx, "camp-1", aDirector, aiplug.Record{}); err == nil {
		t.Error("read with the add-on off")
	}
	// The block the lookup gives can be pasted back as an update unchanged.
	list, _ := r.svc.List(ctx, "camp-1", vDir)
	if b := SceneBlock(list[0]); !strings.Contains(b, `name: "The Eye Opens"`) || !strings.Contains(b, `color: "#3dff7a"`) || strings.Contains(b, "look:") {
		t.Errorf("block %s", b)
	}
	looked := list[0]
	looked.Background.Params.Look, looked.Overlays[0].Params.Look = "eyelids", "rift"
	b := SceneBlock(looked)
	if !strings.Contains(b, "background: {kind: fall-asleep, seconds: 7, look: eyelids}") || !strings.Contains(b, `color: "#3dff7a", look: rift}`) {
		t.Errorf("block with looks %s", b)
	}
}

// The prompt's description names every kind the server accepts, so an AI is
// never told less than the editor offers.
func TestAIDocNamesEveryKind(t *testing.T) {
	_, ai := newAIRig(true)
	doc := ai.Kinds()[0].Doc()
	for _, list := range [][]Kind{BackgroundKinds, OverlayKinds, TextStyles, EndingKinds} {
		for _, k := range list {
			if !strings.Contains(doc, k.Key) {
				t.Errorf("Doc lacks %q", k.Key)
			}
		}
	}
	for kind, list := range Looks {
		for _, l := range list {
			if !strings.Contains(doc, "`"+l.Key+"` ("+l.Name+")") {
				t.Errorf("Doc lacks the %s look %q", kind, l.Key)
			}
		}
	}
	if !strings.Contains(doc, "look: drift") || !strings.Contains(doc, "look: gather") {
		t.Error("Doc's example does not show a look")
	}
	if !strings.Contains(doc, "never shows a scene") {
		t.Error("Doc does not say the AI never shows a scene")
	}
}

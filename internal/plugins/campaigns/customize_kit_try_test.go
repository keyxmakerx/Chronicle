package campaigns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// Keep this kit writes the kit into the draft's look section and keeps
// everything else the owner left unsaved.
func TestKeepKitInDraft(t *testing.T) {
	saved := func() any {
		return map[string]any{"look": "parchment", "page": map[string]any{"look": "annals", "kit": "material", "dmKit": "same"}}
	}
	tests := []struct {
		name    string
		draft   string
		want    []string // paths that must read as given, "section.part.page.key=value"
		keep    []string // raw fragments that must survive
		noInput bool
		wantErr bool
	}{
		{
			name:    "no draft starts the look from what is saved",
			draft:   `{}`,
			want:    []string{"draft.page.kit=ornate", "draft.page.look=annals", "draft.page.dmKit=same", "draft.look=parchment"},
			noInput: true,
		},
		{
			name:  "a held look keeps its other changes and what Save applies",
			draft: `{"look":{"draft":{"look":"midnight","page":{"look":"standard","kit":"lumen"},"colours":{"accent":"#123456"}},"input":{"look":"midnight","page":{"look":"standard","kit":"lumen","dmKit":"same"}},"_changes":[{"label":"Look: Pages"}]},"homes":{"x":1}}`,
			want:  []string{"draft.page.kit=ornate", "draft.page.look=annals", "draft.look=midnight", "input.page.kit=ornate", "input.page.look=annals", "input.page.dmKit=same"},
			keep:  []string{`"homes":{"x":1}`, `"accent":"#123456"`, `"_changes":[{"label":"Look: Pages"}]`},
		},
		{
			name:    "a look that is not an object is refused",
			draft:   `{"look":{"draft":"nonsense"}}`,
			wantErr: true,
		},
		{
			name:    "a draft that is not an object is refused",
			draft:   `[]`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := keepKitInDraft(json.RawMessage(tc.draft), "ornate", saved)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %s", out)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var top map[string]map[string]any
			if err := json.Unmarshal(out, &top); err != nil {
				t.Fatalf("not an object of sections: %s", out)
			}
			look := top["look"]
			for _, w := range tc.want {
				path, val, _ := strings.Cut(w, "=")
				var cur any = look
				for _, k := range strings.Split(path, ".") {
					m, _ := cur.(map[string]any)
					cur = m[k]
				}
				if cur != val {
					t.Errorf("%s = %v, want %s", path, cur, val)
				}
			}
			if _, ok := look["input"]; ok == tc.noInput {
				t.Errorf("input present = %v, want %v", ok, !tc.noInput)
			}
			for _, k := range tc.keep {
				if !strings.Contains(string(out), k) {
					t.Errorf("lost %s: %s", k, out)
				}
			}
		})
	}
}

// The strip's kits are the ones Customize offers, and the tokens it is sent
// draw each of them.
func TestKitTryCoversStyleKits(t *testing.T) {
	ctx := layouts.SetAppearance(context.Background(), &layouts.AppearanceData{PageLook: "annals"})
	css := layouts.KitTryCSS(ctx)
	for _, id := range AppearanceStyleKits {
		if !strings.Contains(css, `[data-kit-try="`+id+`"]`) {
			t.Errorf("no tokens for %s", id)
		}
		if LookChoiceLabel(id) == id {
			t.Errorf("%s has no label", id)
		}
	}
}

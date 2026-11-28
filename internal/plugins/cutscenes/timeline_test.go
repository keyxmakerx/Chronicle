package cutscenes

import (
	"bytes"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v3"
)

// renderEditor draws a scene's editor form as the page serves it.
func renderEditor(t *testing.T, s Scene) *goquery.Document {
	t.Helper()
	var b bytes.Buffer
	if err := editorForm(editorPage{CampaignID: "camp-1", Scene: s}).Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(&b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// submitted is what the browser would post for the form as drawn: every
// named field outside the add-a-row templates, the chosen radio, and each
// select's chosen option (its first when none is marked).
func submitted(doc *goquery.Document) url.Values {
	v := url.Values{}
	doc.Find("input[name], select[name]").Each(func(_ int, f *goquery.Selection) {
		if f.Closest("template").Length() > 0 {
			return
		}
		name, _ := f.Attr("name")
		if goquery.NodeName(f) == "select" {
			opt := f.Find("option[selected]").First()
			if opt.Length() == 0 {
				opt = f.Find("option").First()
			}
			val, _ := opt.Attr("value")
			v.Add(name, val)
			return
		}
		if typ, _ := f.Attr("type"); typ == "radio" || typ == "checkbox" {
			if _, on := f.Attr("checked"); !on {
				return
			}
		}
		val, _ := f.Attr("value")
		v.Add(name, val)
	})
	return v
}

// The timeline is drawn from the rows' own fields (cutscene_timeline.js
// reads them by these names), so the editor must keep serving every one.
func TestEditorHasWhatTheTimelineReads(t *testing.T) {
	doc := renderEditor(t, Scene{Name: "One", Background: Background{Kind: BackgroundFadeToBlack, Seconds: 2},
		Overlays: []Overlay{{Kind: OverlayLetterbox, Seconds: 1}}, Text: []TextLine{{Say: "Hi", Seconds: 3, Style: StylePlain}},
		Moments: []Moment{{At: 4, Ask: "Why?", Ways: []Way{{Kind: WayWrite}}}}})
	for _, sel := range []string{
		"[data-csw-timeline][hidden] [data-csw-tl-lanes]", "[data-csw-timeline] [data-csw-tl-axis]", "[data-csw-timeline] [data-csw-tl-empty]",
		"select#csw-bg-kind", "select#csw-bg-in", "input#csw-bg-sec",
		"#csw-ov-list [data-csw-row] select[name=ov_kind]", "#csw-ov-list [data-csw-row] input[name=ov_at]", "#csw-ov-list [data-csw-row] input[name=ov_seconds]",
		"#csw-tx-list [data-csw-row] input[name=tx_say]", "#csw-tx-list [data-csw-row] input[name=tx_at]", "#csw-tx-list [data-csw-row] input[name=tx_seconds]",
		"#csw-tx-list [data-csw-row] select[name=tx_in]", "#csw-tx-list [data-csw-row] select[name=tx_out]",
		"#csw-mo-list [data-csw-row] input[name=mo_ask]", "#csw-mo-list [data-csw-row] input[name=mo_at]",
	} {
		if doc.Find(sel).Length() == 0 {
			t.Errorf("the editor has no %s", sel)
		}
	}
	// The timeline's tags are keyed by the choices' values.
	for _, sel := range []string{"select[name=tx_in] option[value='']", "select[name=tx_out] option[value='']", "select[name=bg_in] option[value=fall-asleep]"} {
		if doc.Find(sel).Length() == 0 {
			t.Errorf("the editor has no %s", sel)
		}
	}
}

// A scene an AI writes is the timeline the Director then sees: stored, drawn
// into the editor and posted back with nothing touched, it is the same
// scene, and the block the AI is shown for it writes the same scene again.
func TestAITimelineLoadsIntoEditorUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
	}{
		{name: "title card", fields: map[string]any{
			"background": map[any]any{"kind": "black", "in": "fade", "seconds": 1.5},
			"overlays":   []any{map[any]any{"kind": "letterbox", "at": 0, "seconds": 1}},
			"text": []any{
				map[any]any{"say": "Chapter two", "at": 1, "seconds": 4, "style": "caption", "in": "rise", "out": "fade"},
				map[any]any{"say": "The Drowned Bell", "at": 2, "seconds": 4, "style": "title", "in": "settle"},
			},
			"ending": map[any]any{"kind": "auto", "after": 2},
		}},
		{name: "place card", fields: map[string]any{
			"background": map[any]any{"kind": "none"},
			"text": []any{
				map[any]any{"say": "Saltreach", "at": 0.5, "seconds": 0, "style": "title", "layout": "corner", "in": "slide"},
				map[any]any{"say": "Three days later", "at": 0.9, "seconds": 3.5, "style": "whisper", "layout": "corner", "in": "slide", "out": "slide"},
			},
		}},
		{name: "falling asleep with a look", fields: map[string]any{
			"background": map[any]any{"kind": "black", "in": "fall-asleep", "seconds": 7, "look": "drift"},
			"overlays":   []any{map[any]any{"kind": "eye", "at": 7.5, "seconds": 3.5, "color": "#3dff7a", "look": "almond", "pupil": "round"}},
			"text":       []any{map[any]any{"say": "Something has noticed you.", "at": 11, "seconds": 5, "style": "whisper"}},
		}},
		{name: "moments", fields: map[string]any{
			"background": map[any]any{"kind": "black", "in": "fade", "seconds": 2},
			"text":       []any{map[any]any{"say": "Saltreach", "at": 0.5, "seconds": 0, "style": "title"}},
			"moments": []any{
				map[any]any{"at": 4, "ask": "What do the fishermen whisper about the bell?", "ways": []any{"director", map[any]any{"kind": "write"}}, "to": "one", "note": "The bell rang the night the tide didn't turn."},
				map[any]any{"at": 9.5, "ask": "Who rings it now?"},
			},
		}},
		{name: "the older single kind", fields: map[string]any{
			"background": map[any]any{"kind": "fade-to-black", "seconds": 3},
			"overlays":   []any{map[any]any{"kind": "embers", "at": 2, "seconds": 2}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ai := newAIRig(true)
			k := ai.Kinds()[0]
			if err := k.Apply(ctx, "camp-1", aDirector, rec("create", tt.name, tt.fields)); err != nil {
				t.Fatal(err)
			}
			list, _ := r.svc.List(ctx, "camp-1", vDir)
			stored := list[0]
			if ms, ok := tt.fields["moments"].([]any); ok && len(stored.Moments) != len(ms) {
				t.Fatalf("stored %d moments, want %d", len(stored.Moments), len(ms))
			}

			back, err := SceneFromForm(submitted(renderEditor(t, stored)))
			if err != nil {
				t.Fatal(err)
			}
			back, err = Normalize(back)
			if err != nil {
				t.Fatal(err)
			}
			want := stored
			want.ID, want.UpdatedAt = "", back.UpdatedAt
			if !reflect.DeepEqual(back, want) {
				t.Errorf("posted back from the editor\n got %+v\nwant %+v", back, want)
			}

			// The block the AI reads for the scene, written back as an
			// update, changes nothing.
			block := SceneBlock(stored)
			if BackdropOf(stored.Background.Kind) != BackgroundNone && !strings.Contains(block, ", in: "+ArrivalOf(stored.Background.Kind)) {
				t.Errorf("the block does not say how the background arrives: %s", block)
			}
			fields := blockFields(t, block)
			if err := k.Apply(ctx, "camp-1", aDirector, rec("update", tt.name, fields)); err != nil {
				t.Fatal(err)
			}
			list, _ = r.svc.List(ctx, "camp-1", vDir)
			again := list[0]
			again.UpdatedAt = stored.UpdatedAt
			if !reflect.DeepEqual(again, stored) {
				t.Errorf("the AI's own block changed the scene\n got %+v\nwant %+v", again, stored)
			}
		})
	}
}

// blockFields reads a cut-scene block's front matter the way a change
// request arrives: YAML, every key but the ones rec sets itself.
func blockFields(t *testing.T, block string) map[string]any {
	t.Helper()
	var f map[string]any
	if err := yaml.Unmarshal([]byte(block), &f); err != nil {
		t.Fatalf("block is not YAML: %v\n%s", err, block)
	}
	delete(f, "kind")
	delete(f, "name")
	return f
}

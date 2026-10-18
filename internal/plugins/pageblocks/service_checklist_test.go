package pageblocks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func putList(svc Service, v Viewer, doc string, version int) (*View, error) {
	return svc.Put(context.Background(), camp, page, blk, v, SaveInput{Kind: KindChecklist, Data: json.RawMessage(doc), Version: ver(version)})
}

func listOf(t *testing.T, svc Service, v Viewer) (Checklist, *View) {
	t.Helper()
	out, err := svc.Get(context.Background(), camp, page, blk, KindChecklist, v)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	d, ok := out.Data.(Checklist)
	if !ok {
		t.Fatalf("data is %T", out.Data)
	}
	return d, out
}

// hearts is the campaign AI's own case: items each held by the party or the
// core, one hidden, one linked to a page players see and one to a page they
// don't, and a threshold for the DM.
const hearts = `{"title":"The hidden god's hearts","sideA":"Party","sideB":"The core","dmOnly":false,
"items":[{"id":"i1","name":"The bishop's chest","entityId":"keep","holder":"a","dm":false},
         {"id":"i2","name":"The banner","holder":"b","dm":false},
         {"id":"i3","name":"The unfinished room","holder":"","dm":true},
         {"id":"i4","name":"The treasury's real loss","entityId":"vault","holder":"b","dm":false}],
"thresholds":[{"id":"t1","side":"a","count":4,"note":"The third ending opens."}]}`

func TestChecklistGetEmpty(t *testing.T) {
	svc, _ := pageMapEnv()
	d, v := listOf(t, svc, player)
	if v.Version != 0 || v.Kind != KindChecklist || v.CanEdit {
		t.Fatalf("view = %+v", v)
	}
	if d.SideA != defaultSideA || d.SideB != defaultSideB || d.Items == nil || d.Thresholds == nil {
		t.Fatalf("empty checklist = %+v (lists must be non-nil so the JSON has [])", d)
	}
}

func TestChecklistViewerFiltering(t *testing.T) {
	svc, _ := pageMapEnv()
	if _, err := putList(svc, dm, hearts, 0); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		v          Viewer
		wantItems  []string
		wantThresh int
		wantEdit   bool
	}{
		{"owner sees everything", dm, []string{"i1", "i2", "i3", "i4"}, 1, true},
		{"co-DM sees everything", coDM, []string{"i1", "i2", "i3", "i4"}, 1, true},
		{"scribe loses the DM item and the threshold, so cannot edit", scribe, []string{"i1", "i2", "i4"}, 0, false},
		{"player", player, []string{"i1", "i2", "i4"}, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, v := listOf(t, svc, tc.v)
			var ids []string
			for _, it := range d.Items {
				ids = append(ids, it.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantItems, ",") {
				t.Fatalf("items = %v, want %v", ids, tc.wantItems)
			}
			if len(d.Thresholds) != tc.wantThresh || v.CanEdit != tc.wantEdit {
				t.Fatalf("thresholds %d canEdit %v", len(d.Thresholds), v.CanEdit)
			}
			for _, it := range d.Items {
				switch it.ID {
				case "i1":
					if it.EntityName != "The Keep" || it.EntityURL == "" {
						t.Fatalf("a visible link gets its name and path: %+v", it)
					}
				case "i4":
					wantLink := tc.v.VisibilityRole >= 3
					if (it.EntityID != "") != wantLink {
						t.Fatalf("link to a hidden page shown = %v, want %v", it.EntityID != "", wantLink)
					}
				}
			}
		})
	}
}

func TestChecklistDMOnlyBlockReadsEmptyForPlayers(t *testing.T) {
	svc, _ := pageMapEnv()
	doc := `{"title":"Ink","sideA":"Party","sideB":"Core","dmOnly":true,"items":[{"id":"i1","name":"A spot","holder":"a","dm":false}],"thresholds":[]}`
	if _, err := putList(svc, dm, doc, 0); err != nil {
		t.Fatal(err)
	}
	d, v := listOf(t, svc, player)
	if d.Title != "" || d.SideA != "" || len(d.Items) != 0 || v.CanEdit {
		t.Fatalf("player saw a DM-only checklist: %+v", d)
	}
	if d, _ := listOf(t, svc, dm); d.Title != "Ink" || len(d.Items) != 1 {
		t.Fatalf("owner lost the checklist: %+v", d)
	}
}

func TestChecklistPutRules(t *testing.T) {
	plain := `{"title":"Hearts","sideA":"Party","sideB":"Core","dmOnly":false,"items":[{"id":"i1","name":"Banner","holder":"a","dm":false}],"thresholds":[]}`
	tests := []struct {
		name  string
		first string // saved by the owner first, if set
		v     Viewer
		doc   string
		want  int
	}{
		{"scribe saves a plain checklist", "", scribe, plain, 0},
		{"player may not save", "", player, plain, 403},
		{"scribe may not hide the block", "", scribe, strings.Replace(plain, `"dmOnly":false`, `"dmOnly":true`, 1), 403},
		{"scribe may not add a DM item", "", scribe, strings.Replace(plain, `"dm":false`, `"dm":true`, 1), 403},
		{"scribe may not add a threshold", "", scribe, strings.Replace(plain, `"thresholds":[]`, `"thresholds":[{"id":"t1","side":"a","count":1,"note":""}]`, 1), 403},
		{"scribe may not overwrite DM parts they never saw", hearts, scribe, plain, 403},
		{"scribe may not link to a page they can't open", "", scribe, strings.Replace(plain, `"holder":"a"`, `"entityId":"vault","holder":"a"`, 1), 422},
		{"owner may link to a hidden page", "", dm, strings.Replace(plain, `"holder":"a"`, `"entityId":"vault","holder":"a"`, 1), 0},
		{"a link to another campaign's page is refused", "", dm, strings.Replace(plain, `"holder":"a"`, `"entityId":"far","holder":"a"`, 1), 422},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := pageMapEnv()
			version := 0
			if tc.first != "" {
				if _, err := putList(svc, dm, tc.first, 0); err != nil {
					t.Fatal(err)
				}
				version = 1
			}
			_, err := putList(svc, tc.v, tc.doc, version)
			if code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
		})
	}
}

func TestChecklistLinkToGonePage(t *testing.T) {
	svc, _, ents := pageMapEnvEnts()
	doc := `{"title":"","sideA":"A","sideB":"B","dmOnly":false,"items":[{"id":"i1","name":"x","entityId":"keep","holder":"","dm":false}],"thresholds":[]}`
	if _, err := putList(svc, scribe, doc, 0); err != nil {
		t.Fatal(err)
	}
	delete(ents.pages, "keep")
	d, v := listOf(t, svc, scribe)
	if d.Items[0].EntityID != "" || !v.CanEdit {
		t.Fatalf("a link to a deleted page is dropped and locks nothing: %+v canEdit %v", d.Items[0], v.CanEdit)
	}
}

func TestValidateChecklist(t *testing.T) {
	item := func(extra string) string {
		return `{"title":"","sideA":"A","sideB":"B","dmOnly":false,"items":[` + extra + `],"thresholds":[]}`
	}
	tests := []struct {
		name string
		doc  string
		ok   bool
	}{
		{"empty", item(``), true},
		{"bad holder", item(`{"id":"i1","name":"x","holder":"c","dm":false}`), false},
		{"duplicate ids", item(`{"id":"i1","name":"x","holder":"","dm":false},{"id":"i1","name":"y","holder":"","dm":false}`), false},
		{"id with markup", item(`{"id":"<b>","name":"x","holder":"","dm":false}`), false},
		{"control character in name", item(`{"id":"i1","name":"x\u0007","holder":"","dm":false}`), false},
		{"name too long", item(`{"id":"i1","name":"` + strings.Repeat("x", MaxChecklistName+1) + `","holder":"","dm":false}`), false},
		{"unknown field", item(`{"id":"i1","name":"x","holder":"","dm":false,"colour":"red"}`), false},
		{"bad link id", item(`{"id":"i1","name":"x","entityId":"../x","holder":"","dm":false}`), false},
		{"threshold side", `{"title":"","sideA":"A","sideB":"B","dmOnly":false,"items":[],"thresholds":[{"id":"t","side":"c","count":1,"note":""}]}`, false},
		{"threshold count zero", `{"title":"","sideA":"A","sideB":"B","dmOnly":false,"items":[],"thresholds":[{"id":"t","side":"a","count":0,"note":""}]}`, false},
		{"threshold id clashes with an item", `{"title":"","sideA":"A","sideB":"B","dmOnly":false,"items":[{"id":"x","name":"x","holder":"","dm":false}],"thresholds":[{"id":"x","side":"a","count":1,"note":""}]}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateChecklist(json.RawMessage(tc.doc))
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
		})
	}
	many := make([]string, MaxChecklistItems+1)
	for i := range many {
		many[i] = `{"id":"i` + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `","name":"x","holder":"","dm":false}`
	}
	if _, err := validateChecklist(json.RawMessage(item(strings.Join(many, ",")))); err == nil {
		t.Fatal("more than the item limit must be refused")
	}
}

// The page's name and path are added on read; echoed back on a save they
// must not be stored.
func TestChecklistDropsEchoedLinkNames(t *testing.T) {
	d, err := validateChecklist(json.RawMessage(`{"title":"","sideA":"A","sideB":"B","dmOnly":false,"items":[{"id":"i1","name":"x","entityId":"keep","entityName":"Fake","entityUrl":"javascript:x","holder":"","dm":false}],"thresholds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Items[0].EntityName != "" || d.Items[0].EntityURL != "" {
		t.Fatalf("stored read-only fields: %+v", d.Items[0])
	}
}

func TestReachedThresholds(t *testing.T) {
	d := Checklist{
		Items: []ChecklistItem{{ID: "1", Holder: SideA}, {ID: "2", Holder: SideA}, {ID: "3", Holder: SideB}, {ID: "4"}},
		Thresholds: []Threshold{
			{ID: "a2", Side: SideA, Count: 2},
			{ID: "a3", Side: SideA, Count: 3},
			{ID: "b1", Side: SideB, Count: 1},
		},
	}
	var got []string
	for _, th := range ReachedThresholds(d) {
		got = append(got, th.ID)
	}
	if strings.Join(got, ",") != "a2,b1" {
		t.Fatalf("reached = %v, want a2,b1", got)
	}
	if a, b := HeldCounts(d); a != 2 || b != 1 {
		t.Fatalf("counts = %d,%d", a, b)
	}
}

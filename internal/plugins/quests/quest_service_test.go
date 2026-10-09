package quests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	camp  = "camp-1"
	other = "camp-2"
	qid   = "quest-1"
)

var (
	dm     = Viewer{UserID: "dm", MemberRole: 3, VisibilityRole: 3, IsDM: true}
	scribe = Viewer{UserID: "scr", MemberRole: 2, VisibilityRole: 2}
	player = Viewer{UserID: "pl", MemberRole: 1, VisibilityRole: 1}
	guest  = Viewer{}
)

type questEnv struct {
	svc  QuestService
	repo *fakeQuestRepo
	ents *fakeEntities
	maps *fakeMaps
}

func newQuestEnv() questEnv {
	e := questEnv{repo: newFakeQuestRepo(), ents: newFakeEntities(), maps: newFakeMaps()}
	e.ents.add(camp, qid, "Missing Goat", false)
	e.ents.add(camp, "secret-quest", "Secret", true)
	e.ents.add(camp, "boss", "Boss", false)
	e.ents.add(other, "foreign", "Foreign", false)
	e.maps.add(camp, "map-1", "Vale")
	e.maps.add(other, "map-x", "Foreign map")
	e.svc = NewQuestService(e.repo, e.ents, e.maps)
	return e
}

func code(err error) int {
	if err == nil {
		return 0
	}
	if ae, ok := err.(*apperror.AppError); ok {
		return ae.Code
	}
	return -1
}

func parsePatch(t *testing.T, body string) QuestPatch {
	t.Helper()
	var p QuestPatch
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestQuestPutAndPlayerFiltering(t *testing.T) {
	e := newQuestEnv()
	ctx := context.Background()
	_, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,
	  "notice":{"title":"Goat!","reward":"50 gp","blurb":"b","plate":"Wanted"},
	  "status":"active","handedOut":true,"mapId":"map-1",
	  "steps":[{"id":"a","text":"seen","done":true,"shown":true},{"id":"b","text":"SECRET STEP","shown":false}],
	  "rewards":[{"kind":"money","text":"gold","amount":50,"entityId":"boss"}],
	  "foes":[{"text":"Goblin","note":"n","entityId":"boss"}],
	  "links":[{"kind":"map","refId":"map-1","label":"l"}]}`))
	if err != nil {
		t.Fatal(err)
	}

	gotDM, err := e.svc.Get(ctx, camp, qid, dm)
	if err != nil {
		t.Fatal(err)
	}
	dv := gotDM.(*DMQuestView)
	if !dv.CanEdit || dv.Version != 1 || len(dv.Rewards) != 1 || dv.Rewards[0].Name != "Boss" || dv.Links[0].Name != "Vale" || dv.MapName != "Vale" {
		t.Fatalf("dm view wrong: %+v", dv)
	}

	for _, who := range []Viewer{scribe, player, guest} {
		got, err := e.svc.Get(ctx, camp, qid, who)
		if err != nil {
			t.Fatal(err)
		}
		pv, ok := got.(*PlayerQuestView)
		if !ok {
			t.Fatalf("non-DM got %T", got)
		}
		if pv.CanEdit || len(pv.Steps) != 1 || pv.Steps[0].Text != "seen" || !pv.HiddenSteps || !pv.ShowTag || pv.Map == nil || pv.Map.ID != "map-1" {
			t.Fatalf("player view wrong: %+v", pv)
		}
		raw, _ := json.Marshal(pv)
		for _, leak := range []string{"SECRET STEP", "rewards", "foes", "links", "Goblin", `"id":"a"`, "version"} {
			if strings.Contains(string(raw), leak) {
				t.Fatalf("player JSON leaks %q: %s", leak, raw)
			}
		}
	}
}

func TestQuestPlayerHiddenPieces(t *testing.T) {
	e := newQuestEnv()
	ctx := context.Background()
	_, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"notice":{"title":"T","reward":"r"},"mapId":"map-1",
	  "layout":{"notice":{"x":1,"y":2,"w":30,"r":0,"hidden":true},"map":{"x":1,"y":2,"w":30,"r":0,"hidden":true},"tag":{"x":1,"y":2,"w":30,"r":0,"hidden":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, camp, qid, player)
	pv := got.(*PlayerQuestView)
	if pv.Notice != nil || pv.Map != nil || pv.ShowTag {
		t.Fatalf("hidden pieces leaked: %+v", pv)
	}
}

func TestQuestGetVisibilityAndScope(t *testing.T) {
	e := newQuestEnv()
	tests := []struct {
		name string
		eid  string
		v    Viewer
		want int
	}{
		{"visible to player", qid, player, 0},
		{"hidden page 404 for player", "secret-quest", player, 404},
		{"hidden page 404 for guest", "secret-quest", guest, 404},
		{"hidden page ok for DM", "secret-quest", dm, 0},
		{"foreign campaign page 404 even for DM", "foreign", dm, 404},
		{"unknown page 404", "nope", dm, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.svc.Get(context.Background(), camp, tt.eid, tt.v)
			if code(err) != tt.want {
				t.Fatalf("got %v want %d", err, tt.want)
			}
		})
	}
}

func TestQuestPutPermissionsAndValidation(t *testing.T) {
	long := strings.Repeat("x", 121)
	tests := []struct {
		name string
		v    Viewer
		body string
		want int
	}{
		{"scribe forbidden", scribe, `{"version":0,"status":"active"}`, 403},
		{"player forbidden", player, `{"version":0,"status":"active"}`, 403},
		{"guest forbidden", guest, `{"version":0,"status":"active"}`, 403},
		{"missing version", dm, `{"status":"active"}`, 422},
		{"bad status", dm, `{"version":0,"status":"nope"}`, 422},
		{"title too long", dm, `{"version":0,"notice":{"title":"` + long + `"}}`, 422},
		{"too many paragraphs", dm, `{"version":0,"notice":{"body":["a","a","a","a","a","a","a","a","a","a","a"]}}`, 422},
		{"bad look", dm, `{"version":0,"looks":{"board":"neon"}}`, 422},
		{"bad id", dm, `{"version":0,"steps":[{"id":"bad id!","text":"x"}]}`, 422},
		{"duplicate ids", dm, `{"version":0,"steps":[{"id":"a","text":"x"},{"id":"a","text":"y"}]}`, 422},
		{"foreign entity ref", dm, `{"version":0,"foes":[{"text":"x","entityId":"foreign"}]}`, 422},
		{"foreign map", dm, `{"version":0,"mapId":"map-x"}`, 422},
		{"foreign link map", dm, `{"version":0,"links":[{"kind":"map","refId":"map-x","label":"l"}]}`, 422},
		{"bad reward kind", dm, `{"version":0,"rewards":[{"kind":"gem","text":"x"}]}`, 422},
		{"ok clamps layout", dm, `{"version":0,"layout":{"notice":{"x":500,"y":-4,"w":1,"r":99}}}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newQuestEnv()
			_, err := e.svc.Put(context.Background(), camp, qid, tt.v, parsePatch(t, tt.body))
			if code(err) != tt.want {
				t.Fatalf("got %v want %d", err, tt.want)
			}
		})
	}
	t.Run("too many list items", func(t *testing.T) {
		e := newQuestEnv()
		steps := make([]Step, 51)
		raw, _ := json.Marshal(map[string]any{"version": 0, "steps": steps})
		_, err := e.svc.Put(context.Background(), camp, qid, dm, parsePatch(t, string(raw)))
		if code(err) != 422 {
			t.Fatalf("got %v", err)
		}
	})
}

func TestQuestLayoutClamp(t *testing.T) {
	e := newQuestEnv()
	v, err := e.svc.Put(context.Background(), camp, qid, dm, parsePatch(t, `{"version":0,"layout":{"notice":{"x":500,"y":-4,"w":1,"r":99}}}`))
	if err != nil {
		t.Fatal(err)
	}
	n := v.Layout.Notice
	if n.X != 100 || n.Y != 0 || n.W != 5 || n.R != 15 {
		t.Fatalf("not clamped: %+v", n)
	}
	if v.Layout.Map != defaultLayout().Map {
		t.Fatalf("absent layout piece must keep its default: %+v", v.Layout.Map)
	}
}

func TestQuestPartialUpdate(t *testing.T) {
	e := newQuestEnv()
	ctx := context.Background()
	if _, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"status":"active","handedOut":true,"mapId":"map-1",
	  "notice":{"title":"Keep","reward":"R"},"steps":[{"id":"a","text":"s","shown":true}],"looks":{"board":"midnight","ledger":"plain"}}`)); err != nil {
		t.Fatal(err)
	}
	// Absent keys preserve; a present key replaces; explicit null on a
	// not-null field preserves; null on mapId clears.
	v, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":1,"status":"done","steps":null,"looks":{"ledger":"lit"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "done" || v.Notice.Title != "Keep" || !v.HandedOut || len(v.Steps) != 1 || v.MapID != "map-1" ||
		v.Looks.Board != "midnight" || v.Looks.Ledger != "lit" {
		t.Fatalf("partial update clobbered fields: %+v", v)
	}
	v, err = e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":2,"mapId":null}`))
	if err != nil || v.MapID != "" || v.Status != "done" {
		t.Fatalf("null mapId: %v %+v", err, v)
	}
	// Absent handedOut preserved, explicit false replaces.
	v, _ = e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":3,"handedOut":false}`))
	if v.HandedOut {
		t.Fatal("explicit false must replace")
	}
}

func TestQuestVersionConflict(t *testing.T) {
	e := newQuestEnv()
	ctx := context.Background()
	if _, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"status":"active"}`)); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		version string
		want    int
	}{
		{"stale version", "0", http.StatusConflict},
		{"future version", "7", http.StatusConflict},
		{"current version", "1", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":`+tt.version+`,"status":"done"}`))
			if code(err) != tt.want {
				t.Fatalf("got %v want %d", err, tt.want)
			}
		})
	}
}

func TestQuestPutIDOR(t *testing.T) {
	e := newQuestEnv()
	_, err := e.svc.Put(context.Background(), camp, "foreign", dm, parsePatch(t, `{"version":0,"status":"active"}`))
	if code(err) != 404 {
		t.Fatalf("foreign page must 404, got %v", err)
	}
}

func TestQuestHiddenTagHidesReward(t *testing.T) {
	e := newQuestEnv()
	ctx := context.Background()
	_, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"notice":{"title":"T","reward":"500 gp"},
	  "layout":{"tag":{"x":1,"y":2,"w":30,"r":0,"hidden":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, camp, qid, player)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "500 gp") {
		t.Fatalf("player JSON shows a hidden reward: %s", raw)
	}
	if got.(*PlayerQuestView).Notice == nil {
		t.Fatal("hiding the tag must not hide the notice")
	}
	dmv, _ := e.svc.Get(ctx, camp, qid, dm)
	if raw, _ = json.Marshal(dmv); !strings.Contains(string(raw), "500 gp") {
		t.Fatalf("DM lost the reward: %s", raw)
	}
}

func TestQuestDeletedRefDoesNotBlockEdits(t *testing.T) {
	e := newQuestEnv()
	ctx := context.Background()
	if _, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"foes":[{"id":"a","text":"Boss","entityId":"boss"}]}`)); err != nil {
		t.Fatal(err)
	}
	delete(e.ents.byID, "boss")
	if _, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":1,"foes":[{"id":"a","text":"Boss","entityId":"boss"},{"id":"b","text":"Rats"}]}`)); err != nil {
		t.Fatalf("an edit next to a deleted page was refused: %v", err)
	}
	// A new id is still checked.
	if _, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":2,"foes":[{"id":"c","text":"F","entityId":"foreign"}]}`)); code(err) != 422 {
		t.Fatalf("foreign page accepted: %v", err)
	}
}

// With the maps addon off, a quest shows no map and cannot link one.
func TestQuestMapsAddonOff(t *testing.T) {
	ctx := context.Background()
	e := newQuestEnv()
	if _, err := e.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"mapId":"map-1","links":[{"kind":"map","refId":"map-1"}]}`)); err != nil {
		t.Fatal(err)
	}
	e.maps.off = true
	got, err := e.svc.Get(ctx, camp, qid, dm)
	if err != nil {
		t.Fatal(err)
	}
	dv := got.(*DMQuestView)
	if dv.MapsOn || dv.MapName != "" || dv.Links[0].Name != "" {
		t.Fatalf("dm view with addon off: %+v", dv)
	}
	// A link saved earlier is kept (other edits still save); a new one is refused.
	e2 := newQuestEnv()
	e2.maps.off = true
	if _, err := e2.svc.Put(ctx, camp, qid, dm, parsePatch(t, `{"version":0,"links":[{"kind":"map","refId":"map-1"}]}`)); code(err) != 422 {
		t.Fatalf("new map link with addon off: %v", err)
	}
}

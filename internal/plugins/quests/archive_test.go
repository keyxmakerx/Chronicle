package quests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestArchiveRoundTrip(t *testing.T) {
	src := newQuestEnv()
	ctx := context.Background()
	patchBody := `{"version":0,"status":"active","steps":[{"id":"s1","text":"Find the goat","done":false,"shown":true}],
		"foes":[{"id":"f1","text":"Boss","note":"","entityId":"boss"}],
		"links":[{"id":"l1","kind":"map","refId":"map-1","label":"Vale"}],"mapId":"map-1"}`
	var p QuestPatch
	if err := json.Unmarshal([]byte(patchBody), &p); err != nil {
		t.Fatal(err)
	}
	if _, err := src.svc.Put(ctx, camp, qid, dm, p); err != nil {
		t.Fatal(err)
	}

	rows, err := NewArchive(src.repo, src.ents, src.maps, nil).Export(ctx, camp)
	if err != nil || len(rows) != 1 || rows[0].Name != "Missing Goat" {
		t.Fatalf("export = %+v, %v", rows, err)
	}

	// The same sheet restored into a new campaign whose ids all differ, with the
	// boss page not restored.
	dst := newQuestEnv()
	dst.ents.add("camp-new", "new-quest", "Missing Goat", false)
	dst.maps.add("camp-new", "new-map", "Vale")
	arch := NewArchive(dst.repo, dst.ents, dst.maps, nil)
	remap := Remap{
		Entity: func(id string) (string, bool) { return "new-quest", id == qid },
		Map:    func(id string) (string, bool) { return "new-map", id == "map-1" },
		Event:  func(string) (string, bool) { return "", false },
	}
	left, err := arch.Import(ctx, "camp-new", "new-quest", "u", rows[0].Data, remap)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || !strings.Contains(left[0], "foe") {
		t.Fatalf("left = %v", left)
	}
	data, _, found, _ := dst.repo.Get(ctx, "camp-new", "new-quest")
	if !found {
		t.Fatal("sheet not stored")
	}
	var got Quest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusActive || len(got.Steps) != 1 || got.MapID != "new-map" ||
		len(got.Links) != 1 || got.Links[0].RefID != "new-map" || got.Foes[0].EntityID != "" {
		t.Fatalf("restored sheet = %+v", got)
	}

	// A page that is not in the campaign is refused, and a second sheet for the
	// same page is refused rather than overwriting.
	if _, err := arch.Import(ctx, "camp-new", "nope", "u", rows[0].Data, remap); err == nil {
		t.Fatal("unknown page accepted")
	}
	if _, err := arch.Import(ctx, "camp-new", "new-quest", "u", rows[0].Data, remap); err == nil {
		t.Fatal("second sheet overwrote the first")
	}
}

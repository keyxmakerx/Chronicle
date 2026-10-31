package armory

import (
	"context"
	"strings"
	"testing"
)

type fakePlaces map[string]map[string]Place

func (f fakePlaces) List(_ context.Context, _, characterID string) (map[string]Place, error) {
	out := map[string]Place{}
	for k, v := range f[characterID] {
		out[k] = v
	}
	return out, nil
}

func (f fakePlaces) Set(_ context.Context, _, characterID, itemID string, p Place) error {
	if f[characterID] == nil {
		f[characterID] = map[string]Place{}
	}
	if p == PlacePack {
		delete(f[characterID], itemID)
		return nil
	}
	f[characterID][itemID] = p
	return nil
}

func placesFx() (*shareFx, fakePlaces) {
	f := newShareFx()
	pl := fakePlaces{}
	f.svc = NewStashService(StashDeps{
		Repo: f.repo, Directory: f.dir, Visibility: f.vis, Actor: fakeActor{f.dir.fakeDir},
		Fields: f.fields, Relations: f.rels, Events: f.events, Handouts: f.hand, Notifier: f.notify,
		UserNames: fakeNames{"gm": "Greta", "u1": "Tess", "u2": "Robin", "u3": "Lee"},
		Shares:    f.shares, Auditor: f.audit, Places: pl,
	})
	return f, pl
}

func TestParsePlace(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{{"hand", true}, {"worn", true}, {"belt", true}, {"pack", true}, {"", false}, {"Hand", false}, {"stash", false}} {
		if _, ok := ParsePlace(tc.in); ok != tc.ok {
			t.Errorf("%q ok=%v", tc.in, ok)
		}
	}
}

// Only the character's player or a GM may rearrange, only items the
// character holds, and Director-only lines only for those who see them.
func TestSetPlace(t *testing.T) {
	ctx := context.Background()
	tess := Actor{"u1", rPlayer}
	tests := []struct {
		name, item string
		who        Actor
		wantErr    bool
	}{
		{"own player", "i1", robin, false},
		{"gm", "i1", gm, false},
		{"another player", "i1", tess, true},
		{"not held", "nope", robin, true},
		{"director-only line, player", "s1", robin, true},
		{"director-only line, gm", "s1", gm, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, pl := placesFx()
			err := f.svc.SetPlace(ctx, "camp", tc.who, "c2", tc.item, PlaceBelt)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if !tc.wantErr && pl["c2"][tc.item] != PlaceBelt {
				t.Errorf("not placed: %v", pl["c2"])
			}
		})
	}
}

func TestSetPlace_NoStore(t *testing.T) {
	f := newShareFx()
	if err := f.svc.SetPlace(context.Background(), "camp", gm, "c2", "i1", PlaceHand); err == nil {
		t.Fatal("want a refusal with no place store")
	}
}

// Each item is drawn as a tile in its box; unplaced ones are in the backpack,
// and the boxes only take drags when the store is there.
func TestCharacterPanel_Boxes(t *testing.T) {
	ctx := context.Background()
	f, pl := placesFx()
	pl["c2"] = map[string]Place{"i1": PlaceBelt}
	v, err := f.svc.CharacterPanel(ctx, "camp", robin, "c2")
	if err != nil || v == nil {
		t.Fatalf("%v %v", v, err)
	}
	if !v.CanPlace {
		t.Error("CanPlace off with a store")
	}
	got := map[string]Place{}
	for _, it := range v.Items {
		got[it.ItemID] = it.Place
	}
	if got["i1"] != PlaceBelt || got["z1"] != PlacePack {
		t.Errorf("places %v", got)
	}
	var sb strings.Builder
	if err := CharacterPanel(v, "tok").Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	belt := out[strings.Index(out, `data-hp-box="belt"`):strings.Index(out, `data-hp-box="pack"`)]
	if !strings.Contains(belt, `data-item-id="i1"`) || strings.Contains(belt, `data-item-id="z1"`) {
		t.Error("belt pouch should hold only the potion")
	}
	for _, want := range []string{`data-place-url="/campaigns/camp/armory/characters/c2/items/"`, `draggable="true"`, `data-hp-carry="hand"`, `id="hp-d-i1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if n := strings.Count(out, "<li class=\"hp-tile"); n != len(v.Items) {
		t.Errorf("%d tiles for %d items", n, len(v.Items))
	}

	plain := newShareFx()
	v2, _ := plain.svc.CharacterPanel(ctx, "camp", robin, "c2")
	sb.Reset()
	_ = CharacterPanel(v2, "tok").Render(ctx, &sb)
	if strings.Contains(sb.String(), "draggable") || strings.Contains(sb.String(), "data-place-url") || strings.Contains(sb.String(), "data-hp-carry") {
		t.Error("no store: nothing to rearrange")
	}
}

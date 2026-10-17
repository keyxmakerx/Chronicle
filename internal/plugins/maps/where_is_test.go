// where_is_test.go pins the "Where it is" ladder: closest first, the chain of
// maps holding a pin that opens the one before, termination on cycles and on
// depth, and that a step reached only over a DM-only pin is marked for the DM
// and absent for a player.
package maps

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// whPin is a pin in a ladder fixture: on map `on`, optionally for the entity
// `ent` and/or opening map `opens`.
type whPin struct {
	on, ent, opens string
	dmOnly         bool
}

// whRepo builds a repo from map ids (named after their id, in sort order) and
// pins. It filters like the real one: a non-DM viewer gets no dm_only pin.
func whRepo(mapIDs []string, pins []whPin) *mockMapRepo {
	var ms []Map
	for _, id := range mapIDs {
		img := "img-" + id
		ms = append(ms, Map{ID: id, CampaignID: "camp-1", Name: id, ImageID: &img})
	}
	byMap := map[string][]Marker{}
	pinning := map[string][]string{}
	linking := map[string][]string{}
	add := func(m map[string][]string, k, v string) {
		for _, have := range m[k] {
			if have == v {
				return
			}
		}
		m[k] = append(m[k], v)
	}
	for i, p := range pins {
		mk := Marker{ID: fmt.Sprintf("pin-%d", i), MapID: p.on, X: float64(10 + i), Y: float64(20 + i), Visibility: "everyone"}
		if p.dmOnly {
			mk.Visibility = "dm_only"
		}
		if p.ent != "" {
			e := p.ent
			mk.EntityID = &e
			add(pinning, p.ent, p.on)
		}
		if p.opens != "" {
			o := p.opens
			mk.LinkedMapID = &o
			add(linking, p.opens, p.on)
		}
		byMap[p.on] = append(byMap[p.on], mk)
	}
	// The real queries order by the campaign's map order; so does the fixture.
	order := func(m map[string][]string) {
		for k, v := range m {
			var sorted []string
			for _, id := range mapIDs {
				for _, have := range v {
					if have == id {
						sorted = append(sorted, id)
					}
				}
			}
			m[k] = sorted
		}
	}
	order(pinning)
	order(linking)
	return &mockMapRepo{
		pinningEntity: pinning,
		linkingTo:     linking,
		listMapsFn:    func(context.Context, string) ([]Map, error) { return ms, nil },
		listMarkersFn: func(_ context.Context, mapID string, role int) ([]Marker, error) {
			var out []Marker
			for _, mk := range byMap[mapID] {
				if mk.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
					continue
				}
				out = append(out, mk)
			}
			return out, nil
		},
	}
}

// whShape renders steps as "a,b!,c" (! marks a DM-only step).
func whShape(l *WhereLadder) string {
	var parts []string
	for _, s := range l.Steps {
		id := s.Map.ID
		if s.DMOnly {
			id += "!"
		}
		parts = append(parts, id)
	}
	return strings.Join(parts, ",")
}

func whAlso(l *WhereLadder) string {
	var parts []string
	for _, a := range l.Also {
		id := a.Map.ID
		if a.DMOnly {
			id += "!"
		}
		parts = append(parts, id)
	}
	return strings.Join(parts, ",")
}

func TestWhereIs(t *testing.T) {
	player, owner := int(permissions.RolePlayer), int(permissions.RoleOwner)
	five := []string{"room", "inn", "dock", "city", "coast"}
	// The inn opens into the room, the dock into the inn, and so on out.
	chain := []whPin{
		{on: "room", ent: "ent-1"},
		{on: "inn", opens: "room"},
		{on: "dock", opens: "inn"},
		{on: "city", opens: "dock"},
	}
	withCoast := append(append([]whPin(nil), chain...), whPin{on: "coast", opens: "city", dmOnly: true})

	var longIDs []string
	var longPins []whPin
	for i := 0; i < 20; i++ {
		longIDs = append(longIDs, fmt.Sprintf("m%02d", i))
	}
	longPins = append(longPins, whPin{on: "m00", ent: "ent-1"})
	for i := 1; i < 20; i++ {
		longPins = append(longPins, whPin{on: longIDs[i], opens: longIDs[i-1]})
	}

	cases := []struct {
		name          string
		maps          []string
		pins          []whPin
		role          int
		want          string
		wantAlso      string
		wantTruncated bool
	}{
		{"closest first, out to the top", five, chain, player, "room,inn,dock,city", "", false},
		{"a player never gets the DM-only top step", five, withCoast, player, "room,inn,dock,city", "", false},
		{"the DM gets it, marked", five, withCoast, owner, "room,inn,dock,city,coast!", "", false},
		{
			"a DM-only pin in the middle ends the player's ladder and marks every step past it",
			five,
			[]whPin{
				{on: "room", ent: "ent-1"},
				{on: "inn", opens: "room"},
				{on: "dock", opens: "inn", dmOnly: true},
				{on: "city", opens: "dock"},
			},
			player, "room,inn", "", false,
		},
		{
			"the same ladder for the DM, marked from the DM-only pin on",
			five,
			[]whPin{
				{on: "room", ent: "ent-1"},
				{on: "inn", opens: "room"},
				{on: "dock", opens: "inn", dmOnly: true},
				{on: "city", opens: "dock"},
			},
			owner, "room,inn,dock!,city!", "", false,
		},
		{"on no map", five, []whPin{{on: "inn", opens: "room"}}, owner, "", "", false},
		{"someone else's pin does not count", five, []whPin{{on: "room", ent: "ent-2"}}, owner, "", "", false},
		{
			"a pin only the DM can see leaves a player with nothing",
			five, []whPin{{on: "room", ent: "ent-1", dmOnly: true}}, player, "", "", false,
		},
		{
			"a DM-only entity pin still starts the DM's ladder, marked",
			five, []whPin{{on: "room", ent: "ent-1", dmOnly: true}, {on: "inn", opens: "room"}}, owner, "room!,inn!", "", false,
		},
		{
			"a cycle is walked once and stops",
			five,
			[]whPin{
				{on: "room", ent: "ent-1", opens: "inn"},
				{on: "inn", opens: "room"},
			},
			player, "room,inn", "", false,
		},
		{"a map that opens itself adds nothing", five, []whPin{{on: "room", ent: "ent-1", opens: "room"}}, player, "room", "", false},
		{"the depth bound stops a long chain and says so", longIDs, longPins, player, strings.Join(longIDs[:MaxLinkTreeDepth], ","), "", true},
		{
			"several maps open this one: the name decides",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "inn", opens: "room"}, {on: "dock", opens: "room"}},
			player, "room,dock", "", false,
		},
		{
			"several open this one: a map a player can follow into wins over a DM-only pin from an earlier name",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "dock", opens: "room"}, {on: "city", opens: "room", dmOnly: true}},
			owner, "room,dock", "", false,
		},
		{
			"a second pinned map outside the chain goes under Also pinned on",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "inn", opens: "room"}, {on: "coast", ent: "ent-1"}},
			player, "room,inn", "coast", false,
		},
		{
			"a second pinned map inside the chain is not repeated",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "inn", opens: "room", ent: "ent-1"}},
			player, "room,inn", "", false,
		},
		{
			"two pins on one map count that map once",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "room", ent: "ent-1"}},
			player, "room", "", false,
		},
		{
			"a DM-only second map is listed for the DM, marked, and hidden from a player",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "coast", ent: "ent-1", dmOnly: true}},
			owner, "room", "coast!", false,
		},
		{
			"the same second map is absent for a player",
			five,
			[]whPin{{on: "room", ent: "ent-1"}, {on: "coast", ent: "ent-1", dmOnly: true}},
			player, "room", "", false,
		},
		{
			"a player-visible map leads the DM's ladder over an earlier DM-only one",
			five,
			[]whPin{{on: "room", ent: "ent-1", dmOnly: true}, {on: "inn", ent: "ent-1"}},
			owner, "inn", "room!", false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewMapService(whRepo(tc.maps, tc.pins))
			svc.(*mapService).SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{"ent-1": true}})
			got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", tc.role, "u-1")
			if err != nil {
				t.Fatal(err)
			}
			if s := whShape(got); s != tc.want {
				t.Errorf("steps = %q, want %q", s, tc.want)
			}
			if a := whAlso(got); a != tc.wantAlso {
				t.Errorf("also = %q, want %q", a, tc.wantAlso)
			}
			if got.Truncated != tc.wantTruncated {
				t.Errorf("truncated = %v, want %v", got.Truncated, tc.wantTruncated)
			}
		})
	}
}

// The outlined spot on a step is the pin that opens the closer step, and on the
// first step the entity's own pin.
func TestWhereIs_PinSpots(t *testing.T) {
	repo := whRepo([]string{"room", "inn"}, []whPin{{on: "room", ent: "ent-1"}, {on: "inn", opens: "room"}})
	svc := NewMapService(repo)
	svc.(*mapService).SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{"ent-1": true}})
	got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", int(permissions.RolePlayer), "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 || !got.Steps[0].HasPin || got.Steps[0].PinX != 10 || got.Steps[1].PinX != 11 || got.Steps[1].PinY != 21 {
		t.Errorf("steps = %+v, want the entity pin (10,20) then the opening pin (11,21)", got.Steps)
	}
}

// A page the player cannot see names no map: the entity gate blanks the pin's
// entity before the ladder looks at it.
func TestWhereIs_EntityHiddenFromViewer(t *testing.T) {
	repo := whRepo([]string{"room"}, []whPin{{on: "room", ent: "ent-1"}})
	svc := NewMapService(repo)
	svc.(*mapService).SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{}})
	got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", int(permissions.RolePlayer), "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 0 {
		t.Errorf("steps = %v, want none", whShape(got))
	}
}

// Pins hidden from the viewer must not use up the caps: an entity pinned on 60
// maps whose first 55 pins are DM-only still shows a player the 5 they can see.
func TestWhereIs_HiddenPinsDoNotUseTheCap(t *testing.T) {
	var ids []string
	var pins []whPin
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("m%02d", i)
		ids = append(ids, id)
		pins = append(pins, whPin{on: id, ent: "ent-1", dmOnly: i < 55})
	}
	svc := NewMapService(whRepo(ids, pins))
	svc.(*mapService).SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{"ent-1": true}})
	got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", int(permissions.RolePlayer), "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if s, a := whShape(got), whAlso(got); s != "m55" || a != "m56,m57,m58,m59" {
		t.Errorf("player sees steps %q also %q, want m55 and m56..m59", s, a)
	}
	if got.Truncated {
		t.Error("nothing visible was cut, so the ladder is not truncated")
	}
}

// More visible pinned maps than the cap: the list stops and says so.
func TestWhereIs_VisibleStartsCapMarksTruncated(t *testing.T) {
	var ids []string
	var pins []whPin
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("m%02d", i)
		ids = append(ids, id)
		pins = append(pins, whPin{on: id, ent: "ent-1"})
	}
	svc := NewMapService(whRepo(ids, pins))
	svc.(*mapService).SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{"ent-1": true}})
	got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", int(permissions.RolePlayer), "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Steps) + len(got.Also); n != maxWhereStarts || !got.Truncated {
		t.Errorf("%d maps (truncated %v), want %d and truncated", n, got.Truncated, maxWhereStarts)
	}
}

// Many maps opening one map: visible candidates are capped, and the walk's
// marker reads stay within the budget however many there are.
func TestWhereIs_ParentFanOutIsBounded(t *testing.T) {
	ids := []string{"room"}
	pins := []whPin{{on: "room", ent: "ent-1"}}
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("p%03d", i)
		ids = append(ids, id)
		pins = append(pins, whPin{on: id, opens: "room"})
	}
	repo := whRepo(ids, pins)
	reads := 0
	inner := repo.listMarkersFn
	repo.listMarkersFn = func(c context.Context, mapID string, role int) ([]Marker, error) {
		reads++
		return inner(c, mapID, role)
	}
	svc := NewMapService(repo)
	svc.(*mapService).SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{"ent-1": true}})
	for _, role := range []int{int(permissions.RolePlayer), int(permissions.RoleOwner)} {
		reads = 0
		got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", role, "u-1")
		if err != nil {
			t.Fatal(err)
		}
		if reads > maxWhereReads || !got.Truncated || len(got.Steps) < 2 {
			t.Errorf("role %d: %d reads (budget %d), truncated %v, steps %q", role, reads, maxWhereReads, got.Truncated, whShape(got))
		}
	}
}

// A page on no map costs one query: the maps are not even listed.
func TestWhereIs_NoMapSkipsListMaps(t *testing.T) {
	repo := whRepo([]string{"room"}, nil)
	repo.listMapsFn = func(context.Context, string) ([]Map, error) {
		t.Error("ListMaps must not run for a page on no map")
		return nil, nil
	}
	got, err := NewMapService(repo).WhereIs(context.Background(), "camp-1", "ent-1", int(permissions.RolePlayer), "u-1")
	if err != nil || len(got.Steps) != 0 {
		t.Fatalf("steps %v err %v", got, err)
	}
}

// A player gets the player-safe picture of every step and of Also; the DM gets
// the original.
func TestWhereIs_PlayersGetThePlayerSafePicture(t *testing.T) {
	pins := []whPin{{on: "room", ent: "ent-1"}, {on: "inn", opens: "room"}, {on: "coast", ent: "ent-1"}}
	svc := NewMapService(whRepo([]string{"room", "inn", "coast"}, pins)).(*mapService)
	svc.SetEntityVisibilityGate(&fakeEntityVisibilityGate{viewable: map[string]bool{"ent-1": true}})
	svc.SetShadowLookup(fakeShadowLookup{areas: []ShadowArea{{MinX: 80, MinY: 80, MaxX: 95, MaxY: 95, Strength: ShadowAlphaHint}}})
	cases := []struct {
		name         string
		role         int
		wantOriginal bool
	}{
		{"player", int(permissions.RolePlayer), false},
		{"owner", int(permissions.RoleOwner), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.WhereIs(context.Background(), "camp-1", "ent-1", tc.role, "u-1")
			if err != nil {
				t.Fatal(err)
			}
			var all []Map
			for _, s := range got.Steps {
				all = append(all, s.Map)
			}
			for _, a := range got.Also {
				all = append(all, a.Map)
			}
			if len(all) != 3 {
				t.Fatalf("%d maps, want 3", len(all))
			}
			for _, m := range all {
				hasOriginal := m.ImageID != nil
				if hasOriginal != tc.wantOriginal || (!tc.wantOriginal && m.PlayerImageURL == "") {
					t.Errorf("%s: original=%v playerURL=%q, want original=%v", m.ID, hasOriginal, m.PlayerImageURL, tc.wantOriginal)
				}
			}
		})
	}
}

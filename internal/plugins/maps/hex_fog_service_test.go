package maps

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type hexEventCall struct {
	campaignID, mapID string
	version           uint64
	path              []HexKey
}

// fakeHexEvents records every hex.changed the service asks to publish.
type fakeHexEvents struct{ calls []hexEventCall }

func (e *fakeHexEvents) PublishHexChanged(campaignID, mapID string, version uint64, path []HexKey) {
	e.calls = append(e.calls, hexEventCall{campaignID, mapID, version, path})
}

// fogFixture is map-1: 1000x1000 with hexes 100 wide (a 13 x 14 field), with the
// given hexes.party_who. It returns the service, its repo, the event recorder
// and a counter of picture-cache invalidations.
func fogFixture(partyWho string) (HexService, *fakeHexRepo, *fakeHexEvents, *int) {
	repo := newFakeHexRepo()
	events := &fakeHexEvents{}
	svc := hexSvc(repo, DrawWhoScribes)
	svc.SetEventPublisher(events)
	m := &Map{
		ID: "map-1", CampaignID: "camp-1", ImageWidth: 1000, ImageHeight: 1000,
		Display: &DisplaySettings{Grid: &GridDisplay{Type: GridHex, Size: 100}},
	}
	if partyWho != PartyWhoScribes {
		m.Display.Hexes = &HexesDisplay{PartyWho: partyWho}
	}
	svc.SetMapLoader(func(context.Context, string) (*Map, error) { return m, nil })
	inval := 0
	svc.SetPictureInvalidator(func(string) { inval++ })
	return svc, repo, events, &inval
}

func mark(repo *fakeHexRepo, keys ...HexKey) {
	for _, k := range keys {
		repo.cells[k] = HexCell{Col: k.Col, Row: k.Row, Explored: true}
	}
}

func TestMoveParty_WhoMayMove(t *testing.T) {
	tests := []struct {
		name  string
		actor HexActor
		who   string
		allow bool
	}{
		{"owner, scribes setting", actorOwner, PartyWhoScribes, true},
		{"owner, owners setting", actorOwner, PartyWhoOwners, true},
		{"DM grant, owners setting", actorDM, PartyWhoOwners, true},
		{"scribe, scribes setting", actorScribe, PartyWhoScribes, true},
		{"scribe, owners setting", actorScribe, PartyWhoOwners, false},
		{"player, scribes setting", actorPlayer, PartyWhoScribes, false},
		{"visitor, scribes setting", actorNone, PartyWhoScribes, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, _ := fogFixture(tc.who)
			mark(repo, HexKey{5, 5})
			res, err := svc.MoveParty(context.Background(), "camp-1", "map-1", tc.actor, HexFogCell{5, 5})
			if tc.allow {
				if err != nil || res == nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if !isForbidden(err) {
				t.Fatalf("expected forbidden, got %v", err)
			}
			if len(repo.partyMoves) != 0 || len(events.calls) != 0 {
				t.Error("a refused move must not write or publish")
			}
		})
	}
}

func TestMoveParty_ScribeNeedsAnUnwiredMapLoaderToStayOut(t *testing.T) {
	repo := newFakeHexRepo()
	svc := hexSvc(repo, DrawWhoScribes) // no map loader wired
	mark(repo, HexKey{5, 5})
	if _, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorScribe, HexFogCell{5, 5}); !isForbidden(err) {
		t.Fatalf("a scribe must be refused when the party rule cannot be read, got %v", err)
	}
	if _, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{5, 5}); err != nil {
		t.Fatalf("an owner is never blocked by the lookup: %v", err)
	}
}

func TestMoveParty_FirstPlacementRevealsRadiusOne(t *testing.T) {
	svc, repo, events, _ := fogFixture(PartyWhoScribes)
	res, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{5, 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Path) != 1 || res.Path[0] != (HexKey{5, 5}) {
		t.Errorf("path = %v, want just the target", res.Path)
	}
	mv := repo.partyMoves[0]
	if mv.from != nil {
		t.Errorf("first placement must expect no position, got %v", *mv.from)
	}
	if len(mv.reveal) != 7 {
		t.Errorf("revealed %d hexes, want the target and its six neighbours", len(mv.reveal))
	}
	for _, k := range mv.reveal {
		if HexDistance(k, HexKey{5, 5}) > 1 {
			t.Errorf("%v is more than one step from the target", k)
		}
	}
	if len(events.calls) != 1 || len(events.calls[0].path) != 1 {
		t.Errorf("events = %+v, want one with the one-hex path", events.calls)
	}
}

func TestMoveParty_PathRevealsRadiusOneAlongTheLine(t *testing.T) {
	svc, repo, events, _ := fogFixture(PartyWhoScribes)
	repo.layer = &HexLayer{MapID: "map-1", PartyCol: intp(2), PartyRow: intp(2), MilesPerHex: 6, MilesPerDay: 24}
	to := HexKey{8, 6}
	res, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{to.Col, to.Row})
	if err != nil {
		t.Fatal(err)
	}
	line := HexLine(HexKey{2, 2}, to)
	if len(res.Path) != len(line) {
		t.Fatalf("path = %v, want %v", res.Path, line)
	}
	for i := range line {
		if res.Path[i] != line[i] {
			t.Fatalf("path = %v, want %v", res.Path, line)
		}
	}
	want := RevealZone(line, 13, 14)
	mv := repo.partyMoves[0]
	if mv.from == nil || *mv.from != (HexKey{2, 2}) {
		t.Errorf("move expected from %v, want (2,2)", mv.from)
	}
	if len(mv.reveal) != len(want) {
		t.Errorf("revealed %d hexes, want %d", len(mv.reveal), len(want))
	}
	// The server decided what to reveal: every hex the line touches is explored
	// and no hex further than one step from it is.
	for _, p := range line {
		if !repo.cells[p].Explored {
			t.Errorf("path hex %v was not revealed", p)
		}
	}
	for k, c := range repo.cells {
		if !c.Explored {
			continue
		}
		near := false
		for _, p := range line {
			if HexDistance(p, k) <= 1 {
				near = true
			}
		}
		if !near {
			t.Errorf("%v was revealed but is not within one step of the path", k)
		}
	}
	if len(events.calls) != 1 || len(events.calls[0].path) != len(line) {
		t.Errorf("event path = %+v, want the whole path (every hex is explored)", events.calls)
	}
}

func intp(v int) *int { return &v }

func TestMoveParty_ScribeFrontier(t *testing.T) {
	tests := []struct {
		name     string
		explored []HexKey
		target   HexKey
		actor    HexActor
		allow    bool
	}{
		{"onto an explored hex", []HexKey{{5, 5}}, HexKey{5, 5}, actorScribe, true},
		{"onto a neighbour of an explored hex", []HexKey{{5, 5}}, Neighbors(5, 5)[0], actorScribe, true},
		{"two steps from the explored land", []HexKey{{5, 5}}, HexKey{5, 7}, actorScribe, false},
		{"into the dark with nothing explored", nil, HexKey{5, 5}, actorScribe, false},
		{"owner anywhere", nil, HexKey{9, 9}, actorOwner, true},
		{"DM grant anywhere", nil, HexKey{9, 9}, actorDM, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, _ := fogFixture(PartyWhoScribes)
			mark(repo, tc.explored...)
			_, err := svc.MoveParty(context.Background(), "camp-1", "map-1", tc.actor, HexFogCell{tc.target.Col, tc.target.Row})
			if tc.allow {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if !isForbidden(err) {
				t.Fatalf("expected forbidden, got %v", err)
			}
			if len(repo.partyMoves) != 0 || len(events.calls) != 0 {
				t.Error("a refused move must not write or publish")
			}
		})
	}
}

func TestMoveParty_Validation(t *testing.T) {
	tests := []struct {
		name string
		to   HexFogCell
	}{
		{"negative column", HexFogCell{-1, 3}},
		{"negative row", HexFogCell{3, -1}},
		{"past the global limit", HexFogCell{MaxHexCoord + 1, 3}},
		{"past this map's field", HexFogCell{13, 3}},
		{"below this map's field", HexFogCell{3, 14}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := fogFixture(PartyWhoScribes)
			_, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, tc.to)
			if !isBadRequest(err) {
				t.Fatalf("expected bad request, got %v", err)
			}
			if len(repo.partyMoves) != 0 {
				t.Error("an invalid move must not write")
			}
		})
	}
}

func TestMoveParty_CrossCampaignIsNotFound(t *testing.T) {
	svc, _, _, _ := fogFixture(PartyWhoScribes)
	if _, err := svc.MoveParty(context.Background(), "other-camp", "map-1", actorOwner, HexFogCell{1, 1}); !isHexNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestMoveParty_ConflictRetries(t *testing.T) {
	tests := []struct {
		name      string
		conflicts int
		ok        bool
	}{
		{"one lost race succeeds on retry", 1, true},
		{"two lost races succeed on the last try", 2, true},
		{"endless races give up with a conflict", 5, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := fogFixture(PartyWhoScribes)
			repo.conflicts = tc.conflicts
			_, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{4, 4})
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v, want success = %v", err, tc.ok)
			}
			if !tc.ok {
				var ae interface{ Error() string }
				if !errors.As(err, &ae) || !strings.Contains(err.Error(), "moved by someone else") {
					t.Errorf("err = %v, want the conflict message", err)
				}
			}
		})
	}
}

func TestMoveParty_SameHexIsANoOp(t *testing.T) {
	svc, repo, events, _ := fogFixture(PartyWhoScribes)
	repo.layer = &HexLayer{MapID: "map-1", PartyCol: intp(4), PartyRow: intp(4), Version: 9}
	res, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{4, 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != 9 || len(repo.partyMoves) != 0 || len(events.calls) != 0 {
		t.Errorf("staying put must not write, bump or publish (res=%+v)", res)
	}
}

func TestMoveParty_CellCap(t *testing.T) {
	svc, repo, _, _ := fogFixture(PartyWhoScribes)
	repo.count = MaxHexCellsPerMap
	if _, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{4, 4}); !isBadRequest(err) {
		t.Fatalf("a move that would exceed the cell cap must be refused, got %v", err)
	}
}

func TestMoveParty_PathWithheldFromTheEventWhenThePictureIsHidden(t *testing.T) {
	tests := []struct {
		name     string
		anchor   func(p *fakeHexPictures)
		wantPath bool
	}{
		{"whole map", nil, true},
		{"visible picture", func(p *fakeHexPictures) {}, true},
		{"dm-only picture", func(p *fakeHexPictures) { p.drawings["pic-1"].Visibility = "dm_only" }, false},
		{"shadowed picture", func(p *fakeHexPictures) { p.shadowed["pic-1"] = true }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, _ := fogFixture(PartyWhoScribes)
			pics := &fakeHexPictures{drawings: map[string]*Drawing{"pic-1": picture("pic-1", "map-1")}, shadowed: map[string]bool{}}
			svc.SetPictures(pics)
			if tc.anchor != nil {
				tc.anchor(pics)
				repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr("pic-1")}
			}
			if _, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorOwner, HexFogCell{4, 4}); err != nil {
				t.Fatal(err)
			}
			if len(events.calls) != 1 {
				t.Fatalf("events = %d, want 1", len(events.calls))
			}
			if got := events.calls[0].path != nil; got != tc.wantPath {
				t.Errorf("event carries a path = %v, want %v", got, tc.wantPath)
			}
			if events.calls[0].version == 0 {
				t.Error("the event must still carry the version")
			}
		})
	}
}

func TestMoveParty_ScribeRefusedOnAHiddenLayer(t *testing.T) {
	svc, repo, _, _ := fogFixture(PartyWhoScribes)
	pics := &fakeHexPictures{drawings: map[string]*Drawing{"pic-1": picture("pic-1", "map-1")}, shadowed: map[string]bool{}}
	pics.drawings["pic-1"].Visibility = "dm_only"
	svc.SetPictures(pics)
	repo.layer = &HexLayer{MapID: "map-1", AnchorDrawingID: strPtr("pic-1")}
	mark(repo, HexKey{4, 4})
	if _, err := svc.MoveParty(context.Background(), "camp-1", "map-1", actorScribe, HexFogCell{4, 4}); !isForbidden(err) {
		t.Fatalf("a scribe who cannot see the layer must not move its party, got %v", err)
	}
}

func fogCells(n int) []HexFogCell {
	out := make([]HexFogCell, n)
	for i := range out {
		out[i] = HexFogCell{Col: i % 100, Row: i / 100}
	}
	return out
}

func TestRevealFog(t *testing.T) {
	tests := []struct {
		name     string
		actor    HexActor
		cells    []HexFogCell
		explored bool
		wantErr  func(error) bool
	}{
		{"owner reveals", actorOwner, fogCells(3), true, nil},
		{"DM grant reveals", actorDM, fogCells(3), true, nil},
		{"owner hides", actorOwner, fogCells(3), false, nil},
		{"the cap of 500 is allowed", actorOwner, fogCells(500), true, nil},
		{"501 is refused", actorOwner, fogCells(501), true, isBadRequest},
		{"empty is refused", actorOwner, nil, true, isBadRequest},
		{"off the map is refused", actorOwner, []HexFogCell{{MaxHexCoord + 1, 0}}, true, isBadRequest},
		{"negative is refused", actorOwner, []HexFogCell{{-1, 0}}, true, isBadRequest},
		{"scribe refused", actorScribe, fogCells(1), true, isForbidden},
		{"player refused", actorPlayer, fogCells(1), true, isForbidden},
		{"visitor refused", actorNone, fogCells(1), true, isForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, _ := fogFixture(PartyWhoScribes)
			res, err := svc.RevealFog(context.Background(), "camp-1", "map-1", tc.actor, tc.cells, tc.explored)
			if tc.wantErr != nil {
				if !tc.wantErr(err) {
					t.Fatalf("unexpected error %v", err)
				}
				if len(repo.exploredWrites) != 0 || len(events.calls) != 0 {
					t.Error("a refused request must not write or publish")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Updated != len(tc.cells) || len(repo.exploredWrites) != 1 || repo.exploredValue[0] != tc.explored {
				t.Errorf("res = %+v, writes = %d", res, len(repo.exploredWrites))
			}
			if len(events.calls) != 1 {
				t.Errorf("a batch must publish once, got %d events", len(events.calls))
			}
		})
	}
}

func TestRevealFog_FoldsDuplicatesAndRespectsTheCellCap(t *testing.T) {
	svc, repo, _, _ := fogFixture(PartyWhoScribes)
	res, err := svc.RevealFog(context.Background(), "camp-1", "map-1", actorOwner,
		[]HexFogCell{{1, 1}, {1, 1}, {2, 2}}, true)
	if err != nil || res.Updated != 2 || len(repo.exploredWrites[0]) != 2 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}

	full, repoFull, _, _ := fogFixture(PartyWhoScribes)
	repoFull.count = MaxHexCellsPerMap
	if _, err := full.RevealFog(context.Background(), "camp-1", "map-1", actorOwner, []HexFogCell{{1, 1}}, true); !isBadRequest(err) {
		t.Fatalf("revealing a new hex on a full map must be refused, got %v", err)
	}
	repoFull.cells[HexKey{1, 1}] = HexCell{Col: 1, Row: 1, Name: "kept"}
	if _, err := full.RevealFog(context.Background(), "camp-1", "map-1", actorOwner, []HexFogCell{{1, 1}}, true); err != nil {
		t.Fatalf("revealing a hex the map already stores always works: %v", err)
	}
	if _, err := full.RevealFog(context.Background(), "camp-1", "map-1", actorOwner, []HexFogCell{{9, 9}}, false); err != nil {
		t.Fatalf("hiding is never blocked by the cap: %v", err)
	}
}

func TestResetFog(t *testing.T) {
	tests := []struct {
		name  string
		actor HexActor
		ok    bool
	}{
		{"owner", actorOwner, true}, {"DM grant", actorDM, true},
		{"scribe", actorScribe, false}, {"player", actorPlayer, false}, {"visitor", actorNone, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, _ := fogFixture(PartyWhoScribes)
			mark(repo, HexKey{1, 1}, HexKey{2, 2})
			_, err := svc.ResetFog(context.Background(), "camp-1", "map-1", tc.actor)
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v, want success = %v", err, tc.ok)
			}
			if tc.ok {
				if len(repo.cells) != 0 || repo.resets != 1 || len(events.calls) != 1 {
					t.Errorf("cells = %d resets = %d events = %d", len(repo.cells), repo.resets, len(events.calls))
				}
				return
			}
			if repo.resets != 0 || len(repo.cells) != 2 {
				t.Error("a refused reset must not change anything")
			}
		})
	}
}

func TestUpdateLayer_Fog(t *testing.T) {
	t.Run("owner turns fog on and it is announced once", func(t *testing.T) {
		svc, repo, events, inval := fogFixture(PartyWhoScribes)
		res, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner,
			UpdateHexLayerInput{FogEnabled: patch.Of(true)})
		if err != nil {
			t.Fatal(err)
		}
		if !res.FogEnabled || !repo.layer.FogEnabled || repo.anchorSets != 0 {
			t.Errorf("res = %+v anchorSets = %d (fog must not touch the anchor)", res, repo.anchorSets)
		}
		if len(events.calls) != 1 || *inval != 1 {
			t.Errorf("events = %d invalidations = %d, want 1 and 1", len(events.calls), *inval)
		}
	})
	t.Run("an absent fog_enabled keeps fog on", func(t *testing.T) {
		svc, repo, _, _ := anchorFixtureWithFog()
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true}
		if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, anchorTo("pic-1")); err != nil {
			t.Fatal(err)
		}
		if !repo.layer.FogEnabled {
			t.Error("an anchor-only body switched fog off")
		}
	})
	t.Run("null and false turn fog off", func(t *testing.T) {
		for name, f := range map[string]patch.Field[bool]{"null": patch.Null[bool](), "false": patch.Of(false)} {
			svc, repo, _, _ := fogFixture(PartyWhoScribes)
			repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true}
			if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{FogEnabled: f}); err != nil {
				t.Fatal(err)
			}
			if repo.layer.FogEnabled {
				t.Errorf("%s did not turn fog off", name)
			}
		}
	})
	t.Run("roles", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			actor HexActor
			ok    bool
		}{{"owner", actorOwner, true}, {"DM grant", actorDM, true}, {"scribe", actorScribe, false}, {"player", actorPlayer, false}} {
			svc, repo, events, _ := fogFixture(PartyWhoScribes)
			_, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", tc.actor, UpdateHexLayerInput{FogEnabled: patch.Of(true)})
			if tc.ok != (err == nil) {
				t.Errorf("%s: err = %v", tc.name, err)
			}
			if !tc.ok && (repo.layer != nil || len(events.calls) != 0) {
				t.Errorf("%s: a refused change must not write or publish", tc.name)
			}
		}
	})
	t.Run("anchor and fog together publish once", func(t *testing.T) {
		svc, repo, events, _ := anchorFixtureWithFog()
		_, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner,
			UpdateHexLayerInput{AnchorDrawingID: patchOf("pic-1"), FogEnabled: patch.Of(true)})
		if err != nil {
			t.Fatal(err)
		}
		if !repo.layer.FogEnabled || repo.layer.AnchorDrawingID == nil || len(events.calls) != 1 {
			t.Errorf("layer = %+v events = %d", repo.layer, len(events.calls))
		}
	})
}

// anchorFixtureWithFog is anchorFixture plus the events and map loader.
func anchorFixtureWithFog() (HexService, *fakeHexRepo, *fakeHexEvents, *fakeHexPictures) {
	svc, repo, pics := anchorFixture()
	events := &fakeHexEvents{}
	svc.SetEventPublisher(events)
	return svc, repo, events, pics
}

func TestPatchCells_PublishesOncePerBatch(t *testing.T) {
	svc, _, events, _ := fogFixture(PartyWhoScribes)
	_, err := svc.PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
		[]UpdateHexCellInput{paint(1, 1, "forest"), paint(2, 2, "hills"), paint(3, 3, "water")})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.calls) != 1 {
		t.Fatalf("a batch of three hexes published %d events, want 1", len(events.calls))
	}
}

// The viewer's read, by role: fog on, the party standing on an unexplored hex.
func TestGetHexes_FogByRole(t *testing.T) {
	tests := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		cells     int
		party     bool
	}{
		{"owner", campaigns.RoleOwner, false, 3, true},
		{"DM-granted player", campaigns.RolePlayer, true, 3, true},
		{"scribe", campaigns.RoleScribe, false, 1, false},
		{"player", campaigns.RolePlayer, false, 1, false},
		{"public visitor", campaigns.RoleNone, false, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := fogFixture(PartyWhoScribes)
			repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, PartyCol: intp(7), PartyRow: intp(7), Version: 4}
			repo.cells[HexKey{1, 1}] = HexCell{Col: 1, Row: 1, Explored: true, Name: "Port"}
			repo.cells[HexKey{2, 2}] = HexCell{Col: 2, Row: 2, Name: "Hidden barrow"}
			repo.cells[HexKey{3, 3}] = HexCell{Col: 3, Row: 3, Notes: strPtr("secret")}
			h := NewHexHandler(svc)
			rec := hexRequest(t, h, http.MethodGet, "", tc.role, tc.dmGranted, h.GetHexes)
			if rec.status != http.StatusOK {
				t.Fatalf("status = %d", rec.status)
			}
			body := rec.Body.String()
			if got := strings.Count(body, `"explored"`); got != tc.cells {
				t.Errorf("response has %d cells, want %d: %s", got, tc.cells, body)
			}
			privileged := tc.cells == 3
			if has := strings.Contains(body, "Hidden barrow") || strings.Contains(body, "secret"); has != privileged {
				t.Errorf("unexplored contents in response = %v, want %v", has, privileged)
			}
			if has := strings.Contains(body, `"party_col":7`); has != tc.party {
				t.Errorf("party position in response = %v, want %v", has, tc.party)
			}
		})
	}
}

func TestHexHandler_FogAndPartyRoutes(t *testing.T) {
	tests := []struct {
		name      string
		call      string
		body      string
		role      campaigns.Role
		dmGranted bool
		want      int
	}{
		{"owner reveals", "fog", `{"cells":[{"col":1,"row":1}],"explored":true}`, campaigns.RoleOwner, false, http.StatusOK},
		{"DM grant hides", "fog", `{"cells":[{"col":1,"row":1}],"explored":false}`, campaigns.RolePlayer, true, http.StatusOK},
		{"owner resets", "fog", `{"reset":true}`, campaigns.RoleOwner, false, http.StatusOK},
		{"scribe refused", "fog", `{"cells":[{"col":1,"row":1}],"explored":true}`, campaigns.RoleScribe, false, http.StatusForbidden},
		{"player refused", "fog", `{"cells":[{"col":1,"row":1}],"explored":true}`, campaigns.RolePlayer, false, http.StatusForbidden},
		{"scribe cannot reset", "fog", `{"reset":true}`, campaigns.RoleScribe, false, http.StatusForbidden},
		{"explored is required", "fog", `{"cells":[{"col":1,"row":1}]}`, campaigns.RoleOwner, false, http.StatusBadRequest},
		{"a cell needs a row", "fog", `{"cells":[{"col":1}],"explored":true}`, campaigns.RoleOwner, false, http.StatusBadRequest},
		{"not json", "fog", `nope`, campaigns.RoleOwner, false, http.StatusBadRequest},
		{"owner moves the party", "party", `{"col":4,"row":4}`, campaigns.RoleOwner, false, http.StatusOK},
		{"scribe moves onto explored", "party", `{"col":1,"row":1}`, campaigns.RoleScribe, false, http.StatusOK},
		{"scribe into the dark", "party", `{"col":9,"row":9}`, campaigns.RoleScribe, false, http.StatusForbidden},
		{"player refused", "party", `{"col":1,"row":1}`, campaigns.RolePlayer, false, http.StatusForbidden},
		{"party needs a col", "party", `{"row":1}`, campaigns.RoleOwner, false, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := fogFixture(PartyWhoScribes)
			mark(repo, HexKey{1, 1})
			h := NewHexHandler(svc)
			fn := h.PostHexFog
			if tc.call == "party" {
				fn = h.PutHexParty
			}
			rec := hexRequest(t, h, http.MethodPost, tc.body, tc.role, tc.dmGranted, fn)
			if rec.status != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.status, tc.want, rec.Body.String())
			}
			if tc.call == "party" && tc.want == http.StatusOK && !strings.Contains(rec.Body.String(), `"path":[{"col":`) {
				t.Errorf("the move must return the path: %s", rec.Body.String())
			}
		})
	}
}

func TestHexHandler_FogCapIs500(t *testing.T) {
	cells := make([]string, 501)
	for i := range cells {
		cells[i] = `{"col":1,"row":1}`
	}
	svc, _, _, _ := fogFixture(PartyWhoScribes)
	h := NewHexHandler(svc)
	rec := hexRequest(t, h, http.MethodPost, `{"cells":[`+strings.Join(cells, ",")+`],"explored":true}`, campaigns.RoleOwner, false, h.PostHexFog)
	if rec.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for 501 cells", rec.status)
	}
}

func TestFogMask_Service(t *testing.T) {
	t.Run("fog off is no mask", func(t *testing.T) {
		svc, _, _, _ := fogFixture(PartyWhoScribes)
		if m, err := svc.FogMask(context.Background(), "map-1"); err != nil || m != nil {
			t.Fatalf("mask = %v err = %v, want none", m, err)
		}
	})
	t.Run("fog on builds the mask from the explored set", func(t *testing.T) {
		svc, repo, _, _ := fogFixture(PartyWhoScribes)
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, Version: 3}
		mark(repo, HexKey{1, 1})
		repo.cells[HexKey{2, 2}] = HexCell{Col: 2, Row: 2, Name: "unexplored"}
		m, err := svc.FogMask(context.Background(), "map-1")
		if err != nil || m == nil {
			t.Fatalf("mask = %v err = %v", m, err)
		}
		if m.Version != 3 || len(m.Explored) != 1 || !m.Explored[HexKey{1, 1}] || m.Geo.Cols != 13 || m.Geo.Rows != 14 {
			t.Errorf("mask = %+v", m)
		}
	})
	t.Run("hexes that are not the grid mean no fog", func(t *testing.T) {
		svc, repo, _, _ := fogFixture(PartyWhoScribes)
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true}
		svc.SetMapLoader(func(context.Context, string) (*Map, error) {
			return &Map{ID: "map-1", ImageWidth: 1000, ImageHeight: 1000}, nil
		})
		if m, err := svc.FogMask(context.Background(), "map-1"); err != nil || m != nil {
			t.Fatalf("mask = %v err = %v, want none", m, err)
		}
	})
	t.Run("fog on with no way to build it fails closed", func(t *testing.T) {
		repo := newFakeHexRepo()
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true}
		svc := hexSvc(repo, DrawWhoScribes)
		if _, err := svc.FogMask(context.Background(), "map-1"); err == nil {
			t.Fatal("expected an error, not an unfogged map")
		}
	})
	t.Run("a pinned layer uses the picture's box", func(t *testing.T) {
		svc, repo, _, _ := fogFixture(PartyWhoScribes)
		pic := picture("pic-1", "map-1")
		pic.Points = []byte(`[{"x":50,"y":0},{"x":100,"y":100}]`)
		svc.SetPictures(&fakeHexPictures{drawings: map[string]*Drawing{"pic-1": pic}})
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, AnchorDrawingID: strPtr("pic-1")}
		m, err := svc.FogMask(context.Background(), "map-1")
		if err != nil || m == nil || m.AnchorID != "pic-1" || m.CoversMapPicture() {
			t.Fatalf("mask = %+v err = %v", m, err)
		}
		if m.Geo.Ox < 500 {
			t.Errorf("field origin x = %v, want inside the right half of the map", m.Geo.Ox)
		}
	})
	t.Run("a stale anchor falls back to the whole map", func(t *testing.T) {
		svc, repo, _, _ := fogFixture(PartyWhoScribes)
		svc.SetPictures(&fakeHexPictures{drawings: map[string]*Drawing{}})
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, AnchorDrawingID: strPtr("gone")}
		m, err := svc.FogMask(context.Background(), "map-1")
		if err != nil || m == nil || m.AnchorID != "" || !m.CoversMapPicture() {
			t.Fatalf("mask = %+v err = %v", m, err)
		}
	})
}

// A guard that forgets who is allowed to see under fog is the worst failure
// here, so the role split is pinned against the permissions package itself.
func TestFogHidingAppliesMatchesDMOnlyVisibility(t *testing.T) {
	for role := permissions.RoleNone; role <= permissions.RoleOwner; role++ {
		if fogHidingApplies(role) == permissions.CanSeeDmOnly(role) {
			t.Errorf("role %d: fog hiding must apply exactly to those who cannot see DM-only content", role)
		}
	}
}

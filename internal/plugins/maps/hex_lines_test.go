package maps

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func lineOf(v int) patch.Field[int] { return patch.Of(v) }
func lineNull() patch.Field[int]    { return patch.Null[int]() }

func TestPatchCells_LineValidation(t *testing.T) {
	tests := []struct {
		name  string
		entry UpdateHexCellInput
		ok    bool
	}{
		{"road 0 is a dot", UpdateHexCellInput{Road: lineOf(0)}, true},
		{"road 63 is all six sides", UpdateHexCellInput{Road: lineOf(63)}, true},
		{"road 64 is out of range", UpdateHexCellInput{Road: lineOf(64)}, false},
		{"road -1 is out of range", UpdateHexCellInput{Road: lineOf(-1)}, false},
		{"river 64 is out of range", UpdateHexCellInput{River: lineOf(64)}, false},
		{"river -1 is out of range", UpdateHexCellInput{River: lineOf(-1)}, false},
		{"trail 64 is out of range", UpdateHexCellInput{Trail: lineOf(64)}, false},
		{"trail -1 is out of range", UpdateHexCellInput{Trail: lineOf(-1)}, false},
		{"null clears a line", UpdateHexCellInput{Trail: lineNull()}, true},
		{"one bad line fails the entry even beside a good one", UpdateHexCellInput{Road: lineOf(3), River: lineOf(99)}, false},
		{"a lines-only entry names a field to change", UpdateHexCellInput{River: lineOf(5)}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
				[]UpdateHexCellInput{tc.entry})
			if tc.ok && err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if !tc.ok && (!isBadRequest(err) || len(repo.applied) != 0) {
				t.Fatalf("expected 400 and no write, got %v", err)
			}
		})
	}
}

// A line field follows the same contract as terrain: absent is not written,
// null clears, a value sets, and naming a line never marks another field set.
func TestPatchCells_LinePartialUpdateContract(t *testing.T) {
	tests := []struct {
		name  string
		entry UpdateHexCellInput
		want  HexCellWrite
	}{
		{"road only", UpdateHexCellInput{Col: 1, Row: 1, Road: lineOf(9)},
			HexCellWrite{Col: 1, Row: 1, RoadSet: true, Road: intp(9)}},
		{"river only", UpdateHexCellInput{Col: 1, Row: 1, River: lineOf(0)},
			HexCellWrite{Col: 1, Row: 1, RiverSet: true, River: intp(0)}},
		{"null trail clears", UpdateHexCellInput{Col: 1, Row: 1, Trail: lineNull()},
			HexCellWrite{Col: 1, Row: 1, TrailSet: true}},
		{"all three at once", UpdateHexCellInput{Col: 1, Row: 1, Road: lineOf(1), River: lineNull(), Trail: lineOf(2)},
			HexCellWrite{Col: 1, Row: 1, RoadSet: true, Road: intp(1), RiverSet: true, TrailSet: true, Trail: intp(2)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			if _, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
				[]UpdateHexCellInput{tc.entry}); err != nil {
				t.Fatal(err)
			}
			got, want := repo.applied[0][0], tc.want
			if got != want && !(lineEq(got.Road, want.Road) && lineEq(got.River, want.River) && lineEq(got.Trail, want.Trail) &&
				got.RoadSet == want.RoadSet && got.RiverSet == want.RiverSet && got.TrailSet == want.TrailSet) {
				t.Errorf("write = %+v, want %+v", got, want)
			}
			if got.TerrainSet || got.PieceSet || got.NameSet || got.NotesSet {
				t.Errorf("a lines-only entry must not set other fields: %+v", got)
			}
		})
	}
}

func lineEq(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestPatchCells_LineEntriesFold(t *testing.T) {
	repo := newFakeHexRepo()
	_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, []UpdateHexCellInput{
		{Col: 2, Row: 2, Road: lineOf(1)},
		{Col: 2, Row: 2, River: lineOf(4)},
		{Col: 2, Row: 2, Road: lineOf(8)},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := repo.applied[0]
	if len(w) != 1 || !w[0].RoadSet || *w[0].Road != 8 || !w[0].RiverSet || *w[0].River != 4 || w[0].TrailSet {
		t.Errorf("folded write = %+v; want road 8 (later wins), river 4, trail unset", w)
	}
}

func TestPatchCells_LineRoleGates(t *testing.T) {
	tests := []struct {
		name   string
		actor  HexActor
		policy string
		ok     bool
	}{
		{"owner", actorOwner, DrawWhoScribes, true},
		{"scribe", actorScribe, DrawWhoScribes, true},
		{"player", actorPlayer, DrawWhoScribes, false},
		{"public", actorNone, DrawWhoScribes, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			_, err := hexSvc(repo, tc.policy).PatchCells(context.Background(), "camp-1", "map-1", tc.actor,
				[]UpdateHexCellInput{{Col: 0, Row: 0, Road: lineOf(1)}})
			if tc.ok && err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if !tc.ok && (!isForbidden(err) || len(repo.applied) != 0) {
				t.Fatalf("expected 403 and no write, got %v", err)
			}
		})
	}
}

func TestPatchCells_LinesShareTheBatchCap(t *testing.T) {
	repo := newFakeHexRepo()
	entries := make([]UpdateHexCellInput, MaxHexBatch+1)
	for i := range entries {
		entries[i] = UpdateHexCellInput{Col: i % 300, Row: i / 300, River: lineOf(1)}
	}
	_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, entries)
	if !isBadRequest(err) || len(repo.applied) != 0 {
		t.Fatalf("expected 400 and no write, got %v", err)
	}
}

func TestHexHandler_PatchBindsLines(t *testing.T) {
	repo := newFakeHexRepo()
	h := NewHexHandler(hexSvc(repo, DrawWhoScribes))
	rec := hexRequest(t, h, http.MethodPatch,
		`{"cells":[{"col":1,"row":1,"road":9},{"col":2,"row":2,"river":null},{"col":3,"row":3,"terrain":"hills"}]}`,
		campaigns.RoleOwner, false, h.PatchHexCells)
	if rec.status != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.status, rec.Body.String())
	}
	w := repo.applied[0]
	if !w[0].RoadSet || *w[0].Road != 9 || w[0].RiverSet || w[0].TrailSet || w[0].TerrainSet {
		t.Errorf("entry 0 = %+v: only road was sent", w[0])
	}
	if !w[1].RiverSet || w[1].River != nil || w[1].RoadSet {
		t.Errorf("entry 1 = %+v: river null must clear and nothing else change", w[1])
	}
	if w[2].RoadSet || w[2].RiverSet || w[2].TrailSet || !w[2].TerrainSet {
		t.Errorf("entry 2 = %+v: absent lines must stay absent", w[2])
	}

	bad := hexRequest(t, h, http.MethodPatch, `{"cells":[{"col":1,"row":1,"road":64}]}`,
		campaigns.RoleOwner, false, h.PatchHexCells)
	if bad.status != http.StatusBadRequest {
		t.Errorf("road 64 status = %d, want 400", bad.status)
	}
	frac := hexRequest(t, h, http.MethodPatch, `{"cells":[{"col":1,"row":1,"road":1.5}]}`,
		campaigns.RoleOwner, false, h.PatchHexCells)
	if frac.status != http.StatusBadRequest {
		t.Errorf("road 1.5 status = %d, want 400", frac.status)
	}
	str := hexRequest(t, h, http.MethodPatch, `{"cells":[{"col":1,"row":1,"road":"3"}]}`,
		campaigns.RoleOwner, false, h.PatchHexCells)
	if str.status != http.StatusBadRequest {
		t.Errorf("road \"3\" status = %d, want 400", str.status)
	}
}

// bitToward is the mask bit for the neighbour of (col,row) that is (tc,tr).
func bitToward(t *testing.T, col, row, tc, tr int) int {
	t.Helper()
	for i, n := range Neighbors(col, row) {
		if n.Col == tc && n.Row == tr {
			return 1 << i
		}
	}
	t.Fatalf("(%d,%d) is not next to (%d,%d)", tc, tr, col, row)
	return 0
}

// Lines ride on their rows, so these cases prove a filtered viewer gets no
// hidden row and no exit toward a hex it was not sent, while a viewer who sees
// everything gets the stored masks untouched.
func TestVisibleCells_LineExits(t *testing.T) {
	// Row 2 is even: (2,2) neighbours (3,2) east, (2,3) south-east... derived by
	// bitToward so the test does not hard-code the layout.
	a, b, hidden := HexKey{2, 2}, HexKey{3, 2}, HexKey{1, 2}
	absent := Neighbors(a.Col, a.Row)[1] // a neighbour with no row at all
	full := 63
	build := func() []HexCell {
		return []HexCell{
			{Col: a.Col, Row: a.Row, Explored: true, Road: intp(full), River: intp(full), Trail: intp(0)},
			{Col: b.Col, Row: b.Row, Explored: true, River: intp(bitToward(t, b.Col, b.Row, a.Col, a.Row))},
			{Col: hidden.Col, Row: hidden.Row, Explored: false, Road: intp(full), River: intp(full)},
		}
	}
	towardB := bitToward(t, a.Col, a.Row, b.Col, b.Row)

	tests := []struct {
		name string
		fog  bool
		role int
		// wantA is the mask expected on (2,2)'s river; wantRows the rows sent.
		wantRows  int
		wantRiver int
	}{
		{"fog on: owner sees full masks", true, permissions.RoleOwner, 3, full},
		{"fog off: owner sees full masks", false, permissions.RoleOwner, 3, full},
		{"fog off: player sees full masks", false, permissions.RolePlayer, 3, full},
		{"fog off: public sees full masks", false, permissions.RoleNone, 3, full},
		{"fog on: player gets explored rows and clipped exits", true, permissions.RolePlayer, 2, towardB},
		{"fog on: scribe gets explored rows and clipped exits", true, permissions.RoleScribe, 2, towardB},
		{"fog on: public gets explored rows and clipped exits", true, permissions.RoleNone, 2, towardB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := VisibleCells(HexLayer{FogEnabled: tc.fog}, build(), tc.role)
			if len(got) != tc.wantRows {
				t.Fatalf("got %d rows, want %d", len(got), tc.wantRows)
			}
			var ca *HexCell
			for i := range got {
				if got[i].Col == a.Col && got[i].Row == a.Row {
					ca = &got[i]
				}
				if tc.fog && !permissions.CanSeeDmOnly(tc.role) && got[i].Col == hidden.Col && got[i].Row == hidden.Row {
					t.Fatal("an unexplored row, and so its lines, reached a filtered viewer")
				}
			}
			if ca == nil || ca.River == nil || *ca.River != tc.wantRiver || *ca.Road != tc.wantRiver {
				t.Fatalf("(2,2) river/road = %v/%v, want %d", ptrStr(ca.River), ptrStr(ca.Road), tc.wantRiver)
			}
			if ca.Trail == nil || *ca.Trail != 0 {
				t.Errorf("a dot must stay a dot, got %v", ptrStr(ca.Trail))
			}
			// The general rule, stated independently of the expectations above.
			if tc.fog && !permissions.CanSeeDmOnly(tc.role) {
				sent := map[HexKey]bool{}
				for _, c := range got {
					sent[HexKey{c.Col, c.Row}] = true
				}
				for _, c := range got {
					for _, m := range []*int{c.Road, c.River, c.Trail} {
						if m == nil {
							continue
						}
						for i, n := range Neighbors(c.Col, c.Row) {
							if *m&(1<<i) != 0 && !sent[n] {
								t.Errorf("(%d,%d) has an exit toward (%d,%d), which was not sent", c.Col, c.Row, n.Col, n.Row)
							}
						}
					}
				}
				if sent[absent] {
					t.Errorf("fixture bug: %v should be absent", absent)
				}
			}
		})
	}
}

func TestVisibleCells_ClippingDoesNotMutateInput(t *testing.T) {
	in := []HexCell{{Col: 2, Row: 2, Explored: true, River: intp(63)}}
	_ = VisibleCells(HexLayer{FogEnabled: true}, in, permissions.RolePlayer)
	if *in[0].River != 63 {
		t.Error("clipping must not change the caller's stored mask")
	}
}

func TestVisibleCells_NoLinesStaysNil(t *testing.T) {
	got := VisibleCells(HexLayer{FogEnabled: true}, []HexCell{{Col: 0, Row: 0, Explored: true}}, permissions.RolePlayer)
	if got[0].Road != nil || got[0].River != nil || got[0].Trail != nil {
		t.Errorf("a hex with no lines must not gain any: %+v", got[0])
	}
}

// The wire contract: a line absent from a hex is left out of the JSON, while a
// dot (0) is sent as 0.
func TestHexCell_LinesJSON(t *testing.T) {
	b, err := json.Marshal(HexCell{Col: 1, Row: 2, River: intp(0), Trail: intp(5)})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["road"]; ok {
		t.Errorf("absent road must be omitted: %s", b)
	}
	if v, ok := m["river"]; !ok || v != float64(0) {
		t.Errorf("river dot must be sent as 0: %s", b)
	}
	if m["trail"] != float64(5) {
		t.Errorf("trail = %v, want 5", m["trail"])
	}
}

// A writer filtered by fog holds masks clipped to explored hexes. Whatever
// they send back, the stored exits toward unexplored neighbours must stay and
// no new exit into the fog may appear; owners, DM grants and fog-off writes are
// stored exactly as sent.
func TestPatchCells_FogFilteredWriterKeepsHiddenExits(t *testing.T) {
	home := HexKey{2, 2}
	n := Neighbors(home.Col, home.Row)
	bit := func(i int) int { return 1 << i }
	// n[0] is explored, n[1] is unexplored, n[2] has no row at all.
	setup := func(fog bool, stored *int) *fakeHexRepo {
		repo := newFakeHexRepo()
		repo.layer = &HexLayer{MapID: "map-1", FogEnabled: fog}
		repo.cells[home] = HexCell{Col: home.Col, Row: home.Row, Explored: true, River: stored}
		repo.cells[n[0]] = HexCell{Col: n[0].Col, Row: n[0].Row, Explored: true}
		repo.cells[n[1]] = HexCell{Col: n[1].Col, Row: n[1].Row, Explored: false, River: intp(0)}
		return repo
	}
	tests := []struct {
		name   string
		fog    bool
		actor  HexActor
		stored *int
		entry  UpdateHexCellInput
		want   *int
	}{
		{"scribe adds an explored exit; the stored hidden exit survives",
			true, actorScribe, intp(bit(1)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0))}, intp(bit(0) | bit(1))},
		{"scribe's own exit stays when the client sends it back clipped",
			true, actorScribe, intp(bit(0) | bit(1)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0))}, intp(bit(0) | bit(1))},
		{"scribe cannot remove an exit by omitting a hidden side",
			true, actorScribe, intp(bit(1) | bit(2)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(0)}, intp(bit(1) | bit(2))},
		{"scribe cannot add an exit toward an unexplored hex",
			true, actorScribe, intp(bit(0)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0) | bit(1))}, intp(bit(0))},
		{"scribe cannot add an exit toward an absent hex",
			true, actorScribe, intp(bit(0)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0) | bit(2))}, intp(bit(0))},
		{"no stored line yet: only explored sides are taken",
			true, actorScribe, nil, UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0) | bit(1))}, intp(bit(0))},
		{"no stored line yet: a dot is stored as a dot",
			true, actorScribe, nil, UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(0)}, intp(0)},
		{"null still clears the whole line",
			true, actorScribe, intp(bit(0) | bit(1)), UpdateHexCellInput{Col: 2, Row: 2, River: lineNull()}, nil},
		{"owner is stored exactly as sent",
			true, actorOwner, intp(bit(1)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0) | bit(2))}, intp(bit(0) | bit(2))},
		{"DM grant is stored exactly as sent",
			true, actorDM, intp(bit(1)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(2))}, intp(bit(2))},
		{"fog off stores exactly as sent",
			false, actorScribe, intp(bit(1)), UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(bit(0) | bit(2))}, intp(bit(0) | bit(2))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := setup(tc.fog, tc.stored)
			if _, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", tc.actor,
				[]UpdateHexCellInput{tc.entry}); err != nil {
				t.Fatal(err)
			}
			w := repo.applied[0][0]
			if !w.RiverSet || !lineEq(w.River, tc.want) {
				t.Errorf("stored river = %s (set %v), want %s", ptrStr(w.River), w.RiverSet, ptrStr(tc.want))
			}
		})
	}

	t.Run("each line kind is merged against its own stored mask", func(t *testing.T) {
		repo := setup(true, nil)
		c := repo.cells[home]
		c.Road, c.Trail = intp(bit(1)), intp(bit(1)|bit(0))
		repo.cells[home] = c
		_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorScribe,
			[]UpdateHexCellInput{{Col: 2, Row: 2, Road: lineOf(bit(0)), Trail: lineOf(bit(2))}})
		if err != nil {
			t.Fatal(err)
		}
		w := repo.applied[0][0]
		if !lineEq(w.Road, intp(bit(0)|bit(1))) || !lineEq(w.Trail, intp(bit(1))) {
			t.Errorf("road %s trail %s; want %d and %d", ptrStr(w.Road), ptrStr(w.Trail), bit(0)|bit(1), bit(1))
		}
	})

	t.Run("a scribe's write to an unexplored hex is still refused", func(t *testing.T) {
		repo := setup(true, nil)
		_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorScribe,
			[]UpdateHexCellInput{{Col: n[1].Col, Row: n[1].Row, River: lineOf(1)}})
		if !isForbidden(err) {
			t.Fatalf("expected 403, got %v", err)
		}
	})
}

// The neighbour read costs one extra GetCells, and only when a filtered
// writer's batch sets a line to a value.
func TestPatchCells_NeighbourReadOnlyWhenNeeded(t *testing.T) {
	tests := []struct {
		name  string
		actor HexActor
		fog   bool
		entry UpdateHexCellInput
		reads int
	}{
		{"filtered writer, line value", actorScribe, true, UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(1)}, 2},
		{"filtered writer, line cleared", actorScribe, true, UpdateHexCellInput{Col: 2, Row: 2, River: lineNull()}, 1},
		{"filtered writer, terrain stroke", actorScribe, true, UpdateHexCellInput{Col: 2, Row: 2, Terrain: patchOf("forest")}, 1},
		{"owner, line value", actorOwner, true, UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(1)}, 1},
		{"fog off, line value", actorScribe, false, UpdateHexCellInput{Col: 2, Row: 2, River: lineOf(1)}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			repo.layer = &HexLayer{MapID: "map-1", FogEnabled: tc.fog}
			repo.cells[HexKey{2, 2}] = HexCell{Col: 2, Row: 2, Explored: true}
			if _, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", tc.actor,
				[]UpdateHexCellInput{tc.entry}); err != nil {
				t.Fatal(err)
			}
			if repo.getCalls != tc.reads {
				t.Errorf("GetCells called %d times, want %d", repo.getCalls, tc.reads)
			}
		})
	}
}

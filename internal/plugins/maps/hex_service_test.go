package maps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeHexRepo is an in-memory HexRepository that records every write.
type fakeHexRepo struct {
	layer      *HexLayer
	cells      map[HexKey]HexCell
	applied    [][]HexCellWrite
	by         string
	count      int // overrides CountCells when non-zero
	anchorSets int // SetAnchor calls
	travelSets int // SetTravel calls
	bumps      int // BumpVersion calls

	// Fog and party writes.
	exploredWrites [][]HexKey // keys of each SetExplored call
	exploredValue  []bool     // the value each SetExplored call set
	resets         int
	partyMoves     []partyMove
	conflicts      int // ApplyParty answers Conflict this many times first
}

// partyMove records one ApplyParty call.
type partyMove struct {
	from   *HexKey
	to     HexKey
	reveal []HexKey
}

func newFakeHexRepo() *fakeHexRepo { return &fakeHexRepo{cells: map[HexKey]HexCell{}} }

func (r *fakeHexRepo) GetLayer(context.Context, string) (*HexLayer, error) { return r.layer, nil }
func (r *fakeHexRepo) ListCells(context.Context, string) ([]HexCell, error) {
	out := []HexCell{}
	for _, c := range r.cells {
		out = append(out, c)
	}
	return out, nil
}
func (r *fakeHexRepo) GetCells(_ context.Context, _ string, keys []HexKey) (map[HexKey]HexCell, error) {
	out := map[HexKey]HexCell{}
	for _, k := range keys {
		if c, ok := r.cells[k]; ok {
			out[k] = c
		}
	}
	return out, nil
}
func (r *fakeHexRepo) CountCells(context.Context, string) (int, error) {
	if r.count != 0 {
		return r.count, nil
	}
	return len(r.cells), nil
}
func (r *fakeHexRepo) ApplyCells(_ context.Context, _, userID string, w []HexCellWrite) (uint64, error) {
	r.applied = append(r.applied, w)
	r.by = userID
	return uint64(len(r.applied)), nil
}

func (r *fakeHexRepo) BumpVersion(_ context.Context, mapID string) (uint64, error) {
	r.ensureLayer(mapID)
	r.layer.Version++
	r.bumps++
	return r.layer.Version, nil
}

// hexSvc builds a service whose map "map-1" belongs to "camp-1".
func hexSvc(repo *fakeHexRepo, policy string) HexService {
	s := NewHexService(repo)
	s.SetMapLookup(func(_ context.Context, id string) (string, error) {
		if id != "map-1" {
			return "", apperror.NewNotFound("map not found")
		}
		return "camp-1", nil
	})
	s.SetDrawPolicyLookup(func(context.Context, string) (string, error) { return policy, nil })
	return s
}

func paint(col, row int, terrain string) UpdateHexCellInput {
	return UpdateHexCellInput{Col: col, Row: row, Terrain: patchOf(terrain)}
}

var (
	actorOwner  = HexActor{UserID: "u-owner", Role: permissions.RoleOwner, IsDM: true}
	actorDM     = HexActor{UserID: "u-dm", Role: permissions.RolePlayer, IsDM: true}
	actorScribe = HexActor{UserID: "u-scribe", Role: permissions.RoleScribe}
	actorPlayer = HexActor{UserID: "u-player", Role: permissions.RolePlayer}
	actorNone   = HexActor{Role: permissions.RoleNone}
)

func isForbidden(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusForbidden
}
func isBadRequest(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusBadRequest
}
func isHexNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

func TestPatchCells_RoleGates(t *testing.T) {
	tests := []struct {
		name    string
		actor   HexActor
		policy  string
		fog     bool
		explore bool // whether hex 1,1 is explored
		allow   bool
	}{
		{"owner writes", actorOwner, DrawWhoScribes, false, false, true},
		{"owner writes on an owners-only map", actorOwner, DrawWhoOwners, false, false, true},
		{"owner writes an unexplored hex under fog", actorOwner, DrawWhoScribes, true, false, true},
		{"DM grant writes on an owners-only map", actorDM, DrawWhoOwners, false, false, true},
		{"DM grant writes an unexplored hex under fog", actorDM, DrawWhoScribes, true, false, true},
		{"scribe writes where scribes may draw", actorScribe, DrawWhoScribes, false, false, true},
		{"scribe refused when draw policy is owners", actorScribe, DrawWhoOwners, false, false, false},
		{"scribe refused on an unexplored hex under fog", actorScribe, DrawWhoScribes, true, false, false},
		{"scribe writes an explored hex under fog", actorScribe, DrawWhoScribes, true, true, true},
		{"scribe writes an unexplored hex when fog is off", actorScribe, DrawWhoScribes, false, false, true},
		{"player refused", actorPlayer, DrawWhoScribes, false, true, false},
		{"member-less visitor refused", actorNone, DrawWhoScribes, false, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			repo.layer = &HexLayer{MapID: "map-1", FogEnabled: tc.fog, MilesPerHex: 6, MilesPerDay: 24}
			repo.cells[HexKey{1, 1}] = HexCell{Col: 1, Row: 1, Explored: tc.explore}
			_, err := hexSvc(repo, tc.policy).PatchCells(context.Background(), "camp-1", "map-1", tc.actor,
				[]UpdateHexCellInput{paint(1, 1, "forest")})
			if tc.allow {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				if len(repo.applied) != 1 {
					t.Fatalf("expected one write, got %d", len(repo.applied))
				}
				if repo.by != tc.actor.UserID {
					t.Errorf("updated_by = %q, want %q", repo.by, tc.actor.UserID)
				}
				return
			}
			if !isForbidden(err) {
				t.Fatalf("expected 403, got %v", err)
			}
			if len(repo.applied) != 0 {
				t.Error("a refused write reached the repository")
			}
		})
	}
}

// One unexplored hex in a scribe's batch refuses the whole batch, so a partly
// allowed stroke cannot be used to probe which hexes are explored.
func TestPatchCells_FogRefusesWholeBatch(t *testing.T) {
	repo := newFakeHexRepo()
	repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true}
	repo.cells[HexKey{1, 1}] = HexCell{Col: 1, Row: 1, Explored: true}
	_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorScribe,
		[]UpdateHexCellInput{paint(1, 1, "forest"), paint(2, 2, "hills")})
	if !isForbidden(err) || len(repo.applied) != 0 {
		t.Fatalf("expected 403 and no write, got %v (%d writes)", err, len(repo.applied))
	}
}

func TestPatchCells_CrossCampaignMapIsNotFound(t *testing.T) {
	repo := newFakeHexRepo()
	svc := hexSvc(repo, DrawWhoScribes)
	_, err := svc.PatchCells(context.Background(), "camp-OTHER", "map-1", actorOwner, []UpdateHexCellInput{paint(0, 0, "forest")})
	if !isHexNotFound(err) {
		t.Errorf("PatchCells on another campaign's map = %v, want 404", err)
	}
	if _, err := svc.GetLayer(context.Background(), "camp-OTHER", "map-1", permissions.RoleOwner); !isHexNotFound(err) {
		t.Errorf("GetLayer on another campaign's map = %v, want 404", err)
	}
	if len(repo.applied) != 0 {
		t.Error("a cross-campaign write reached the repository")
	}
}

func TestPatchCells_FailsClosedWithoutLookups(t *testing.T) {
	repo := newFakeHexRepo()
	if _, err := NewHexService(repo).PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
		[]UpdateHexCellInput{paint(0, 0, "forest")}); err == nil || len(repo.applied) != 0 {
		t.Error("an unwired map lookup must refuse, not trust the path")
	}

	s := NewHexService(repo)
	s.SetMapLookup(func(context.Context, string) (string, error) { return "camp-1", nil })
	s.SetDrawPolicyLookup(func(context.Context, string) (string, error) { return "", errors.New("db down") })
	if _, err := s.PatchCells(context.Background(), "camp-1", "map-1", actorScribe,
		[]UpdateHexCellInput{paint(0, 0, "forest")}); err == nil || len(repo.applied) != 0 {
		t.Error("a failing draw-policy lookup must refuse a scribe")
	}
}

func TestPatchCells_BatchCap(t *testing.T) {
	build := func(n int) []UpdateHexCellInput {
		out := make([]UpdateHexCellInput, n)
		for i := range out {
			out[i] = paint(i%100, i/100, "plains")
		}
		return out
	}
	tests := []struct {
		name string
		n    int
		ok   bool
	}{
		{"empty batch", 0, false},
		{"one", 1, true},
		{"exactly the cap", MaxHexBatch, true},
		{"one over the cap", MaxHexBatch + 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			res, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, build(tc.n))
			if tc.ok && (err != nil || res.Updated != tc.n) {
				t.Fatalf("expected %d updated, got %v %v", tc.n, res, err)
			}
			if !tc.ok && (!isBadRequest(err) || len(repo.applied) != 0) {
				t.Fatalf("expected 400 and no write, got %v", err)
			}
		})
	}
}

func TestPatchCells_Validation(t *testing.T) {
	long := strings.Repeat("é", MaxHexNameRunes) // multi-byte: the cap counts runes
	tests := []struct {
		name  string
		entry UpdateHexCellInput
		ok    bool
	}{
		{"every terrain is allowed: plains", paint(0, 0, "plains"), true},
		{"every terrain is allowed: road", paint(0, 0, "road"), true},
		{"unknown terrain", paint(0, 0, "lava"), false},
		{"empty terrain string is not a clear", paint(0, 0, ""), false},
		{"terrain is case sensitive", paint(0, 0, "Forest"), false},
		{"null terrain clears", UpdateHexCellInput{Terrain: patchNull()}, true},
		{"negative col", paint(-1, 0, "forest"), false},
		{"negative row", paint(0, -1, "forest"), false},
		{"col at the cap", paint(MaxHexCoord, 0, "forest"), true},
		{"col over the cap", paint(MaxHexCoord+1, 0, "forest"), false},
		{"row over the cap", paint(0, MaxHexCoord+1, "forest"), false},
		{"name at the cap in runes", UpdateHexCellInput{Name: patchOf(long)}, true},
		{"name one rune over the cap", UpdateHexCellInput{Name: patchOf(long + "é")}, false},
		{"name with a newline", UpdateHexCellInput{Name: patchOf("a\nb")}, false},
		{"name with NUL", UpdateHexCellInput{Name: patchOf("a\x00b")}, false},
		{"name with invalid UTF-8", UpdateHexCellInput{Name: patchOf("a\xffb")}, false},
		{"markup in a name is stored as text", UpdateHexCellInput{Name: patchOf("<b>Keep</b>")}, true},
		{"notes at the cap", UpdateHexCellInput{Notes: patchOf(strings.Repeat("n", MaxHexNotesRunes))}, true},
		{"notes over the cap", UpdateHexCellInput{Notes: patchOf(strings.Repeat("n", MaxHexNotesRunes+1))}, false},
		{"notes keep newlines", UpdateHexCellInput{Notes: patchOf("one\ntwo\tthree")}, true},
		{"notes with a control character", UpdateHexCellInput{Notes: patchOf("a\x07b")}, false},
		{"null name and notes clear", UpdateHexCellInput{Name: patchNull(), Notes: patchNull()}, true},
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

// The service must hand the repository exactly what the caller named: an
// absent field is not written, an explicit null clears, a value replaces.
func TestPatchCells_PartialUpdateContract(t *testing.T) {
	tests := []struct {
		name  string
		entry UpdateHexCellInput
		want  HexCellWrite
	}{
		{"terrain only leaves name and notes alone",
			UpdateHexCellInput{Col: 2, Row: 3, Terrain: patchOf("forest")},
			HexCellWrite{Col: 2, Row: 3, TerrainSet: true, Terrain: strp("forest")}},
		{"name only leaves terrain alone",
			UpdateHexCellInput{Col: 2, Row: 3, Name: patchOf("Port")},
			HexCellWrite{Col: 2, Row: 3, NameSet: true, Name: "Port"}},
		{"null terrain clears it",
			UpdateHexCellInput{Col: 2, Row: 3, Terrain: patchNull()},
			HexCellWrite{Col: 2, Row: 3, TerrainSet: true}},
		{"null name clears it",
			UpdateHexCellInput{Col: 2, Row: 3, Name: patchNull()},
			HexCellWrite{Col: 2, Row: 3, NameSet: true}},
		{"null notes clears them",
			UpdateHexCellInput{Col: 2, Row: 3, Notes: patchNull()},
			HexCellWrite{Col: 2, Row: 3, NotesSet: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			if _, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
				[]UpdateHexCellInput{tc.entry}); err != nil {
				t.Fatal(err)
			}
			got := repo.applied[0][0]
			if got.Col != tc.want.Col || got.Row != tc.want.Row ||
				got.TerrainSet != tc.want.TerrainSet || got.NameSet != tc.want.NameSet || got.NotesSet != tc.want.NotesSet ||
				got.Name != tc.want.Name || !strPtrEq(got.Terrain, tc.want.Terrain) || !strPtrEq(got.Notes, tc.want.Notes) {
				t.Errorf("write = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func strPtrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestPatchCells_DuplicateEntriesFold(t *testing.T) {
	repo := newFakeHexRepo()
	_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, []UpdateHexCellInput{
		{Col: 1, Row: 1, Terrain: patchOf("forest"), Name: patchOf("First")},
		{Col: 1, Row: 1, Terrain: patchOf("hills")},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := repo.applied[0]
	if len(w) != 1 || *w[0].Terrain != "hills" || w[0].Name != "First" || !w[0].NameSet {
		t.Errorf("folded write = %+v, want hills terrain keeping the first entry's name", w)
	}
}

// The cap counts only hexes the map does not store yet, so a full map can
// still be edited and cleared but not grown.
func TestPatchCells_CellCap(t *testing.T) {
	tests := []struct {
		name   string
		count  int
		stored []HexKey
		entry  UpdateHexCellInput
		ok     bool
	}{
		{"new hex on a full map is refused", MaxHexCellsPerMap, nil, paint(0, 0, "forest"), false},
		{"edit of a stored hex on a full map works", MaxHexCellsPerMap, []HexKey{{0, 0}}, paint(0, 0, "forest"), true},
		{"clear of a stored hex on a full map works", MaxHexCellsPerMap, []HexKey{{0, 0}}, UpdateHexCellInput{Terrain: patchNull()}, true},
		{"new hex one below the cap works", MaxHexCellsPerMap - 1, nil, paint(0, 0, "forest"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			repo.count = tc.count
			for _, k := range tc.stored {
				repo.cells[k] = HexCell{Col: k.Col, Row: k.Row}
			}
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

// A batch mixing stored and new hexes counts only the new ones against the cap.
func TestPatchCells_CellCapCountsOnlyNewKeys(t *testing.T) {
	repo := newFakeHexRepo()
	repo.count = MaxHexCellsPerMap - 1
	repo.cells[HexKey{0, 0}] = HexCell{Col: 0, Row: 0}
	entries := []UpdateHexCellInput{paint(0, 0, "forest"), paint(1, 0, "forest")}
	if _, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, entries); err != nil {
		t.Fatalf("one stored plus one new fits in one free slot, got %v", err)
	}
	repo.applied = nil
	entries = append(entries, paint(2, 0, "forest"))
	if _, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, entries); !isBadRequest(err) {
		t.Fatalf("two new hexes need two free slots, got %v", err)
	}
}

func TestPatchCells_RefusesEntryNamingNoField(t *testing.T) {
	repo := newFakeHexRepo()
	_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
		[]UpdateHexCellInput{paint(0, 0, "forest"), {Col: 1, Row: 1}})
	if !isBadRequest(err) || len(repo.applied) != 0 {
		t.Fatalf("an entry with only col and row must be refused whole, got %v", err)
	}
}

func TestGetLayer_DefaultAndFogFiltering(t *testing.T) {
	repo := newFakeHexRepo()
	svc := hexSvc(repo, DrawWhoScribes)

	v, err := svc.GetLayer(context.Background(), "camp-1", "map-1", permissions.RolePlayer)
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 0 || v.Layer.MilesPerHex != 6 || v.Layer.MilesPerDay != 24 || v.Cells == nil || len(v.Cells) != 0 {
		t.Errorf("empty map view = %+v", v)
	}

	repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, Version: 7}
	repo.cells[HexKey{0, 0}] = HexCell{Col: 0, Row: 0, Explored: true}
	repo.cells[HexKey{1, 0}] = HexCell{Col: 1, Row: 0, Explored: false, Name: "secret"}
	for role, want := range map[int]int{
		permissions.RoleNone: 1, permissions.RolePlayer: 1, permissions.RoleScribe: 1, permissions.RoleOwner: 2,
	} {
		v, err := svc.GetLayer(context.Background(), "camp-1", "map-1", role)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Cells) != want || v.Version != 7 {
			t.Errorf("role %d got %d cells at version %d, want %d at 7", role, len(v.Cells), v.Version, want)
		}
	}
}

// --- Handler: role mapping from the campaign context ---

// hexRequest runs a handler and reports the HTTP status the app's error
// handler would send: the recorder's code on success, the AppError's code on
// failure.
func hexRequest(t *testing.T, h *HexHandler, method, body string, role campaigns.Role, dmGranted bool, fn func(echo.Context) error) *hexResult {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/camp-1/maps/map-1/hexes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "mid")
	c.SetParamValues("camp-1", "map-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dmGranted,
	})
	res := &hexResult{ResponseRecorder: rec}
	if err := fn(c); err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) {
			t.Fatalf("handler returned a raw error: %v", err)
		}
		res.status = ae.Code
		return res
	}
	res.status = rec.Code
	return res
}

type hexResult struct {
	*httptest.ResponseRecorder
	status int
}

func TestHexHandler_PatchRoles(t *testing.T) {
	body := `{"cells":[{"col":1,"row":2,"terrain":"forest"}]}`
	tests := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		policy    string
		want      int
	}{
		{"owner", campaigns.RoleOwner, false, DrawWhoOwners, http.StatusOK},
		{"DM-granted player", campaigns.RolePlayer, true, DrawWhoOwners, http.StatusOK},
		{"scribe, scribes may draw", campaigns.RoleScribe, false, DrawWhoScribes, http.StatusOK},
		{"scribe, owners only", campaigns.RoleScribe, false, DrawWhoOwners, http.StatusForbidden},
		{"player", campaigns.RolePlayer, false, DrawWhoScribes, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			h := NewHexHandler(hexSvc(repo, tc.policy))
			rec := hexRequest(t, h, http.MethodPatch, body, tc.role, tc.dmGranted, h.PatchHexCells)
			if rec.status != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.status, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusOK {
				var res HexWriteResult
				if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Version != 1 || res.Updated != 1 {
					t.Errorf("response = %s", rec.Body.String())
				}
			}
		})
	}
}

func TestHexHandler_PatchBodyErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not json", `nope`},
		{"missing col", `{"cells":[{"row":1,"terrain":"forest"}]}`},
		{"missing row", `{"cells":[{"col":1,"terrain":"forest"}]}`},
		{"no cells key", `{}`},
		{"terrain of the wrong type", `{"cells":[{"col":1,"row":1,"terrain":5}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			h := NewHexHandler(hexSvc(repo, DrawWhoScribes))
			rec := hexRequest(t, h, http.MethodPatch, tc.body, campaigns.RoleOwner, false, h.PatchHexCells)
			if rec.status != http.StatusBadRequest || len(repo.applied) != 0 {
				t.Errorf("status = %d, writes = %d, want 400 and none", rec.status, len(repo.applied))
			}
		})
	}
}

// Absent and null must survive the JSON bind: this is the wire half of the
// partial-update contract.
func TestHexHandler_PatchBindsAbsentAndNull(t *testing.T) {
	repo := newFakeHexRepo()
	h := NewHexHandler(hexSvc(repo, DrawWhoScribes))
	rec := hexRequest(t, h, http.MethodPatch,
		`{"cells":[{"col":1,"row":1,"terrain":"hills"},{"col":2,"row":2,"name":null},{"col":3,"row":3,"notes":"x"}]}`,
		campaigns.RoleOwner, false, h.PatchHexCells)
	if rec.status != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.status, rec.Body.String())
	}
	w := repo.applied[0]
	if !w[0].TerrainSet || w[0].NameSet || w[0].NotesSet {
		t.Errorf("entry 0 = %+v: only terrain was sent", w[0])
	}
	if !w[1].NameSet || w[1].Name != "" || w[1].TerrainSet {
		t.Errorf("entry 1 = %+v: name null must clear and nothing else change", w[1])
	}
	if !w[2].NotesSet || w[2].Notes == nil || *w[2].Notes != "x" || w[2].NameSet {
		t.Errorf("entry 2 = %+v: only notes was sent", w[2])
	}
}

func TestHexHandler_GetUsesVisibilityRole(t *testing.T) {
	repo := newFakeHexRepo()
	repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, Version: 3}
	repo.cells[HexKey{0, 0}] = HexCell{Col: 0, Row: 0, Explored: true}
	repo.cells[HexKey{1, 0}] = HexCell{Col: 1, Row: 0, Explored: false, Name: "secret"}
	h := NewHexHandler(hexSvc(repo, DrawWhoScribes))
	tests := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		want      int
	}{
		{"owner", campaigns.RoleOwner, false, 2},
		{"DM-granted player is promoted", campaigns.RolePlayer, true, 2},
		{"scribe", campaigns.RoleScribe, false, 1},
		{"player", campaigns.RolePlayer, false, 1},
		{"public visitor", campaigns.RoleNone, false, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := hexRequest(t, h, http.MethodGet, "", tc.role, tc.dmGranted, h.GetHexes)
			if rec.status != http.StatusOK {
				t.Fatalf("status = %d", rec.status)
			}
			var v struct {
				Cells   []map[string]any `json:"cells"`
				Version uint64           `json:"version"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
				t.Fatal(err)
			}
			if len(v.Cells) != tc.want || v.Version != 3 {
				t.Errorf("got %d cells at version %d, want %d at 3", len(v.Cells), v.Version, tc.want)
			}
			if tc.want == 1 && strings.Contains(rec.Body.String(), "secret") {
				t.Error("an unexplored hex's name is in the response")
			}
		})
	}
}

func patchOf(s string) patch.Field[string] { return patch.Of(s) }
func patchNull() patch.Field[string]       { return patch.Null[string]() }

// CanPaintHexes is what the page offers; it must agree with requireWriter.
func TestMapViewData_CanPaintHexes(t *testing.T) {
	tests := []struct {
		name string
		data MapViewData
		want bool
	}{
		{"owner, scribes policy", MapViewData{IsScribe: true, IsOwner: true, IsDM: true}, true},
		{"owner, owners policy", MapViewData{IsScribe: true, IsOwner: true, IsDM: true, Display: ResolvedDisplay{Frame: "x", DrawWho: DrawWhoOwners}}, true},
		{"DM-granted player, owners policy", MapViewData{IsDM: true, Display: ResolvedDisplay{Frame: "x", DrawWho: DrawWhoOwners}}, true},
		{"DM-granted player, scribes policy", MapViewData{IsDM: true}, true},
		{"scribe, scribes policy", MapViewData{IsScribe: true, Display: ResolvedDisplay{Frame: "x", DrawWho: DrawWhoScribes}}, true},
		{"scribe, owners policy", MapViewData{IsScribe: true, Display: ResolvedDisplay{Frame: "x", DrawWho: DrawWhoOwners}}, false},
		{"player", MapViewData{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.data.CanPaintHexes(); got != tc.want {
				t.Errorf("CanPaintHexes = %v, want %v", got, tc.want)
			}
		})
	}
}

// CanMoveParty is what the page offers; it must agree with requirePartyMover.
func TestMapViewData_CanMoveParty(t *testing.T) {
	owners := ResolvedDisplay{Frame: "x", PartyWho: PartyWhoOwners}
	scribes := ResolvedDisplay{Frame: "x", PartyWho: PartyWhoScribes}
	tests := []struct {
		name string
		data MapViewData
		want bool
	}{
		{"owner, owners policy", MapViewData{IsScribe: true, IsOwner: true, IsDM: true, Display: owners}, true},
		{"DM-granted player, owners policy", MapViewData{IsDM: true, Display: owners}, true},
		{"scribe, scribes policy", MapViewData{IsScribe: true, Display: scribes}, true},
		{"scribe, owners policy", MapViewData{IsScribe: true, Display: owners}, false},
		{"player, scribes policy", MapViewData{Display: scribes}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.data.CanMoveParty(); got != tc.want {
				t.Errorf("CanMoveParty = %v, want %v", got, tc.want)
			}
		})
	}
}

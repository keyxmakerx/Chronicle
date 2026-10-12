package pageblocks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	camp  = "camp-1"
	other = "camp-2"
	page  = "page-1"
	blk   = "blk-a"
)

var (
	dm     = Viewer{UserID: "dm", MemberRole: 3, VisibilityRole: 3}
	coDM   = Viewer{UserID: "co", MemberRole: 2, VisibilityRole: 3}
	scribe = Viewer{UserID: "scr", MemberRole: 2, VisibilityRole: 2}
	player = Viewer{UserID: "pl", MemberRole: 1, VisibilityRole: 1}
	guest  = Viewer{}
)

func newEnv() (Service, *fakeRepo) {
	ents := &fakeEntities{pages: map[string]fakePage{
		page:      {campaign: camp, editable: true},
		"secret":  {campaign: camp, hidden: true, editable: true},
		"locked":  {campaign: camp, editable: false},
		"foreign": {campaign: other, editable: true},
	}}
	repo := newFakeRepo()
	return NewService(repo, ents), repo
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

func ver(n int) *int { return &n }

func put(t *testing.T, svc Service, eid string, v Viewer, kind, data string, version int) (*View, error) {
	t.Helper()
	return svc.Put(context.Background(), camp, eid, blk, v, SaveInput{Kind: kind, Data: json.RawMessage(data), Version: ver(version)})
}

const tableWithDM = `{"columns":[{"id":"c1","label":"Roll","dmOnly":false},{"id":"c2","label":"Secret","dmOnly":true}],
 "rows":[{"id":"r1","cells":{"c1":"1","c2":"trapdoor"}}]}`

func TestGetEmptyDefaults(t *testing.T) {
	svc, _ := newEnv()
	tests := []struct {
		kind    string
		wantErr int
		want    string
	}{
		{KindReadAloud, 0, `{"text":""}`},
		{KindTable, 0, `{"columns":[],"rows":[]}`},
		{KindSteps, 0, `{"steps":[]}`},
		{"nonsense", 422, ""},
		{"", 422, ""},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			v, err := svc.Get(context.Background(), camp, page, blk, tc.kind, player)
			if code(err) != tc.wantErr {
				t.Fatalf("code = %d (%v), want %d", code(err), err, tc.wantErr)
			}
			if err != nil {
				return
			}
			b, _ := json.Marshal(v.Data)
			if string(b) != tc.want || v.Version != 0 || v.Kind != tc.kind {
				t.Fatalf("got %s v%d kind %s", b, v.Version, v.Kind)
			}
			if v.CanEdit {
				t.Fatal("a plain player must not get edit")
			}
		})
	}
}

func TestHiddenAndForeignPagesAre404(t *testing.T) {
	svc, _ := newEnv()
	ctx := context.Background()
	for _, eid := range []string{"secret", "foreign", "missing"} {
		if _, err := svc.Get(ctx, camp, eid, blk, KindSteps, player); code(err) != 404 {
			t.Errorf("Get %s: code %d, want 404", eid, code(err))
		}
		// Even a scribe who could edit a visible page gets 404, not 403.
		if _, err := put(t, svc, eid, scribe, KindSteps, `{"steps":[]}`, 0); code(err) != 404 {
			t.Errorf("Put %s: code %d, want 404", eid, code(err))
		}
	}
	// The DM team sees the hidden page.
	if _, err := svc.Get(ctx, camp, "secret", blk, KindSteps, dm); err != nil {
		t.Errorf("owner on hidden page: %v", err)
	}
	// A malformed block id is not found, whatever the page.
	for _, bad := range []string{"", "a/b", "a b", strings.Repeat("x", 65), "<x>"} {
		if _, err := svc.Get(ctx, camp, page, bad, KindSteps, dm); code(err) != 404 {
			t.Errorf("block id %q: code %d, want 404", bad, code(err))
		}
	}
}

func TestPutPermissions(t *testing.T) {
	svc, repo := newEnv()
	tests := []struct {
		name string
		eid  string
		v    Viewer
		want int
	}{
		{"owner", page, dm, 0},
		{"co-DM", page, coDM, 0},
		{"scribe", page, scribe, 0},
		{"player", page, player, 403},
		{"guest", page, guest, 403},
		{"page the edit rule refuses", "locked", scribe, 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Put(context.Background(), camp, tc.eid, blk+tc.name[:1], tc.v,
				SaveInput{Kind: KindReadAloud, Data: json.RawMessage(`{"text":"hi"}`), Version: ver(0)})
			if code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
		})
	}
	if repo.saves != 3 {
		t.Fatalf("saves = %d, want 3 (refused writes must not reach the repository)", repo.saves)
	}
}

func TestPutVersionAndKind(t *testing.T) {
	svc, _ := newEnv()
	if _, err := put(t, svc, page, dm, KindReadAloud, `{"text":"one"}`, 0); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		kind    string
		version *int
		want    int
	}{
		{"stale version", KindReadAloud, ver(0), 409},
		{"future version", KindReadAloud, ver(5), 409},
		{"missing version", KindReadAloud, nil, 422},
		{"negative version", KindReadAloud, ver(-1), 422},
		{"kind differs from the stored row", KindSteps, ver(1), 422},
		{"unknown kind", "bogus", ver(1), 422},
		{"current version wins", KindReadAloud, ver(1), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Put(context.Background(), camp, page, blk, dm,
				SaveInput{Kind: tc.kind, Data: json.RawMessage(`{"text":"two"}`), Version: tc.version})
			if code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
		})
	}
	v, _ := svc.Get(context.Background(), camp, page, blk, "", dm)
	if v.Version != 2 {
		t.Fatalf("version = %d, want 2 after two saves", v.Version)
	}
}

func TestSavedKindWinsOverHint(t *testing.T) {
	svc, _ := newEnv()
	if _, err := put(t, svc, page, dm, KindSteps, `{"steps":[{"id":"s1","title":"Enter","text":"","done":false}]}`, 0); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(context.Background(), camp, page, blk, KindReadAloud, dm)
	if err != nil || v.Kind != KindSteps {
		t.Fatalf("kind = %v err = %v, want the stored steps", v, err)
	}
}

func TestPutValidation(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	manyCols := func(n int) string {
		var cs []string
		for i := 0; i < n; i++ {
			cs = append(cs, `{"id":"c`+string(rune('a'+i))+`","label":"x","dmOnly":false}`)
		}
		return `{"columns":[` + strings.Join(cs, ",") + `],"rows":[]}`
	}
	manyRows := func(n int) string {
		var rs []string
		for i := 0; i < n; i++ {
			rs = append(rs, `{"id":"r`+strings.Repeat("x", i%5)+string(rune('A'+i/5))+string(rune('a'+i%26))+`","cells":{}}`)
		}
		return `{"columns":[{"id":"c1","label":"x","dmOnly":false}],"rows":[` + strings.Join(rs, ",") + `]}`
	}
	manySteps := func(n int) string {
		var ss []string
		for i := 0; i < n; i++ {
			ss = append(ss, `{"id":"s`+string(rune('a'+i%26))+string(rune('a'+i/26))+`","title":"t","text":"","done":false}`)
		}
		return `{"steps":[` + strings.Join(ss, ",") + `]}`
	}
	tests := []struct {
		name string
		kind string
		data string
		want int
	}{
		{"read-aloud at limit", KindReadAloud, `{"text":"` + long(MaxReadAloudChars) + `"}`, 0},
		{"read-aloud over limit", KindReadAloud, `{"text":"` + long(MaxReadAloudChars+1) + `"}`, 422},
		{"read-aloud keeps line breaks", KindReadAloud, `{"text":"a\nb\r\nc"}`, 0},
		{"read-aloud rejects control char", KindReadAloud, `{"text":"a\u0007b"}`, 422},
		{"read-aloud unknown field", KindReadAloud, `{"text":"a","html":"<b>"}`, 422},
		{"read-aloud empty body", KindReadAloud, ``, 422},
		{"table ok", KindTable, tableWithDM, 0},
		{"table zero columns", KindTable, `{"columns":[],"rows":[]}`, 422},
		{"table 8 columns", KindTable, manyCols(8), 0},
		{"table 9 columns", KindTable, manyCols(9), 422},
		{"table 60 rows", KindTable, manyRows(60), 0},
		{"table 61 rows", KindTable, manyRows(61), 422},
		{"table label over limit", KindTable, `{"columns":[{"id":"c1","label":"` + long(MaxLabelChars+1) + `"}],"rows":[]}`, 422},
		{"table label with line break", KindTable, `{"columns":[{"id":"c1","label":"a\nb"}],"rows":[]}`, 422},
		{"table cell over limit", KindTable, `{"columns":[{"id":"c1","label":"x"}],"rows":[{"id":"r1","cells":{"c1":"` + long(MaxCellChars+1) + `"}}]}`, 422},
		{"table duplicate column id", KindTable, `{"columns":[{"id":"c1","label":"x"},{"id":"c1","label":"y"}],"rows":[]}`, 422},
		{"table unsafe id", KindTable, `{"columns":[{"id":"c 1","label":"x"}],"rows":[]}`, 422},
		{"table duplicate row id", KindTable, `{"columns":[{"id":"c1","label":"x"}],"rows":[{"id":"r1","cells":{}},{"id":"r1","cells":{}}]}`, 422},
		{"steps ok", KindSteps, `{"steps":[{"id":"s1","title":"Enter","text":"Door","done":true}]}`, 0},
		{"steps 30", KindSteps, manySteps(30), 0},
		{"steps 31", KindSteps, manySteps(31), 422},
		{"step title over limit", KindSteps, `{"steps":[{"id":"s1","title":"` + long(MaxStepTitleChars+1) + `","text":"","done":false}]}`, 422},
		{"step text over limit", KindSteps, `{"steps":[{"id":"s1","title":"","text":"` + long(MaxStepTextChars+1) + `","done":false}]}`, 422},
		{"step duplicate id", KindSteps, `{"steps":[{"id":"s1","title":"","text":""},{"id":"s1","title":"","text":""}]}`, 422},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newEnv()
			_, err := put(t, svc, page, dm, tc.kind, tc.data, 0)
			if code(err) != tc.want {
				t.Fatalf("code %d (%v), want %d", code(err), err, tc.want)
			}
		})
	}
}

func TestPutCleansDocument(t *testing.T) {
	svc, repo := newEnv()
	_, err := put(t, svc, page, dm, KindTable,
		`{"columns":[{"id":"c1","label":"A"}],"rows":[{"id":"r1","cells":{"c1":"x\r\ny","gone":"orphan","c1b":""}}]}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got Table
	if err := json.Unmarshal(repo.rows[key(camp, page, blk)].Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Rows[0].Cells["c1"] != "x\ny" || len(got.Rows[0].Cells) != 1 {
		t.Fatalf("cells = %v, want only c1 with normalised line break", got.Rows[0].Cells)
	}
}

func TestTableDMOnlyColumnsAreStrippedBelowOwnerVisibility(t *testing.T) {
	svc, _ := newEnv()
	if _, err := put(t, svc, page, dm, KindTable, tableWithDM, 0); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		v        Viewer
		wantCols int
		canEdit  bool
	}{
		{"owner sees all", dm, 2, true},
		{"co-DM sees all", coDM, 2, true},
		{"scribe sees public columns only and cannot edit", scribe, 1, false},
		{"player sees public columns only", player, 1, false},
		{"guest sees public columns only", guest, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := svc.Get(context.Background(), camp, page, blk, KindTable, tc.v)
			if err != nil {
				t.Fatal(err)
			}
			tbl := v.Data.(Table)
			if len(tbl.Columns) != tc.wantCols || v.CanEdit != tc.canEdit {
				t.Fatalf("columns=%d canEdit=%v, want %d/%v", len(tbl.Columns), v.CanEdit, tc.wantCols, tc.canEdit)
			}
			raw, _ := json.Marshal(v)
			if tc.wantCols == 1 && (strings.Contains(string(raw), "trapdoor") || strings.Contains(string(raw), "Secret") || strings.Contains(string(raw), "c2")) {
				t.Fatalf("DM-only content leaked: %s", raw)
			}
		})
	}
}

func TestDMOnlySaveRule(t *testing.T) {
	svc, repo := newEnv()
	if _, err := put(t, svc, page, dm, KindTable, tableWithDM, 0); err != nil {
		t.Fatal(err)
	}
	before := string(repo.rows[key(camp, page, blk)].Data)

	// A scribe cannot save over a table that has DM-only columns, whatever
	// they send: the hidden column would be erased.
	_, err := put(t, svc, page, scribe, KindTable, `{"columns":[{"id":"c1","label":"Roll"}],"rows":[]}`, 1)
	if code(err) != 403 {
		t.Fatalf("scribe over a DM-only table: code %d, want 403", code(err))
	}
	if string(repo.rows[key(camp, page, blk)].Data) != before {
		t.Fatal("refused save changed the stored table")
	}

	// A scribe cannot create a DM-only column.
	svc2, _ := newEnv()
	_, err = put(t, svc2, page, scribe, KindTable, tableWithDM, 0)
	if code(err) != 403 {
		t.Fatalf("scribe marking a column DM-only: code %d, want 403", code(err))
	}

	// A scribe can edit a table with no DM-only columns.
	_, err = put(t, svc2, page, scribe, KindTable, `{"columns":[{"id":"c1","label":"Roll","dmOnly":false}],"rows":[]}`, 0)
	if err != nil {
		t.Fatalf("scribe on a plain table: %v", err)
	}

	// The DM team can still edit a table with DM-only columns, and the
	// response carries them.
	v, err := put(t, svc, page, coDM, KindTable, tableWithDM, 1)
	if err != nil || len(v.Data.(Table).Columns) != 2 || v.Version != 2 {
		t.Fatalf("co-DM save: %v %+v", err, v)
	}
}

func TestStepsTickRoundTrip(t *testing.T) {
	svc, _ := newEnv()
	if _, err := put(t, svc, page, scribe, KindSteps, `{"steps":[{"id":"s1","title":"Enter","text":"","done":false},{"id":"s2","title":"Fight","text":"","done":false}]}`, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := put(t, svc, page, scribe, KindSteps, `{"steps":[{"id":"s1","title":"Enter","text":"","done":true},{"id":"s2","title":"Fight","text":"","done":false}]}`, 1); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(context.Background(), camp, page, blk, KindSteps, player)
	st := v.Data.(Steps)
	if !st.Steps[0].Done || st.Steps[1].Done || v.CanEdit {
		t.Fatalf("got %+v canEdit=%v", st, v.CanEdit)
	}
}

func TestSameBlockIDOnTwoPagesIsSeparate(t *testing.T) {
	svc, _ := newEnv()
	svc.(*service).entities.(*fakeEntities).pages["page-2"] = fakePage{campaign: camp, editable: true}
	if _, err := put(t, svc, page, dm, KindReadAloud, `{"text":"first"}`, 0); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(context.Background(), camp, "page-2", blk, KindReadAloud, dm)
	if err != nil || v.Version != 0 || v.Data.(ReadAloud).Text != "" {
		t.Fatalf("second page leaked the first's block: %+v %v", v, err)
	}
}

package designs

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// fakePkgs ships designs per game system, as the loader would.
type fakePkgs struct {
	items map[string][]PackageItem
	names map[string]string
}

func (f fakePkgs) Package(_ context.Context, systemID string) (string, []PackageItem) {
	return f.names[systemID], f.items[systemID]
}

func pkgWorld() (*service, *memRepo) {
	s, repo, _ := newWorld()
	s.pkgs = fakePkgs{
		names: map[string]string{"drawsteel": "Draw Steel", "dnd5e": "D&D 5e"},
		items: map[string][]PackageItem{
			"drawsteel": {
				{Kind: KindRoomTheme, Name: "Ashgate Forge", Note: "Warm oak", Maker: "Pack Maker", File: "designs/ashgate.json", Data: json.RawMessage(`{"part":"oak"}`)},
				{Kind: "crafting-circle", Name: "Ward", File: "designs/ward.json", Data: json.RawMessage(`{"part":"oak"}`)},
			},
			"dnd5e": {{Kind: KindRoomTheme, Name: "Waterdeep Stall", File: "designs/stall.json", Data: json.RawMessage(`{"part":"silver"}`)}},
		},
	}
	return s, repo
}

func TestPackageGroupIsTheCampaignsOwnSystem(t *testing.T) {
	s, _ := pkgWorld()
	cases := []struct {
		name, campaign, user, kind string
		wantSystem                 string
		wantNames                  []string
	}{
		{"draw steel campaign", "A1", "alice", "", "Draw Steel", []string{"Ashgate Forge", "Ward"}},
		{"dnd campaign sees only its own", "A2", "alice", "", "D&D 5e", []string{"Waterdeep Stall"}},
		{"kind filter", "A1", "alice", KindRoomTheme, "Draw Steel", []string{"Ashgate Forge"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := s.Package(context.Background(), tc.campaign, tc.user, tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, d := range g.Designs {
				got = append(got, d.Name)
				if !strings.HasPrefix(d.ID, "pkg-") {
					t.Errorf("id %q could be read as a library id", d.ID)
				}
			}
			if g.You != "Alice" {
				t.Errorf("you = %q", g.You)
			}
			if g.SystemName != tc.wantSystem || strings.Join(got, ",") != strings.Join(tc.wantNames, ",") {
				t.Fatalf("got %q %v", g.SystemName, got)
			}
		})
	}
}

func TestPackageGroupIsEmptyWithoutASystemOrSource(t *testing.T) {
	s, _ := pkgWorld()
	s.dir.(*fakeDir).campaigns["A3"] = CampaignInfo{ID: "A3", SystemID: ""}
	s.dir.(*fakeDir).owners["A3"] = "alice"
	s.dir.(*fakeDir).campaigns["A4"] = CampaignInfo{ID: "A4", SystemID: "custom:abc"}
	s.dir.(*fakeDir).owners["A4"] = "alice"
	for _, c := range []string{"A3", "A4"} {
		g, err := s.Package(context.Background(), c, "alice", "")
		if err != nil || len(g.Designs) != 0 || g.Designs == nil {
			t.Fatalf("%s: %v %v", c, g, err)
		}
	}
	s.pkgs = nil
	if g, err := s.Package(context.Background(), "A1", "alice", ""); err != nil || len(g.Designs) != 0 {
		t.Fatalf("no source: %v %v", g, err)
	}
}

func TestPackageCallsRefuseNonOwners(t *testing.T) {
	s, _ := pkgWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Mine", "oak")
	pid := packageID(KindRoomTheme, "designs/ashgate.json")
	for _, u := range []string{"bob", "dan", "pat", "stranger", ""} {
		_, err := s.Package(ctx, "A1", u, "")
		wantCode(t, err, http.StatusForbidden)
		_, err = s.CopyPackage(ctx, "A1", u, pid)
		wantCode(t, err, http.StatusForbidden)
		_, err = s.Suggest(ctx, "A1", u, d.ID, SuggestInput{})
		wantCode(t, err, http.StatusForbidden)
	}
}

func TestPackageDesignsAreReadOnly(t *testing.T) {
	s, repo := pkgWorld()
	ctx := context.Background()
	g, _ := s.Package(ctx, "A1", "alice", "")
	pid := g.Designs[0].ID
	_, err := s.Update(ctx, "A1", "alice", pid, UpdateDesignInput{Name: patch.Of("Renamed")})
	wantCode(t, err, http.StatusNotFound)
	wantCode(t, s.Delete(ctx, "A1", "alice", pid), http.StatusNotFound)
	if _, err := s.ShareState(ctx, "A1", "alice", pid); err == nil {
		t.Error("a package design could be shared")
	}
	if n, _ := repo.Count(ctx, "A1"); n != 0 {
		t.Errorf("library has %d rows after refused changes", n)
	}
	if again, _ := s.Package(ctx, "A1", "alice", ""); again.Designs[0].Name != "Ashgate Forge" {
		t.Error("the package changed")
	}
}

func TestSaveAsMyDesignIsIndependent(t *testing.T) {
	s, repo := pkgWorld()
	ctx := context.Background()
	g, _ := s.Package(ctx, "A1", "alice", "")
	cp, err := s.CopyPackage(ctx, "A1", "alice", g.Designs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Origin != OriginPackage || cp.CampaignID != "A1" || cp.SystemID != "drawsteel" || cp.MakerName != "Pack Maker" || cp.OriginRef != "Draw Steel" || cp.ID == g.Designs[0].ID {
		t.Fatalf("copy = %+v", cp)
	}
	// Renaming, then deleting, the copy leaves the package group whole.
	if _, err := s.Update(ctx, "A1", "alice", cp.ID, UpdateDesignInput{Name: patch.Of("Mine now")}); err != nil {
		t.Fatal(err)
	}
	// The package changing later leaves the copy as it was taken.
	s.pkgs.(fakePkgs).items["drawsteel"][0].Data = json.RawMessage(`{"part":"silver"}`)
	got, _ := s.Get(ctx, "A1", "alice", cp.ID)
	if got.Name != "Mine now" || strings.Contains(string(got.Data), "silver") {
		t.Fatalf("copy followed the package: %+v", got)
	}
	if err := s.Delete(ctx, "A1", "alice", cp.ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.Package(ctx, "A1", "alice", ""); len(after.Designs) != 2 {
		t.Error("deleting the copy touched the package")
	}
	if n, _ := repo.Count(ctx, "A1"); n != 0 {
		t.Errorf("rows = %d", n)
	}
}

func TestCopyPackageRefusals(t *testing.T) {
	s, _ := pkgWorld()
	ctx := context.Background()
	other := packageID(KindRoomTheme, "designs/stall.json") // dnd5e's design, from a draw steel campaign
	_, err := s.CopyPackage(ctx, "A1", "alice", other)
	wantCode(t, err, http.StatusNotFound)
	_, err = s.CopyPackage(ctx, "A1", "alice", "pkg-nope")
	wantCode(t, err, http.StatusNotFound)
	// A design whose kind this server no longer knows is refused, not stored.
	_, err = s.CopyPackage(ctx, "A1", "alice", packageID("crafting-circle", "designs/ward.json"))
	wantCode(t, err, http.StatusBadRequest)
}

func TestSuggestExportShape(t *testing.T) {
	s, _ := pkgWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Moonlit Ward / Æ!", "oak")
	if _, err := s.Update(ctx, "A1", "alice", d.ID, UpdateDesignInput{Note: patch.Of("Silver and glass")}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		in        SuggestInput
		wantMaker string
		wantMsg   string
	}{
		{"credited with a note", SuggestInput{Note: " Suits the Director ", Credit: true}, "Alice", "Suits the Director"},
		{"anonymous", SuggestInput{Credit: false}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sg, err := s.Suggest(ctx, "A1", "alice", d.ID, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(sg)
			var back struct {
				Format  string `json:"format"`
				System  string `json:"system"`
				Designs []struct {
					Kind, Name, Note, Maker, File string
				} `json:"designs"`
				Data    map[string]json.RawMessage `json:"data"`
				Message string                     `json:"messageForMaintainer"`
			}
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatal(err)
			}
			if back.Format != SuggestionFormat || back.System != "drawsteel" || len(back.Designs) != 1 {
				t.Fatalf("shape: %s", raw)
			}
			e := back.Designs[0]
			if e.Kind != KindRoomTheme || e.Name != d.Name || e.Note != "Silver and glass" || e.Maker != tc.wantMaker || back.Message != tc.wantMsg {
				t.Fatalf("entry %+v msg %q", e, back.Message)
			}
			// What a maintainer pastes must pass the manifest's own rules: a
			// relative .json path under designs/, and its data present.
			if !strings.HasPrefix(e.File, "designs/") || strings.Contains(e.File, "..") || !strings.HasSuffix(e.File, ".json") || strings.ContainsAny(e.File, " \\") {
				t.Errorf("file %q", e.File)
			}
			if string(back.Data[e.File]) != `{"part":"oak"}` || len(back.Data) != 1 {
				t.Errorf("data = %v", back.Data)
			}
			if sg.Filename != "drawsteel-design-moonlit-ward.json" {
				t.Errorf("filename %q", sg.Filename)
			}
			// Nothing about goods, prices, pictures or the campaign leaks in.
			for _, bad := range []string{"Ashgate", "A1", "campaign", "price"} {
				if strings.Contains(string(raw), bad) {
					t.Errorf("export mentions %q: %s", bad, raw)
				}
			}
		})
	}
}

func TestSuggestRefusals(t *testing.T) {
	s, _ := pkgWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Mine", "oak")
	_, err := s.Suggest(ctx, "A1", "alice", d.ID, SuggestInput{Note: strings.Repeat("a", MaxSuggestNoteLen+1)})
	wantCode(t, err, http.StatusBadRequest)
	if _, err := s.Suggest(ctx, "A1", "alice", d.ID, SuggestInput{Note: strings.Repeat("a", MaxSuggestNoteLen)}); err != nil {
		t.Errorf("note at the cap refused: %v", err)
	}
	// Another campaign's design is not found, as everywhere.
	_, err = s.Suggest(ctx, "A2", "alice", d.ID, SuggestInput{})
	wantCode(t, err, http.StatusNotFound)
	// A campaign with no package maintainer has nobody to send it to.
	s.dir.(*fakeDir).campaigns["A4"] = CampaignInfo{ID: "A4", SystemID: "custom:abc"}
	s.dir.(*fakeDir).owners["A4"] = "alice"
	_, err = s.Suggest(ctx, "A4", "alice", d.ID, SuggestInput{})
	wantCode(t, err, http.StatusBadRequest)
	// Data the kind now refuses is never exported.
	s.repo.(*memRepo).designs[d.ID].Data = json.RawMessage(`{"part":"lava"}`)
	_, err = s.Suggest(ctx, "A1", "alice", d.ID, SuggestInput{})
	wantCode(t, err, http.StatusBadRequest)
}

func TestFileSlug(t *testing.T) {
	long := strings.Repeat("ab ", 30)
	cases := map[string]string{"Moonlit Ward": "moonlit-ward", "  --A/B--  ": "a-b", "Æ!": "design", "": "design"}
	for in, want := range cases {
		if got := fileSlug(in); got != want {
			t.Errorf("fileSlug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := fileSlug(long); len(got) > 40 || strings.HasSuffix(got, "-") {
		t.Errorf("long slug %q", got)
	}
}

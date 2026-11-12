package designs

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// labelKind accepts any object and returns it unchanged, so a design can carry
// free text of its own (a name, a label) for the data checks to read.
type labelKind struct{}

func (labelKind) Validate(raw json.RawMessage) (json.RawMessage, error) { return raw, nil }
func (labelKind) Lacks(context.Context, string, json.RawMessage) ([]string, error) {
	return nil, nil
}

type namesFake map[string]string

func (n namesFake) DisplayName(_ context.Context, id string) (string, error) { return n[id], nil }

func withLabelKind(lib *service) {
	lib.kinds.(*Kinds).Register("label-theme", labelKind{})
}

// libDesign puts a design straight into alice's library, with any data.
func libDesign(t *testing.T, lib *service, id, kind, data string) *Design {
	t.Helper()
	d := &Design{ID: id, CampaignID: "A1", Kind: kind, SystemID: "drawsteel", Name: id, Data: json.RawMessage(data)}
	if err := lib.repo.Insert(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

// A removed or refused entry that is updated waits as resubmitted, and a
// second clean update must not walk it back onto the gallery.
func TestRepublishCarriesPendingForward(t *testing.T) {
	tests := []struct {
		name       string
		decide     string // admin decision made first; "" leaves it visible
		puts       int
		wantStatus string
		wantReason string
	}{
		{"visible stays visible", "", 2, StatusVisible, ""},
		{"removed then one update", DecisionRemove, 1, StatusPending, ReasonResubmitted},
		{"removed then two updates", DecisionRemove, 2, StatusPending, ReasonResubmitted},
		{"removed then five updates", DecisionRemove, 5, StatusPending, ReasonResubmitted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, lib, _, _ := galleryWorld()
			ctx := context.Background()
			m, d := publish(t, g, lib, "Ward", "oak", "")
			if tc.decide != "" {
				if _, err := g.AdminDecide(ctx, "root", m.ID, tc.decide); err != nil {
					t.Fatal(err)
				}
			}
			var got *GalleryMine
			for i := 0; i < tc.puts; i++ {
				var err error
				if got, err = g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{}); err != nil {
					t.Fatal(err)
				}
			}
			if got.Status != tc.wantStatus || (tc.wantReason != "" && !hasReason(t, g, m.ID, tc.wantReason)) {
				t.Fatalf("after %d updates: %+v", tc.puts, got)
			}
		})
	}
}

// An unpublish cannot wipe a review: a waiting or refused entry is kept, and
// publishing the design again meets the same entry, going back to review.
func TestUnpublishAndPublishAgainCannotWipeModeration(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, g *gallery, m *GalleryMine)
		wantKept  bool
		wantAfter string // status after Publish again
	}{
		{"visible is deleted and may be shared again", func(t *testing.T, g *gallery, m *GalleryMine) {}, false, StatusVisible},
		{"rejected is kept", func(t *testing.T, g *gallery, m *GalleryMine) {
			g.repo.(*memGallery).entries[m.ID].Status = StatusPending
			if _, err := g.AdminDecide(context.Background(), "root", m.ID, DecisionReject); err != nil {
				t.Fatal(err)
			}
		}, true, StatusPending},
		{"removed is kept", func(t *testing.T, g *gallery, m *GalleryMine) {
			if _, err := g.AdminDecide(context.Background(), "root", m.ID, DecisionRemove); err != nil {
				t.Fatal(err)
			}
		}, true, StatusPending},
		{"reported three times is kept", func(t *testing.T, g *gallery, m *GalleryMine) {
			g.repo.(*memGallery).entries[m.ID].Status = StatusVisible
			for _, u := range []string{"r1", "r2", "r3"} {
				g.repo.(*memGallery).reports[m.ID] = append(g.repo.(*memGallery).reports[m.ID], GalleryReport{ReporterUserID: u, Reason: ReportSpam})
			}
			_, _ = g.repo.FlagReported(context.Background(), m.ID, []string{ReasonReported})
		}, true, StatusPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, lib, mg, _ := galleryWorld()
			ctx := context.Background()
			m, d := publish(t, g, lib, "Ward", "oak", "")
			tc.setup(t, g, m)
			err := g.Unpublish(ctx, "A1", "alice", m.ID)
			if tc.wantKept {
				wantCode(t, err, http.StatusConflict)
				if e, _ := mg.Get(ctx, m.ID); e == nil {
					t.Fatal("the held entry was deleted")
				}
			} else {
				wantCode(t, err, 0)
			}
			got, err := g.Publish(ctx, "A1", "alice", d.ID, PublishInput{Name: "Ward"})
			if tc.name == "reported three times is kept" {
				wantCode(t, err, http.StatusConflict) // still waiting on the reports
				if e, _ := mg.Get(ctx, m.ID); e.Status == StatusVisible {
					t.Fatal("reports were wiped")
				}
				return
			}
			if err != nil || got.Status != tc.wantAfter {
				t.Fatalf("publish again = %+v, %v", got, err)
			}
			if tc.wantKept && !hasReason(t, g, got.ID, ReasonResubmitted) {
				t.Fatal("the resubmission was not marked")
			}
		})
	}
}

// The same choices an admin refused wait for review from any other library
// design or name, so a removal is not undone by publishing a copy.
func TestRefusedDataIsNotRepublishedFreshly(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	m, _ := publish(t, g, lib, "Ward", "oak", "")
	if _, err := g.AdminDecide(ctx, "root", m.ID, DecisionRemove); err != nil {
		t.Fatal(err)
	}
	other := mustSave(t, lib, "Copy of ward", "oak")
	got, err := g.Publish(ctx, "A1", "alice", other.ID, PublishInput{Name: "Totally new name"})
	if err != nil || got.Status != StatusPending || !hasReason(t, g, got.ID, ReasonResubmitted) {
		t.Fatalf("same data, new design = %+v, %v", got, err)
	}
	// Different data is unaffected.
	fresh := mustSave(t, lib, "Silver", "silver")
	if got, _ := g.Publish(ctx, "A1", "alice", fresh.ID, PublishInput{}); got.Status != StatusVisible {
		t.Fatalf("different data = %s", got.Status)
	}
}

func TestMaturityReadsTextInsideTheData(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"clean labels", `{"name":"Oak","label":"A warm hearth","props":["anvil","lamp"]}`, StatusVisible},
		{"flagged label", `{"label":"free p0rn"}`, StatusPending},
		{"flagged name", `{"name":"s.e.x room"}`, StatusPending},
		{"flagged deep in a list", `{"props":[{"tag":["fine",{"x":"hentai"}]}]}`, StatusPending},
		{"hex colours are not words", `{"wood":"#b00b5","trim":"#ffffff","cloth":"#c0ffee","warmth":0.8}`, StatusVisible},
		{"repeats in data are not spam", `{"wood":"#000000","bar":"=========","label":"aaaaaaaa"}`, StatusVisible},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, lib, _, _ := galleryWorld()
			withLabelKind(lib)
			d := libDesign(t, lib, "d"+string(rune('a'+i)), "label-theme", tc.data)
			m, err := g.Publish(context.Background(), "A1", "alice", d.ID, PublishInput{Name: "Plain name"})
			if err != nil || m.Status != tc.want {
				t.Fatalf("status = %v, %v; want %s", m, err, tc.want)
			}
			if tc.want == StatusPending && !hasReason(t, g, m.ID, ReasonMaturity) {
				t.Fatal("no maturity reason")
			}
		})
	}
	if dataName([]byte(`{"name":" Moon\u0007 Ward "}`)) != "Moon Ward" || dataName([]byte(`[1]`)) != "" {
		t.Fatal("dataName")
	}
}

func TestAdminEntryShowsPublisherAndDataName(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	g.names = namesFake{"alice": "Alice Real-Name"}
	withLabelKind(lib)
	d := libDesign(t, lib, "dd", "label-theme", `{"name":"Inner name","label":"hentai den"}`)
	if _, err := g.Publish(context.Background(), "A1", "alice", d.ID, PublishInput{Name: "Outer", MakerName: "Shown As"}); err != nil {
		t.Fatal(err)
	}
	q, _ := g.AdminQueue(context.Background(), "", 1)
	e := q.Entries[0]
	if e.PublisherName != "Alice Real-Name" || e.MakerName != "Shown As" || e.DataName != "Inner name" || !strings.Contains(e.Summary, "label") {
		t.Fatalf("admin entry = %+v", e)
	}
	g.names = nil
	if q, _ = g.AdminQueue(context.Background(), "", 1); q.Entries[0].PublisherName != "" {
		t.Fatal("a missing resolver must leave the name blank, not fail")
	}
}

// The Not approved filter lists refused and taken-down entries, and either can
// be approved after all.
func TestNotApprovedFilterAllowsUndo(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	a, _ := publish(t, g, lib, "One", "oak", "")
	b := mustSave(t, lib, "Two lib", "silver")
	bm, _ := g.Publish(ctx, "A1", "alice", b.ID, PublishInput{Name: "Two", Note: "see www.x"})
	if _, err := g.AdminDecide(ctx, "root", a.ID, DecisionRemove); err != nil {
		t.Fatal(err)
	}
	if _, err := g.AdminDecide(ctx, "root", bm.ID, DecisionReject); err != nil {
		t.Fatal(err)
	}
	q, err := g.AdminQueue(ctx, StatusRejected, 1)
	if err != nil || q.Total != 2 || q.Status != StatusRejected {
		t.Fatalf("not approved = %+v, %v", q, err)
	}
	for _, id := range []string{a.ID, bm.ID} {
		if e, err := g.AdminDecide(ctx, "root", id, DecisionApprove); err != nil || e.Status != StatusVisible {
			t.Fatalf("approve after all = %+v, %v", e, err)
		}
	}
	if q, _ = g.AdminQueue(ctx, StatusRejected, 1); q.Total != 0 {
		t.Fatal("approved entries still listed")
	}
}

// Pages are clamped, so an absurd offset never reaches the database.
func TestPagesAreClamped(t *testing.T) {
	for in, want := range map[int]int{-5: 1, 0: 1, 1: 1, 500: 500, MaxPage: MaxPage, MaxPage + 1: MaxPage, 1 << 40: MaxPage} {
		if got := clampPage(in); got != want {
			t.Errorf("clampPage(%d) = %d, want %d", in, got, want)
		}
	}
	g, _, _, _ := galleryWorld()
	ctx := context.Background()
	if p, err := g.Browse(ctx, "A1", "alice", BrowseInput{Page: 1 << 30}); err != nil || p.Page != MaxPage {
		t.Fatalf("browse page = %+v, %v", p, err)
	}
	if p, err := g.AdminQueue(ctx, "", 1<<30); err != nil || p.Page != MaxPage {
		t.Fatalf("admin page = %+v, %v", p, err)
	}
}

// An empty shown-as name on an update means the Owner's display name.
func TestRepublishEmptyMakerMeansDisplayName(t *testing.T) {
	tests := []struct {
		name string
		in   RepublishInput
		want string
	}{
		{"absent keeps", RepublishInput{}, "Custom Maker"},
		{"empty means display name", RepublishInput{MakerName: patch.Of("")}, "Alice"},
		{"spaces mean display name", RepublishInput{MakerName: patch.Of("   ")}, "Alice"},
		{"present replaces", RepublishInput{MakerName: patch.Of("New Maker")}, "New Maker"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, lib, _, _ := galleryWorld()
			d := mustSave(t, lib, "Ward", "oak")
			if _, err := g.Publish(context.Background(), "A1", "alice", d.ID, PublishInput{MakerName: "Custom Maker"}); err != nil {
				t.Fatal(err)
			}
			m, err := g.Republish(context.Background(), "A1", "alice", d.ID, tc.in)
			if err != nil || m.MakerName != tc.want {
				t.Fatalf("maker = %v, %v; want %q", m, err, tc.want)
			}
		})
	}
}

// racyGallery lets a test change an entry between the report's read and its
// threshold update, as an admin or a second request would.
type racyGallery struct {
	*memGallery
	onReports func()
}

func (r *racyGallery) Reports(ctx context.Context, id string) ([]GalleryReport, error) {
	if r.onReports != nil {
		r.onReports()
	}
	return r.memGallery.Reports(ctx, id)
}

func TestThirdReportDoesNotOverwriteAConcurrentDecision(t *testing.T) {
	tests := []struct {
		name       string
		concurrent string
		wantStatus string
	}{
		{"still visible is flagged", "", StatusPending},
		{"removed meanwhile stays removed", StatusRemoved, StatusRemoved},
		{"rejected meanwhile stays rejected", StatusRejected, StatusRejected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, lib, mg, dir := galleryWorld()
			ctx := context.Background()
			dir.campaigns["D1"], dir.campaigns["E1"] = CampaignInfo{ID: "D1", SystemID: "drawsteel"}, CampaignInfo{ID: "E1", SystemID: "drawsteel"}
			dir.owners["D1"], dir.owners["E1"] = "dora", "eli"
			m, _ := publish(t, g, lib, "Ward", "oak", "")
			rg := &racyGallery{memGallery: mg}
			g.repo = rg
			rep := func(camp, user string) {
				if err := g.Report(ctx, camp, user, m.ID, ReportInput{Reason: ReportSpam}); err != nil {
					t.Fatal(err)
				}
			}
			rep("B1", "bob")
			rep("D1", "dora")
			if tc.concurrent != "" {
				rg.onReports = func() {
					e := mg.entries[m.ID]
					e.Status, e.ReviewedBy, e.Reasons = tc.concurrent, "admin1", []string{}
				}
			}
			rep("E1", "eli")
			e, _ := mg.Get(ctx, m.ID)
			if e.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s", e.Status, tc.wantStatus)
			}
			if tc.concurrent != "" && (e.ReviewedBy != "admin1" || len(e.Reasons) != 0) {
				t.Fatalf("a stale save overwrote the decision: %+v", e)
			}
		})
	}
}

type duplicateInsertGallery struct{ *memGallery }

func (duplicateInsertGallery) Insert(context.Context, *GalleryEntry) error {
	return ErrGalleryDuplicate
}

// Two publishes of one design racing: the unique key's refusal is a 409.
func TestPublishInsertRaceIsAConflict(t *testing.T) {
	g, lib, mg, _ := galleryWorld()
	g.repo = duplicateInsertGallery{mg}
	d := mustSave(t, lib, "Ward", "oak")
	_, err := g.Publish(context.Background(), "A1", "alice", d.ID, PublishInput{})
	wantCode(t, err, http.StatusConflict)
}

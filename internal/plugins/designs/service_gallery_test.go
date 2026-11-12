package designs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// galleryWorld is newWorld plus a gallery over the same fakes.
func galleryWorld() (*gallery, *service, *memGallery, *fakeDir) {
	lib, repo, dir := newWorld()
	mg := newMemGallery()
	g := &gallery{lib: lib, repo: mg, maturity: defaultMaturity, now: time.Now}
	_ = repo
	return g, lib, mg, dir
}

func publish(t *testing.T, g *gallery, lib *service, name, part, note string) (*GalleryMine, *Design) {
	t.Helper()
	d := mustSave(t, lib, name+" lib", part)
	m, err := g.Publish(context.Background(), "A1", "alice", d.ID, PublishInput{Name: name, Note: note})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return m, d
}

func TestPublishCleanIsVisibleAndSnapshotted(t *testing.T) {
	g, lib, mg, _ := galleryWorld()
	ctx := context.Background()
	m, d := publish(t, g, lib, "Ward", "oak", "A warm ward")
	if m.Status != StatusVisible || len(m.Reasons) != 0 || m.MakerName != "Alice" || m.SystemID != "drawsteel" {
		t.Fatalf("entry = %+v", m)
	}
	// Renaming or deleting the library design leaves the gallery entry as it was.
	if _, err := lib.Update(ctx, "A1", "alice", d.ID, UpdateDesignInput{Name: patch.Of("Changed")}); err != nil {
		t.Fatal(err)
	}
	if err := lib.Delete(ctx, "A1", "alice", d.ID); err != nil {
		t.Fatal(err)
	}
	e, _ := mg.Get(ctx, m.ID)
	if e == nil || e.Name != "Ward" || e.Status != StatusVisible {
		t.Fatalf("entry after library change = %+v", e)
	}
	if _, err := g.Publish(ctx, "A1", "alice", d.ID, PublishInput{}); err == nil {
		t.Fatal("publishing a deleted library design succeeded")
	}
}

func TestPublishLabelRules(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	d := mustSave(t, lib, "Ward", "oak")
	tests := []struct {
		name string
		in   PublishInput
		want int
	}{
		{"name too long", PublishInput{Name: strings.Repeat("x", 61)}, http.StatusBadRequest},
		{"note too long", PublishInput{Name: "Ok", Note: strings.Repeat("n", 141)}, http.StatusBadRequest},
		{"maker too long", PublishInput{Name: "Ok", MakerName: strings.Repeat("m", 101)}, http.StatusBadRequest},
		{"defaults fill in", PublishInput{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := g.Publish(context.Background(), "A1", "alice", d.ID, tc.in)
			wantCode(t, err, tc.want)
		})
	}
}

func TestPublishPublishedTwiceConflicts(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	_, d := publish(t, g, lib, "Ward", "oak", "")
	_, err := g.Publish(context.Background(), "A1", "alice", d.ID, PublishInput{})
	wantCode(t, err, http.StatusConflict)
}

// A flagged entry waits; its publisher reads plain reasons that never include
// the matched word; nobody else, and no browse, sees it; an admin queue does.
func TestFlaggedEntryWaitsAndIsHidden(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	m, _ := publish(t, g, lib, "Fine name", "oak", "free p0rn at http://x.io")
	if m.Status != StatusPending {
		t.Fatalf("status = %s", m.Status)
	}
	for _, r := range m.Reasons {
		if strings.Contains(strings.ToLower(r), "porn") {
			t.Fatalf("a reason names the matched term: %q", r)
		}
	}
	if len(m.Reasons) != 2 {
		t.Fatalf("reasons = %v", m.Reasons)
	}
	own, _ := g.Mine(ctx, "A1", "alice")
	if len(own) != 1 || own[0].Status != StatusPending {
		t.Fatalf("publisher list = %+v", own)
	}
	for _, who := range []struct{ camp, user string }{{"A1", "alice"}, {"B1", "bob"}} {
		page, err := g.Browse(ctx, who.camp, who.user, BrowseInput{AllSystems: true})
		if err != nil || len(page.Entries) != 0 {
			t.Fatalf("%s browsed %d entries, err %v", who.user, len(page.Entries), err)
		}
	}
	if _, err := g.Copy(ctx, "B1", "bob", m.ID); err == nil {
		t.Fatal("copied a pending entry")
	} else {
		wantCode(t, err, http.StatusNotFound)
	}
	if err := g.Report(ctx, "B1", "bob", m.ID, ReportInput{Reason: ReportSpam}); err == nil {
		t.Fatal("reported a pending entry")
	}
	q, _ := g.AdminQueue(ctx, "", 1)
	if q.Total != 1 || q.Entries[0].ReasonCodes[0] != ReasonMaturity {
		t.Fatalf("admin queue = %+v", q)
	}
}

func TestRateLimit(t *testing.T) {
	g, lib, mg, _ := galleryWorld()
	ctx := context.Background()
	// Five distinct designs publish; the sixth in 24 hours waits.
	for i := 0; i < MaxGalleryPerDay; i++ {
		d := &Design{ID: fmt.Sprintf("d%d", i), CampaignID: "A1", Kind: KindRoomTheme, SystemID: "drawsteel", Name: "n", Data: json.RawMessage(`{"part":"oak"}`)}
		_ = lib.repo.Insert(ctx, d)
		m, err := g.Publish(ctx, "A1", "alice", d.ID, PublishInput{Name: fmt.Sprintf("Design %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && m.Status != StatusVisible {
			t.Fatalf("first publish %s", m.Status)
		}
		// Different data per design is not possible with the fake kind, so the
		// duplicate rule fires from the second one on; the rate rule is read apart.
	}
	reasons := textReasons(g.maturity, "x", "", "")
	n, _ := mg.CountEvents(ctx, "alice", time.Now().Add(-24*time.Hour))
	if n != MaxGalleryPerDay || len(reasons) != 0 {
		t.Fatalf("events = %d", n)
	}
	d := &Design{ID: "d9", CampaignID: "A1", Kind: KindRoomTheme, SystemID: "drawsteel", Name: "n", Data: json.RawMessage(`{"part":"silver"}`)}
	_ = lib.repo.Insert(ctx, d)
	m, err := g.Publish(ctx, "A1", "alice", d.ID, PublishInput{Name: "Sixth"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != StatusPending || !hasReason(t, g, m.ID, ReasonRate) {
		t.Fatalf("sixth publish = %+v", m)
	}
	// A day later the window has passed.
	g.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	n, _ = mg.CountEvents(ctx, "alice", g.now().Add(-24*time.Hour))
	if n != 0 {
		t.Fatalf("events after a day = %d", n)
	}
}

func hasReason(t *testing.T, g *gallery, id, code string) bool {
	t.Helper()
	e, _ := g.repo.Get(context.Background(), id)
	for _, r := range e.Reasons {
		if r == code {
			return true
		}
	}
	return false
}

func TestDuplicateDataBySameUser(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	first, _ := publish(t, g, lib, "One", "oak", "")
	if first.Status != StatusVisible {
		t.Fatal("first should be visible")
	}
	second, _ := publish(t, g, lib, "Two", "oak", "")
	if second.Status != StatusPending || !hasReason(t, g, second.ID, ReasonDuplicate) {
		t.Fatalf("second = %+v", second)
	}
	// The same data by another user is not a duplicate.
	d, _ := lib.Save(ctx, "B1", "bob", SaveInput{Kind: KindRoomTheme, Name: "Bob's", Data: json.RawMessage(`{"part":"oak"}`)})
	m, err := g.Publish(ctx, "B1", "bob", d.ID, PublishInput{})
	if err != nil || m.Status != StatusVisible {
		t.Fatalf("bob's = %+v, %v", m, err)
	}
}

func TestEveryGalleryCallRefusesNonOwners(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	m, d := publish(t, g, lib, "Ward", "oak", "")
	calls := map[string]func(u string) error{
		"publish":   func(u string) error { _, err := g.Publish(ctx, "A1", u, d.ID, PublishInput{}); return err },
		"republish": func(u string) error { _, err := g.Republish(ctx, "A1", u, d.ID, RepublishInput{}); return err },
		"unpublish": func(u string) error { return g.Unpublish(ctx, "A1", u, m.ID) },
		"mine":      func(u string) error { _, err := g.Mine(ctx, "A1", u); return err },
		"browse":    func(u string) error { _, err := g.Browse(ctx, "A1", u, BrowseInput{}); return err },
		"copy":      func(u string) error { _, err := g.Copy(ctx, "A1", u, m.ID); return err },
		"report":    func(u string) error { return g.Report(ctx, "A1", u, m.ID, ReportInput{Reason: ReportSpam}) },
	}
	for name, call := range calls {
		for _, u := range []string{"dan", "pat", "stranger", ""} {
			if err := call(u); err == nil {
				t.Errorf("%s by %q succeeded", name, u)
			} else {
				wantCode(t, err, http.StatusForbidden)
			}
		}
	}
	// Bob owns a campaign, but not A1 and not alice's design.
	if err := g.Unpublish(ctx, "B1", "bob", m.ID); err == nil {
		t.Error("bob unpublished alice's entry")
	} else {
		wantCode(t, err, http.StatusNotFound)
	}
	if _, err := g.Publish(ctx, "B1", "bob", d.ID, PublishInput{}); err == nil {
		t.Error("bob published a design from another campaign")
	}
}

func seed(mg *memGallery, id, user, kind, system, name, maker string, status string) {
	_ = mg.Insert(context.Background(), &GalleryEntry{ID: id, PublisherUserID: user, Kind: kind, SystemID: system, Name: name, MakerName: maker,
		Data: json.RawMessage(`{"part":"oak"}`), DataHash: id, Status: status, Reasons: []string{}})
}

func TestBrowseFilters(t *testing.T) {
	g, _, mg, _ := galleryWorld()
	ctx := context.Background()
	seed(mg, "e1", "bob", KindRoomTheme, "drawsteel", "Oak Room", "Bob", StatusVisible)
	seed(mg, "e2", "bob", "crafting-circle", "drawsteel", "Seven Star", "Bob", StatusVisible)
	seed(mg, "e3", "cara", KindRoomTheme, "dnd5e", "Tavern", "Cara", StatusVisible)
	seed(mg, "e4", "cara", "crafting-bench", "dnd5e", "Folding Bench", "Cara", StatusVisible)
	seed(mg, "e5", "cara", KindRoomTheme, "drawsteel", "Hidden", "Cara", StatusPending)
	seed(mg, "e6", "cara", KindRoomTheme, "drawsteel", "Gone", "Cara", StatusRemoved)
	tests := []struct {
		name string
		in   BrowseInput
		want []string
	}{
		{"current system, newest first", BrowseInput{}, []string{"e2", "e1"}},
		{"all systems", BrowseInput{AllSystems: true}, []string{"e4", "e3", "e2", "e1"}},
		{"rooms only", BrowseInput{AllSystems: true, Kind: KindFilterRooms}, []string{"e3", "e1"}},
		{"crafting only", BrowseInput{AllSystems: true, Kind: KindFilterCrafting}, []string{"e4", "e2"}},
		{"search name", BrowseInput{AllSystems: true, Q: "bench"}, []string{"e4"}},
		{"search maker", BrowseInput{AllSystems: true, Q: "cara"}, []string{"e4", "e3"}},
		{"no match", BrowseInput{Q: "zzz"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := g.Browse(ctx, "A1", "alice", tc.in)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, e := range page.Entries {
				got = append(got, e.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") || page.Total != len(tc.want) {
				t.Fatalf("got %v (total %d), want %v", got, page.Total, tc.want)
			}
		})
	}
	_, err := g.Browse(ctx, "A1", "alice", BrowseInput{Kind: "bogus"})
	wantCode(t, err, http.StatusBadRequest)
}

func TestBrowsePaginates(t *testing.T) {
	g, _, mg, _ := galleryWorld()
	for i := 0; i < GalleryPageSize+3; i++ {
		seed(mg, fmt.Sprintf("p%02d", i), "bob", KindRoomTheme, "drawsteel", "Room", "Bob", StatusVisible)
	}
	p1, _ := g.Browse(context.Background(), "A1", "alice", BrowseInput{Page: 1})
	p2, _ := g.Browse(context.Background(), "A1", "alice", BrowseInput{Page: 2})
	if len(p1.Entries) != GalleryPageSize || len(p2.Entries) != 3 || p1.Total != GalleryPageSize+3 || p2.Page != 2 {
		t.Fatalf("pages: %d, %d, total %d", len(p1.Entries), len(p2.Entries), p1.Total)
	}
	if p1.Entries[0].ID == p2.Entries[0].ID {
		t.Fatal("pages overlap")
	}
}

func TestCopyIsIndependentAndShowsLacks(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	m, _ := publish(t, g, lib, "Silver Ward", "silver", "")
	// Bob's campaign lacks silver: the card says so, and the copy still works.
	page, _ := g.Browse(ctx, "B1", "bob", BrowseInput{})
	if len(page.Entries) != 1 || len(page.Entries[0].Lacks) != 1 || !page.Entries[0].SameSystem || page.Entries[0].Mine {
		t.Fatalf("card = %+v", page.Entries)
	}
	cp, err := g.Copy(ctx, "B1", "bob", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cp.CampaignID != "B1" || cp.Origin != OriginGallery || cp.MakerName != "Alice" || cp.Name != "Silver Ward" {
		t.Fatalf("copy = %+v", cp)
	}
	// Unpublishing leaves the copy; the copy is its own row.
	if err := g.Unpublish(ctx, "A1", "alice", m.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := lib.Get(ctx, "B1", "bob", cp.ID); err != nil || got.Name != "Silver Ward" {
		t.Fatalf("copy after unpublish: %+v, %v", got, err)
	}
	if _, err := g.Copy(ctx, "B1", "bob", m.ID); err == nil {
		t.Fatal("copied an unpublished entry")
	}
}

func TestCopyOfAnotherSystemIsAllowed(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	m, _ := publish(t, g, lib, "Ward", "oak", "")
	cp, err := g.Copy(context.Background(), "C1", "cara", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The copy takes the receiving campaign's system, as a code does.
	if cp.SystemID != "dnd5e" {
		t.Fatalf("system = %s", cp.SystemID)
	}
	page, _ := g.Browse(context.Background(), "C1", "cara", BrowseInput{AllSystems: true})
	if page.Entries[0].SameSystem {
		t.Fatal("card should say a different system")
	}
}

func TestReportThreshold(t *testing.T) {
	g, lib, mg, dir := galleryWorld()
	ctx := context.Background()
	dir.campaigns["D1"] = CampaignInfo{ID: "D1", SystemID: "drawsteel"}
	dir.campaigns["E1"] = CampaignInfo{ID: "E1", SystemID: "drawsteel"}
	dir.owners["D1"], dir.owners["E1"] = "dora", "eli"
	m, _ := publish(t, g, lib, "Ward", "oak", "")
	rep := func(camp, user string) error {
		return g.Report(ctx, camp, user, m.ID, ReportInput{Reason: ReportAdult, Note: "bad"})
	}
	if err := rep("B1", "bob"); err != nil {
		t.Fatal(err)
	}
	wantCode(t, rep("B1", "bob"), http.StatusConflict)
	if e, _ := mg.Get(ctx, m.ID); e.Status != StatusVisible {
		t.Fatal("one report must not hide it")
	}
	if err := rep("D1", "dora"); err != nil {
		t.Fatal(err)
	}
	if e, _ := mg.Get(ctx, m.ID); e.Status != StatusVisible {
		t.Fatal("two distinct reports must not hide it")
	}
	if err := rep("E1", "eli"); err != nil {
		t.Fatal(err)
	}
	e, _ := mg.Get(ctx, m.ID)
	if e.Status != StatusPending || len(e.Reasons) != 1 || e.Reasons[0] != ReasonReported {
		t.Fatalf("after three = %+v", e)
	}
	q, _ := g.AdminQueue(ctx, StatusPending, 1)
	if len(q.Entries) != 1 || len(q.Entries[0].Reports) != 3 {
		t.Fatalf("queue = %+v", q)
	}
}

func TestReportRules(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	m, _ := publish(t, g, lib, "Ward", "oak", "")
	tests := []struct {
		name       string
		camp, user string
		in         ReportInput
		id         string
		wantCode   int
	}{
		{"own entry", "A1", "alice", ReportInput{Reason: ReportSpam}, m.ID, http.StatusBadRequest},
		{"bad reason", "B1", "bob", ReportInput{Reason: "rude"}, m.ID, http.StatusBadRequest},
		{"long note", "B1", "bob", ReportInput{Reason: ReportOther, Note: strings.Repeat("x", 141)}, m.ID, http.StatusBadRequest},
		{"missing entry", "B1", "bob", ReportInput{Reason: ReportOther}, "nope", http.StatusNotFound},
		{"fine", "B1", "bob", ReportInput{Reason: ReportOther}, m.ID, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantCode(t, g.Report(ctx, tc.camp, tc.user, tc.id, tc.in), tc.wantCode)
		})
	}
}

func TestRepublish(t *testing.T) {
	g, lib, mg, _ := galleryWorld()
	ctx := context.Background()
	m, d := publish(t, g, lib, "Ward", "oak", "keep this note")

	// Partial: only the name is sent, so the note is kept.
	got, err := g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{Name: patch.Of("Better Ward")})
	if err != nil || got.Name != "Better Ward" || got.Note != "keep this note" || got.Status != StatusVisible {
		t.Fatalf("republish = %+v, %v", got, err)
	}
	// Re-running the checks: the new text is flagged and the entry waits.
	got, _ = g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{Note: patch.Of("see www.spam")})
	if got.Status != StatusPending {
		t.Fatalf("flagged republish = %s", got.Status)
	}
	// Clean again goes visible.
	got, _ = g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{Note: patch.Null[string]()})
	if got.Status != StatusVisible || got.Note != "" {
		t.Fatalf("clean republish = %+v", got)
	}
	// A refused entry is never put straight back on show.
	if _, err := g.AdminDecide(ctx, "root", m.ID, DecisionRemove); err != nil {
		t.Fatal(err)
	}
	got, _ = g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{})
	if got.Status != StatusPending || !hasReason(t, g, m.ID, ReasonResubmitted) {
		t.Fatalf("resubmitted = %+v", got)
	}
	// So does a reported one that is still waiting.
	e, _ := mg.Get(ctx, m.ID)
	e.Status, e.Reasons = StatusPending, []string{ReasonReported}
	_ = mg.Save(ctx, e)
	got, _ = g.Republish(ctx, "A1", "alice", d.ID, RepublishInput{})
	if got.Status != StatusPending {
		t.Fatalf("reported republish = %+v", got)
	}
	// A design that is not in the gallery cannot be updated.
	d2 := mustSave(t, lib, "Other", "silver")
	_, err = g.Republish(ctx, "A1", "alice", d2.ID, RepublishInput{})
	wantCode(t, err, http.StatusNotFound)
}

func TestAdminDecisions(t *testing.T) {
	g, lib, _, _ := galleryWorld()
	ctx := context.Background()
	pending, _ := publish(t, g, lib, "Fine", "oak", "free http://x.io")
	tests := []struct {
		name, decision string
		want           int
		status         string
	}{
		{"unknown decision", "ban", http.StatusBadRequest, ""},
		{"cannot remove a waiting entry", DecisionRemove, http.StatusConflict, ""},
		{"reject", DecisionReject, 0, StatusRejected},
		{"cannot reject twice", DecisionReject, http.StatusConflict, ""},
		{"approve a refused entry", DecisionApprove, 0, StatusVisible},
		{"cannot approve a visible one", DecisionApprove, http.StatusConflict, ""},
		{"remove a visible one", DecisionRemove, 0, StatusRemoved},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, err := g.AdminDecide(ctx, "root", pending.ID, tc.decision)
			wantCode(t, err, tc.want)
			if tc.want == 0 && e.Status != tc.status {
				t.Fatalf("status = %s, want %s", e.Status, tc.status)
			}
		})
	}
	_, err := g.AdminDecide(ctx, "root", "nope", DecisionApprove)
	wantCode(t, err, http.StatusNotFound)
	own, _ := g.Mine(ctx, "A1", "alice")
	if own[0].Status != StatusRemoved || len(own[0].Reasons) == 0 {
		t.Fatalf("publisher sees %+v", own[0])
	}
	_, err = g.AdminQueue(ctx, "removed", 1)
	wantCode(t, err, http.StatusBadRequest)
}

func TestApprovedEntryStartsWithoutReports(t *testing.T) {
	g, lib, mg, _ := galleryWorld()
	ctx := context.Background()
	m, _ := publish(t, g, lib, "Ward", "oak", "")
	_ = g.Report(ctx, "B1", "bob", m.ID, ReportInput{Reason: ReportSpam})
	e, _ := mg.Get(ctx, m.ID)
	e.Status, e.Reasons = StatusPending, []string{ReasonReported}
	_ = mg.Save(ctx, e)
	if _, err := g.AdminDecide(ctx, "root", m.ID, DecisionApprove); err != nil {
		t.Fatal(err)
	}
	if r, _ := mg.Reports(ctx, m.ID); len(r) != 0 {
		t.Fatal("reports survived approval")
	}
	// Bob can report again after the review.
	if err := g.Report(ctx, "B1", "bob", m.ID, ReportInput{Reason: ReportSpam}); err != nil {
		t.Fatal(err)
	}
}

func TestSummarize(t *testing.T) {
	got := summarize(json.RawMessage(`{"name":"x","version":1,"wall":"plaster","props":["a","b"],"theme":{"k":1},"n":3}`))
	if got != "n: 3 · props: 2 · theme: 1 parts · wall: plaster" {
		t.Fatalf("summary = %q", got)
	}
	if summarize(json.RawMessage(`[1]`)) != "" || len([]rune(summarize(json.RawMessage(`{"a":"`+strings.Repeat("z", 400)+`"}`)))) > maxSummaryLen {
		t.Fatal("summary bounds")
	}
}

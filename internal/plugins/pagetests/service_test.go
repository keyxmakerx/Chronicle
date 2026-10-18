package pagetests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	dm     = Viewer{UserID: "u-dm", VisibilityRole: permissions.RoleOwner}
	scribe = Viewer{UserID: "u-scribe", VisibilityRole: permissions.RoleScribe}
	player = Viewer{UserID: "u-player", VisibilityRole: permissions.RolePlayer}
)

func newTestService(cal Calendar) (*service, *memRepo, *fakePages) {
	repo, pages := newMemRepo(), newFakePages()
	s := NewService(repo, pages, cal).(*service)
	s.now = func() time.Time { return time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC) }
	return s, repo, pages
}

func intp(n int) *int { return &n }

func statusOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// lastwell is a montage: five successes before three failures, with a
// result offered by each total and one picked only by hand.
func lastwell() SaveInput {
	return SaveInput{
		Name: "Restore the Lastwell core", Kind: "Montage", Needed: 5, Limit: 3,
		Results: []Result{
			{ID: "win", Label: "Total success", On: OnSuccesses},
			{ID: "part", Label: "Partial success"},
			{ID: "lose", Label: "Total failure", On: OnFailures},
		},
	}
}

func TestCreateAndReadForEachViewer(t *testing.T) {
	svc, _, _ := newTestService(nil)
	ctx := context.Background()
	hidden, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if hidden.Version != 1 || hidden.Kind != "Montage" || len(hidden.Results) != 3 {
		t.Fatalf("unexpected test: %+v", hidden)
	}
	shownIn := lastwell()
	shownIn.Name, shownIn.Shown = "Cross the salt flats", true
	shown, err := svc.Create(ctx, "c1", "page1", dm, shownIn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(ctx, "c1", shown.ID, dm, RecordInput{Result: "part", Version: intp(shown.Version)}); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListForPage(ctx, "c1", "page1", dm)
	if err != nil || len(list.Tests) != 2 || !list.CanEdit {
		t.Fatalf("DM list: %+v %v", list, err)
	}
	for _, v := range []Viewer{scribe, player} {
		got, err := svc.ListForPage(ctx, "c1", "page1", v)
		if err != nil {
			t.Fatal(err)
		}
		if got.CanEdit || len(got.Tests) != 1 {
			t.Fatalf("role %d sees only shown tests and can't edit: %+v", v.VisibilityRole, got)
		}
		p := got.Tests[0]
		if p.Name != "Cross the salt flats" || p.Recorded == nil || p.Recorded.Label != "Partial success" {
			t.Errorf("shown test should carry its name and result: %+v", p)
		}
		if len(p.Results) != 0 || p.Needed != 0 || p.Limit != 0 || p.Version != 0 || p.Offer != "" {
			t.Errorf("a player is told nothing beyond the name and result: %+v", p)
		}
	}
	if _, err := svc.ListForPage(ctx, "c1", "secret", dm); statusOf(err) != 404 {
		t.Errorf("hidden page: want 404, got %v", err)
	}
	if _, err := svc.ListForPage(ctx, "c2", "page1", dm); statusOf(err) != 404 {
		t.Errorf("other campaign: want 404, got %v", err)
	}
}

// Writing is the DM's alone, whatever the page allows.
func TestOnlyTheDMWrites(t *testing.T) {
	svc, _, _ := newTestService(nil)
	ctx := context.Background()
	tt, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []Viewer{scribe, player, {}} {
		if _, err := svc.Create(ctx, "c1", "page1", v, lastwell()); statusOf(err) != 403 {
			t.Errorf("create as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Update(ctx, "c1", tt.ID, v, lastwell()); statusOf(err) != 403 {
			t.Errorf("update as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Tally(ctx, "c1", tt.ID, v, TallyInput{Side: SideSuccess, Step: 1, Version: intp(1)}); statusOf(err) != 403 {
			t.Errorf("tally as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Record(ctx, "c1", tt.ID, v, RecordInput{Result: "win", Version: intp(1)}); statusOf(err) != 403 {
			t.Errorf("record as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if err := svc.Delete(ctx, "c1", tt.ID, v); statusOf(err) != 403 {
			t.Errorf("delete as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Choices(ctx, "c1", v); statusOf(err) != 403 {
			t.Errorf("choices as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Outcomes(ctx, "c1", []string{tt.ID}, v); statusOf(err) != 403 {
			t.Errorf("outcomes as %d: want 403, got %v", v.VisibilityRole, err)
		}
	}
}

func TestReadOnlyPageRefusesWrites(t *testing.T) {
	svc, _, pages := newTestService(nil)
	ctx := context.Background()
	tt, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	pages.readOnly["page1"] = true
	if _, err := svc.Create(ctx, "c1", "page1", dm, lastwell()); statusOf(err) != 403 {
		t.Errorf("create: want 403, got %v", err)
	}
	if _, err := svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: SideSuccess, Step: 1, Version: intp(1)}); statusOf(err) != 403 {
		t.Errorf("tally: want 403, got %v", err)
	}
	pages.hidden["page1"] = true
	if _, err := svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Result: "win", Version: intp(1)}); statusOf(err) != 404 {
		t.Errorf("a test on a hidden page reads as missing: got %v", err)
	}
}

func TestSetupValidation(t *testing.T) {
	svc, _, _ := newTestService(nil)
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*SaveInput)
	}{
		{"no name", func(in *SaveInput) { in.Name = "  " }},
		{"name too long", func(in *SaveInput) { in.Name = string(make([]rune, MaxNameChars+1)) }},
		{"control character", func(in *SaveInput) { in.Kind = "Mon\x00tage" }},
		{"total too high", func(in *SaveInput) { in.Needed = MaxTotal + 1 }},
		{"negative total", func(in *SaveInput) { in.Limit = -1 }},
		{"no results", func(in *SaveInput) { in.Results = nil }},
		{"too many results", func(in *SaveInput) {
			for i := 0; i < MaxResults; i++ {
				in.Results = append(in.Results, Result{ID: "x" + string(rune('a'+i)), Label: "X"})
			}
		}},
		{"duplicate result id", func(in *SaveInput) { in.Results[1].ID = "win" }},
		{"bad result id", func(in *SaveInput) { in.Results[1].ID = "<b>" }},
		{"unnamed result", func(in *SaveInput) { in.Results[1].Label = "" }},
		{"unknown trigger", func(in *SaveInput) { in.Results[1].On = "rounds" }},
		{"two results on one total", func(in *SaveInput) { in.Results[1].On = OnSuccesses }},
		{"trigger with no total", func(in *SaveInput) { in.Needed = 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := lastwell()
			tc.mutate(&in)
			if _, err := svc.Create(ctx, "c1", "page1", dm, in); statusOf(err) != 422 {
				t.Errorf("want 422, got %v", err)
			}
		})
	}
}

func TestTallyStepsOfferAndRecord(t *testing.T) {
	svc, _, _ := newTestService(fakeCal{ok: true})
	ctx := context.Background()
	tt, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	step := func(side string, by int) {
		t.Helper()
		got, err := svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: side, Step: by, Version: intp(tt.Version)})
		if err != nil {
			t.Fatalf("tally %s %d: %v", side, by, err)
		}
		tt = got
	}
	step(SideFailure, -1)
	if tt.Failures != 0 {
		t.Errorf("a side never goes below zero: %+v", tt)
	}
	for i := 0; i < 4; i++ {
		step(SideFailure, 1)
	}
	if tt.Failures != 3 {
		t.Errorf("a side stops at its total: %d", tt.Failures)
	}
	if tt.Offer != "lose" {
		t.Errorf("the failures filled, so their result is offered: %q", tt.Offer)
	}
	step(SideFailure, -1)
	if tt.Offer != "" {
		t.Errorf("stepping back takes the offer away: %q", tt.Offer)
	}
	for i := 0; i < 5; i++ {
		step(SideSuccess, 1)
	}
	if tt.Offer != "win" {
		t.Errorf("the successes filled: %q", tt.Offer)
	}
	if _, err := svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: SideSuccess, Step: 2, Version: intp(tt.Version)}); statusOf(err) != 422 {
		t.Errorf("a step of two: want 422, got %v", err)
	}
	if _, err := svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: "luck", Step: 1, Version: intp(tt.Version)}); statusOf(err) != 422 {
		t.Errorf("an unknown side: want 422, got %v", err)
	}

	rec, err := svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Result: "part", Version: intp(tt.Version)})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Recorded == nil || rec.Recorded.Label != "Partial success" || rec.Recorded.DateLabel != "14 Sanguin 812" ||
		rec.Recorded.Date == nil || rec.Recorded.Date.Day != 14 {
		t.Errorf("the record carries the result and the calendar date: %+v", rec.Recorded)
	}
	if rec.Offer != "" {
		t.Errorf("nothing is offered once a result is recorded: %q", rec.Offer)
	}
	if _, err := svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Result: "nope", Version: intp(rec.Version)}); statusOf(err) != 422 {
		t.Errorf("an unknown result: want 422, got %v", err)
	}

	again, err := svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Reset: true, Version: intp(rec.Version)})
	if err != nil {
		t.Fatal(err)
	}
	if again.Recorded != nil || again.Successes != 0 || again.Failures != 0 {
		t.Errorf("start again clears the record and the tally: %+v", again)
	}
}

func TestRecordWithoutACalendar(t *testing.T) {
	for _, cal := range []Calendar{nil, fakeCal{}, fakeCal{err: errCalendar}} {
		svc, _, _ := newTestService(cal)
		ctx := context.Background()
		tt, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
		if err != nil {
			t.Fatal(err)
		}
		rec, err := svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Result: "win", Version: intp(tt.Version)})
		if err != nil {
			t.Fatalf("%T: a calendar problem never stops a record: %v", cal, err)
		}
		if rec.Recorded.Date != nil || rec.Recorded.DateLabel != "" || rec.Recorded.At.IsZero() {
			t.Errorf("%T: no calendar date, but the time is kept: %+v", cal, rec.Recorded)
		}
	}
}

func TestVersionsGuardEveryWrite(t *testing.T) {
	svc, _, _ := newTestService(nil)
	ctx := context.Background()
	tt, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: SideSuccess, Step: 1}); statusOf(err) != 422 {
		t.Errorf("missing version: want 422, got %v", err)
	}
	if _, err := svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: SideSuccess, Step: 1, Version: intp(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Result: "win", Version: intp(1)}); statusOf(err) != 409 {
		t.Errorf("stale version: want 409, got %v", err)
	}
	in := lastwell()
	in.Version = intp(1)
	if _, err := svc.Update(ctx, "c1", tt.ID, dm, in); statusOf(err) != 409 {
		t.Errorf("stale setup: want 409, got %v", err)
	}
}

// Changing the setup keeps the count made at the table, trimmed to the new
// totals, and keeps the record while its result still exists.
func TestSetupChangeKeepsTheTally(t *testing.T) {
	svc, _, _ := newTestService(nil)
	ctx := context.Background()
	tt, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if tt, err = svc.Tally(ctx, "c1", tt.ID, dm, TallyInput{Side: SideSuccess, Step: 1, Version: intp(tt.Version)}); err != nil {
			t.Fatal(err)
		}
	}
	if tt, err = svc.Record(ctx, "c1", tt.ID, dm, RecordInput{Result: "part", Version: intp(tt.Version)}); err != nil {
		t.Fatal(err)
	}
	in := lastwell()
	in.Needed = 3
	in.Results[1].Label = "Partly"
	in.Version = intp(tt.Version)
	got, err := svc.Update(ctx, "c1", tt.ID, dm, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Successes != 3 || got.Recorded == nil || got.Recorded.Label != "Partly" {
		t.Errorf("tally trimmed and record renamed: %+v %+v", got, got.Recorded)
	}
	in.Results = in.Results[:1]
	in.Version = intp(got.Version)
	if got, err = svc.Update(ctx, "c1", tt.ID, dm, in); err != nil {
		t.Fatal(err)
	}
	if got.Recorded != nil {
		t.Errorf("a record of a removed result is cleared: %+v", got.Recorded)
	}
}

func TestPerPageLimitAndDelete(t *testing.T) {
	svc, _, _ := newTestService(nil)
	ctx := context.Background()
	var last *Test
	for i := 0; i < MaxPerPage; i++ {
		got, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
		if err != nil {
			t.Fatal(err)
		}
		last = got
	}
	if _, err := svc.Create(ctx, "c1", "page1", dm, lastwell()); statusOf(err) != 422 {
		t.Errorf("past the cap: want 422, got %v", err)
	}
	if err := svc.Delete(ctx, "c1", last.ID, dm); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "c1", last.ID, dm); statusOf(err) != 404 {
		t.Errorf("gone: want 404, got %v", err)
	}
}

// The encounter builder and an encounter's picks see tests only on pages the
// DM may open, in this campaign.
func TestChoicesAndOutcomes(t *testing.T) {
	svc, repo, _ := newTestService(nil)
	ctx := context.Background()
	a, err := svc.Create(ctx, "c1", "page1", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, "c1", "page2", dm, lastwell())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(ctx, "c1", b.ID, dm, RecordInput{Result: "lose", Version: intp(b.Version)}); err != nil {
		t.Fatal(err)
	}
	// A test whose page was hidden after it was made.
	hidden := repo.rows[a.ID]
	hidden.ID, hidden.EntityID = "hidden-test", "secret"
	repo.rows[hidden.ID] = hidden

	choices, err := svc.Choices(ctx, "c1", dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 2 {
		t.Fatalf("choices: %+v", choices)
	}
	for _, c := range choices {
		if c.Page.Name == "" || len(c.Results) != 3 {
			t.Errorf("a choice names its page and results: %+v", c)
		}
	}
	outs, err := svc.Outcomes(ctx, "c1", []string{a.ID, b.ID, "hidden-test", "<bad>", "missing"}, dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 || outs[b.ID].Recorded != "lose" || outs[a.ID].Recorded != "" || outs[a.ID].Name != "Restore the Lastwell core" {
		t.Errorf("outcomes: %+v", outs)
	}
	if other, _ := svc.Outcomes(ctx, "c2", []string{a.ID}, dm); len(other) != 0 {
		t.Errorf("another campaign sees nothing: %+v", other)
	}
}

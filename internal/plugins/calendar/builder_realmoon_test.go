package calendar

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- a recorder for the wizard's create path --------------------------------

// builderWriteRecorder records EVERY write BuilderCreateAPI makes, so "what the
// wizard actually persisted" can be asserted rather than reasoned about. The
// embedded CalendarService panics on anything not stubbed, which is deliberate:
// a new write appearing on this path must show up as a test failure, not be
// absorbed silently — that absorption is how the dropped moon survived.
type builderWriteRecorder struct {
	CalendarService

	created  *Calendar
	moons    []MoonInput
	seasons  []Season
	eras     []EraInput
	applied  *ImportResult
	updated  *UpdateCalendarInput
	moonsSet bool
	seasSet  bool
	deleted  string
}

func (r *builderWriteRecorder) CreateCalendar(_ context.Context, campaignID string, in CreateCalendarInput) (*Calendar, error) {
	// Mirror what the real CreateCalendar leaves behind for a real-life
	// calendar: seedDefaults' twelve Gregorian months and seven weekdays, the
	// wall-clock date, and NO moons, seasons or eras.
	cal := moonFallbackGregorianCal("cal-new", 2026, 8, 11)
	cal.CampaignID = campaignID
	cal.Mode = in.Mode
	cal.Name = in.Name
	cal.EpochName = in.EpochName
	r.created = cal
	return cal, nil
}

func (r *builderWriteRecorder) UpdateCalendar(_ context.Context, _ string, in UpdateCalendarInput) error {
	cp := in
	r.updated = &cp
	return nil
}

func (r *builderWriteRecorder) SetMoons(_ context.Context, _ string, m []MoonInput) error {
	r.moons, r.moonsSet = m, true
	return nil
}

func (r *builderWriteRecorder) SetSeasons(_ context.Context, _ string, s []Season) error {
	r.seasons, r.seasSet = s, true
	return nil
}

func (r *builderWriteRecorder) SetEras(_ context.Context, _ string, e []EraInput) error {
	r.eras = e
	return nil
}

func (r *builderWriteRecorder) ApplyImport(_ context.Context, _ string, res *ImportResult) (MonthEditImpact, error) {
	r.applied = res
	return MonthEditImpact{}, nil
}

func (r *builderWriteRecorder) DeleteCalendar(_ context.Context, id string) error {
	r.deleted = id
	return nil
}

// builderRunCreate drives the shipped BuilderCreateAPI with the form the Review
// station actually posts for the given draft.
func builderRunCreate(t *testing.T, d *builderDraft) (*builderWriteRecorder, error) {
	t.Helper()
	rec := &builderWriteRecorder{}
	h := &Handler{svc: rec}
	c := builderFormCtx(t, builderFormFor(d, len(builderStations)-1, false))
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner, IsMember: true,
	})
	return rec, h.BuilderCreateAPI(c)
}

// --- the guard --------------------------------------------------------------

// TestBuilderRealLife_ThePreviewedMoonIsTheCreatedMoon.
//
// THE DEFECT. The real-life card seeded `{Name: "Luna", Period: 29.53,
// NewAt: 6}`, Review printed "Moons: 1", and builder_handler.go returned at the
// [VS-2] landing before the only writer of moons — so the author was shown a
// moon and given none (2026-08-11 observations report §1.2).
//
// THE FIX IS NOT "PERSIST LUNA", AND THAT IS WHAT THIS TEST ENCODES. Measured
// against Chronicle's own gregorianMoonPhase on a calendar shaped as
// CreateCalendar seeds one, Luna runs 0.0287 cycles — 0.85 DAYS — behind the
// real sky, and persisting any row at all switches OFF synthesizedRealMoon,
// which is exact. So the card declares no moon, the create path writes none,
// and both the wizard's preview and the created calendar show THE Moon.
//
// The assertion is the wizard's own law — "the calendar the wizard previews and
// exports and the calendar Create writes are the same calendar" — applied to
// the moon: the phase the preview draws on a date must be the phase the created
// calendar draws on that date, which is gregorianMoonPhase's answer.
func TestBuilderRealLife_ThePreviewedMoonIsTheCreatedMoon(t *testing.T) {
	d, err := builderPresetDraft("gregorian")
	if err != nil {
		t.Fatal(err)
	}
	if d.Mode != ModeRealLife {
		t.Fatalf("fixture drift: the gregorian card is mode %q", d.Mode)
	}

	preview := draftCalendar(d)
	if len(preview.Moons) != 1 {
		t.Fatalf("the wizard previews %d moons for a real-world calendar; want exactly 1 — "+
			"the preview column must draw what Create produces", len(preview.Moons))
	}
	if preview.Moons[0].Name != realMoonName {
		t.Errorf("the preview names its moon %q; the created calendar's is %q. The wizard "+
			"must not advertise a body the product does not create",
			preview.Moons[0].Name, realMoonName)
	}

	// EVERY DAY OF THE PREVIEWED YEAR, against the one astronomy. This is the
	// assertion Luna failed by 0.85 days on every single one of them.
	for _, probe := range []struct{ y, m, dd int }{
		{2026, 1, 1}, {2026, 3, 17}, {2026, 8, 11}, {2026, 12, 31},
	} {
		abs := preview.AbsoluteDay(probe.y, probe.m, probe.dd)
		got := preview.Moons[0].MoonPhase(abs)
		want := gregorianMoonPhase(probe.y, probe.m, probe.dd)
		if diff := phaseDistance(got, want); diff > moonFallbackEpsilon {
			t.Errorf("%04d-%02d-%02d: the wizard previews phase %.6f, the real Moon is at "+
				"%.6f — %.3f days apart. The preview must not draw an approximation of "+
				"the sky it names", probe.y, probe.m, probe.dd, got, want,
				diff*realMoonCycleDays)
		}
	}

	// AND WHAT CREATE ACTUALLY WROTE. An empty moon list is not an oversight
	// here: it is the condition synthesizedRealMoon requires, so a row would
	// trade the exact Moon for a stored approximation.
	rec, err := builderRunCreate(t, d)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !rec.moonsSet {
		t.Error("the real-life branch never reached a moon writer — the declaration is " +
			"dropped at the [VS-2] landing exactly as it was before this fix")
	}
	if len(rec.moons) != 0 {
		t.Errorf("Create wrote %d moon rows (%+v); a real-world calendar must carry none, "+
			"because one authored row replaces the exact synthesized Moon whole",
			len(rec.moons), rec.moons)
	}

	// The created calendar therefore shows one moon, and it is the right one.
	live := rec.created
	if live == nil {
		t.Fatal("no calendar was created")
	}
	applyRealMoonFallback(live)
	if len(live.Moons) != 1 || live.Moons[0].Name != realMoonName {
		t.Fatalf("the created calendar carries %+v; want exactly THE Moon", live.Moons)
	}
	abs := live.AbsoluteDay(2026, 8, 11)
	if diff := phaseDistance(live.Moons[0].MoonPhase(abs), gregorianMoonPhase(2026, 8, 11)); diff > moonFallbackEpsilon {
		t.Errorf("the created calendar's Moon is %.3f days off the sky",
			diff*realMoonCycleDays)
	}
}

// TestBuilderRealLife_ReviewCountsWhatCreateWrites is the general form of the
// same defect, and it is the one that stops a future preset re-introducing it.
//
// The Review panel is headed "Everything below is what Create will write — one
// act, one calendar." Every count under that heading must therefore be a count
// of what the created calendar ends up with. It was not: the real-life card
// printed 1 moon, 4 seasons and 1 era over a create path that wrote none.
func TestBuilderRealLife_ReviewCountsWhatCreateWrites(t *testing.T) {
	d, err := builderPresetDraft("gregorian")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := builderRunCreate(t, d)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Moons: Review prints builderMoonsShown, which is 1 for the synthesized
	// Moon — and that is what the calendar has.
	live := rec.created
	applyRealMoonFallback(live)
	if got, want := builderMoonsShown(d), len(live.Moons); got != want {
		t.Errorf("Review prints %d moons; the created calendar has %d", got, want)
	}

	// Seasons: the card declares none, so none is written, so Review says none.
	if len(d.Seasons) != 0 {
		t.Errorf("the real-life card declares %d seasons; a hemisphere's seasons are not "+
			"a fact about the real world and must not be seeded on the author's behalf",
			len(d.Seasons))
	}
	if len(rec.seasons) != len(d.Seasons) {
		t.Errorf("Create wrote %d seasons against a draft declaring %d",
			len(rec.seasons), len(d.Seasons))
	}

	// Eras: the draft carries one — [WZ-3] holds Create until it does — and its
	// destination is the calendar's epoch name, not a calendar_eras row. Review
	// says exactly that in words rather than letting the count imply a row.
	if len(d.Eras) != 1 {
		t.Fatalf("the era gate needs one era on the draft; got %d", len(d.Eras))
	}
	if rec.eras != nil {
		t.Errorf("Create wrote %d era rows on a real-life calendar; the Eras editor is not "+
			"offered for one (calendar_settings.templ), so the row could never be removed",
			len(rec.eras))
	}
	if live.EpochName == nil || *live.EpochName != "AD" {
		t.Errorf("the era's reading did not reach the calendar's epoch name: %v", live.EpochName)
	}

	// And the panel itself says all three.
	html := builderRenderReview(t, d)
	if !strings.Contains(html, builderRealMoonPhrase) {
		t.Errorf("Review does not name the real Moon; a bare %q would read as an authored "+
			"moon that Settings will show and does not", "1")
	}
	if !strings.Contains(html, "written as the year reckoning, not as an era row") {
		t.Error("Review's era row still implies a calendar_eras row that Create does not write")
	}
}

// TestBuilderRealLife_TheLandingIsUnchanged pins [VS-2] against this fix. The
// dropped moon was fixed by writing BEFORE the redirect; the redirect itself is
// a signed ruling and is not this stage's to move.
func TestBuilderRealLife_TheLandingIsUnchanged(t *testing.T) {
	d, err := builderPresetDraft("gregorian")
	if err != nil {
		t.Fatal(err)
	}
	rec := &builderWriteRecorder{}
	h := &Handler{svc: rec}
	c := builderFormCtx(t, builderFormFor(d, len(builderStations)-1, false))
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner, IsMember: true,
	})
	if err := h.BuilderCreateAPI(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	res := c.Response()
	if res.Status != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", res.Status)
	}
	if got, want := res.Header().Get("Location"), "/campaigns/camp-1/apps/calendar"; got != want {
		t.Errorf("[VS-2]'s landing moved: %q, want %q", got, want)
	}
	// ApplyImport must NOT run on this branch: it forces the date to 1/1 and
	// service.ApplyImport's W8 guard rejects it outright on a real-time
	// calendar. The narrow Set* writers are what this branch uses.
	if rec.applied != nil {
		t.Error("the real-life branch ran ApplyImport, which rewrites the wall-clock date " +
			"the whole mode exists to preserve")
	}
}

// TestBuilderRealLife_AnAuthoredMoonSurvivesTheLanding. The real-life card jumps
// to Review, but stations 1–7 stay navigable by `wz_step`, so an author CAN walk
// back and declare a moon. Before this fix nothing on the branch wrote one and
// it was dropped at the redirect — the preset's moon was only the visible half
// of that hole.
func TestBuilderRealLife_AnAuthoredMoonSurvivesTheLanding(t *testing.T) {
	d, err := builderPresetDraft("gregorian")
	if err != nil {
		t.Fatal(err)
	}
	d.Moons = []builderMoon{{Name: "Selûne", Period: 30.4, NewAt: 2}}
	d.Seasons = []builderSeason{{Name: "The Wet", StartMonth: 11, StartDay: 1, EndMonth: 3, EndDay: 31}}

	rec, err := builderRunCreate(t, d)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(rec.moons) != 1 || rec.moons[0].Name != "Selûne" {
		t.Fatalf("an authored moon was dropped at the landing: %+v", rec.moons)
	}
	if math.Abs(rec.moons[0].CycleDays-30.4) > 1e-9 {
		t.Errorf("cycle = %v, want 30.4", rec.moons[0].CycleDays)
	}
	if len(rec.seasons) != 1 || rec.seasons[0].Name != "The Wet" {
		t.Fatalf("an authored season was dropped at the landing: %+v", rec.seasons)
	}
	// And an authored moon replaces the synthesized one whole — the author's
	// declaration wins, it does not sit beside a body they did not add.
	if builderRealMoonSynthesized(d) {
		t.Error("the synthesized Moon is still claimed alongside an authored one")
	}
	preview := draftCalendar(d)
	if len(preview.Moons) != 1 || preview.Moons[0].Name != "Selûne" {
		t.Fatalf("the preview shows %+v beside one authored moon", preview.Moons)
	}
}

// builderPreviewMinusRealMoon returns the wizard's preview calendar with the
// SYNTHESIZED real Moon removed, for the two parity guards that compare the
// preview against builderImportResult.
//
// THE ONE PERMITTED DIVERGENCE BETWEEN PREVIEW AND CREATE, AND IT IS NARROW.
// Those guards enforce "the calendar the wizard previews is the calendar Create
// writes", and the law is untouched for every authored body. The real Moon is
// the single case where the two must differ ON PURPOSE: the preview SHOWS it,
// and the create payload deliberately STORES nothing, because one stored row
// switches synthesizedRealMoon off and would trade an exact Moon for a frozen
// approximation (see builder_presets.go). Stripping it here — after asserting
// it is exactly what it claims to be — keeps the divergence one named body wide
// instead of letting a `len` mismatch be waved through.
func builderPreviewMinusRealMoon(t *testing.T, d *builderDraft) *Calendar {
	t.Helper()
	cal := draftCalendar(d)
	if !builderRealMoonSynthesized(d) {
		return cal
	}
	if len(cal.Moons) != 1 || cal.Moons[0].Name != realMoonName ||
		cal.Moons[0].CycleDays != realMoonCycleDays {
		t.Fatalf("the preview's extra body is %+v; the ONLY divergence permitted between "+
			"the preview and the create payload is the synthesized real Moon", cal.Moons)
	}
	cal.Moons = nil
	return cal
}

// builderRenderReview renders the Review station's shell and returns its HTML.
func builderRenderReview(t *testing.T, d *builderDraft) string {
	t.Helper()
	data := builderView("camp-1", "tok", d, len(builderStations)-1, false, 0)
	var sb strings.Builder
	if err := BuilderShellFragment(data).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

package sessions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// C-RSVP-P9 — alternating-week availability.
//
// The whole feature rests on one question: does a block stored against "week A"
// fall on the right real dates? Everything else is plumbing. These tests pin the
// answer against real calendar dates rather than against the implementation.

func cd(y int, m time.Month, d int) timeutil.CivilDate {
	return timeutil.CivilDate{Year: y, Month: m, Day: d}
}

// A track must be stable across a whole week: every day from Sunday to Saturday
// has to agree, or a member's "alternate Mondays" would land on a different
// fortnight from their "alternate Saturdays" in the same painted week.
func TestCadence_IsConstantWithinASundayStartedWeek(t *testing.T) {
	// 2026-08-16 is a Sunday.
	start := cd(2026, time.August, 16)
	want := WeekCadenceFor(start)
	for i := 0; i < 7; i++ {
		got := WeekCadenceFor(start.AddDays(i))
		if got != want {
			t.Fatalf("%s: cadence %d, but the week starting %s is %d — a track that changes mid-week would split one painted week across two fortnights",
				start.AddDays(i).String(), got, start.String(), want)
		}
	}
}

// Consecutive weeks must alternate, and the week after next must come back
// round. This is the property that makes "every other week" mean every other.
func TestCadence_AlternatesAndReturns(t *testing.T) {
	sunday := cd(2026, time.August, 16)
	a := WeekCadenceFor(sunday)
	b := WeekCadenceFor(sunday.AddDays(7))
	c := WeekCadenceFor(sunday.AddDays(14))

	if a == b {
		t.Fatalf("consecutive weeks share track %d — nothing alternates", a)
	}
	if a != c {
		t.Fatalf("week+14 is track %d, want %d — the cycle is not two weeks long", c, a)
	}
	if !ValidWeekCadence(a) || a == CadenceEveryWeek {
		t.Fatalf("WeekCadenceFor returned %d; a DATE is never 'every week'", a)
	}
}

// Dates before the epoch must not swap the two tracks. Go's / truncates toward
// zero, so a naive weekIndex would flip sign and hand 1969 the wrong track —
// and a bug that only shows up on one side of one date is exactly the kind that
// survives review.
func TestCadence_DoesNotFlipBeforeTheEpoch(t *testing.T) {
	// 1970-01-04 is the epoch Sunday; 1969-12-28 is the Sunday before it.
	epochWeek := WeekCadenceFor(cd(1970, time.January, 4))
	weekBefore := WeekCadenceFor(cd(1969, time.December, 28))
	weekBeforeThat := WeekCadenceFor(cd(1969, time.December, 21))

	if weekBefore == epochWeek {
		t.Fatalf("the week before the epoch has the same track as the epoch week (%d) — floor division is wrong", epochWeek)
	}
	if weekBeforeThat != epochWeek {
		t.Fatalf("two weeks before the epoch is track %d, want %d", weekBeforeThat, epochWeek)
	}
}

// cadenceApplies is the projection's gate. Every-week and unrecognised values
// must apply ALWAYS — that is what keeps every row written before this feature
// existed meaning what it meant.
func TestCadenceApplies_EveryWeekAndUnknownAlwaysApply(t *testing.T) {
	for _, d := range []timeutil.CivilDate{
		cd(2026, time.August, 16), cd(2026, time.August, 23), cd(2026, time.August, 30),
	} {
		if !cadenceApplies(CadenceEveryWeek, d) {
			t.Fatalf("%s: an every-week block did not apply", d.String())
		}
		if !cadenceApplies(99, d) {
			t.Fatalf("%s: an unrecognised cadence did not apply; availability must fail LOUD (too often), never silently vanish", d.String())
		}
	}
}

func TestCadenceApplies_AlternatingHitsEverySecondWeek(t *testing.T) {
	sunday := cd(2026, time.August, 16)
	track := WeekCadenceFor(sunday)

	hits := 0
	for w := 0; w < 6; w++ {
		if cadenceApplies(track, sunday.AddDays(7*w)) {
			hits++
		}
	}
	if hits != 3 {
		t.Fatalf("a fortnightly block landed on %d of 6 weeks, want 3", hits)
	}
	other := CadenceWeekA
	if track == CadenceWeekA {
		other = CadenceWeekB
	}
	if cadenceApplies(other, sunday) {
		t.Fatalf("the opposite track also applied on %s — the two tracks overlap", sunday.String())
	}
}

// The picker labels each track with a real Sunday. Those two labels must be a
// week apart and must actually BE the tracks they claim, or the member picks a
// date and gets the other fortnight.
func TestCadenceLabel_NamesTheRightSundays(t *testing.T) {
	from := cd(2026, time.August, 19) // a Wednesday, mid-week on purpose
	a := CadenceLabel(CadenceWeekA, from)
	b := CadenceLabel(CadenceWeekB, from)

	if WeekCadenceFor(a) != CadenceWeekA {
		t.Fatalf("the 'week A' label %s is not in track A", a.String())
	}
	if WeekCadenceFor(b) != CadenceWeekB {
		t.Fatalf("the 'week B' label %s is not in track B", b.String())
	}
	if a.Weekday() != time.Sunday || b.Weekday() != time.Sunday {
		t.Fatalf("labels must be Sundays, got %s (%s) and %s (%s)",
			a.String(), a.Weekday(), b.String(), b.Weekday())
	}
	diff := 0
	for cur := a; cur.String() != b.String() && diff < 21; cur = cur.AddDays(1) {
		diff++
	}
	if diff != 7 {
		t.Fatalf("the two labels are %d days apart, want 7", diff)
	}
	// One of them must be the week `from` itself is in — otherwise the picker
	// offers two future weeks and the member cannot express "starting now".
	ownSunday := from.AddDays(-int(from.Weekday()))
	if a.String() != ownSunday.String() && b.String() != ownSunday.String() {
		t.Fatalf("neither label (%s, %s) is the current week %s", a.String(), b.String(), ownSunday.String())
	}
}

// The overlay must actually honour the cadence. This drives the real projection
// rather than cadenceApplies directly, because the defect that matters is a
// block that is stored fortnightly and rendered weekly.
func TestOverlay_FortnightlyBlockAppearsOnAlternateWeeksOnly(t *testing.T) {
	// Monday 2026-08-17; track is whatever that week is.
	monday := cd(2026, time.August, 17)
	track := WeekCadenceFor(monday)

	blocks := []AvailabilityBlock{{
		UserID: "u1", DayOfWeek: int(time.Monday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "UTC", WeekCadence: track,
	}}
	availByUser := map[string][]AvailabilityBlock{"u1": blocks}

	onTrack := effectiveBlocks("u1", monday, availByUser, nil)
	if len(onTrack) != 1 {
		t.Fatalf("the block did not apply on its own week (%s): got %d blocks", monday.String(), len(onTrack))
	}
	offTrack := effectiveBlocks("u1", monday.AddDays(7), availByUser, nil)
	if len(offTrack) != 0 {
		t.Fatalf("a fortnightly block applied on the OFF week (%s) too — it is behaving as weekly",
			monday.AddDays(7).String())
	}
	backOn := effectiveBlocks("u1", monday.AddDays(14), availByUser, nil)
	if len(backOn) != 1 {
		t.Fatalf("the block did not come back two weeks later (%s)", monday.AddDays(14).String())
	}
}

// A pre-cadence row (WeekCadence zero-valued) must still apply every week. This
// is the migration's promise, asserted through the projection.
func TestOverlay_PreCadenceBlockStillAppliesEveryWeek(t *testing.T) {
	monday := cd(2026, time.August, 17)
	availByUser := map[string][]AvailabilityBlock{"u1": {{
		UserID: "u1", DayOfWeek: int(time.Monday), StartMinute: 18 * 60, EndMinute: 22 * 60,
		State: AvailAvailable, TZ: "UTC", // WeekCadence left at its zero value
	}}}
	for w := 0; w < 4; w++ {
		d := monday.AddDays(7 * w)
		if got := effectiveBlocks("u1", d, availByUser, nil); len(got) != 1 {
			t.Fatalf("%s: a pre-cadence block did not apply (got %d) — the migration's DEFAULT 0 promise is broken", d.String(), len(got))
		}
	}
}

// --- the answered-or-not state ---------------------------------------------

// The whole point: saving an EMPTY grid is an answer. If Answered were derived
// from len(blocks) this test would fail, which is why it exists.
func TestSaveMyAvailability_EmptyGridStillCountsAsAnswering(t *testing.T) {
	var gotTZ string
	var gotBlocks int
	called := false
	repo := &mockSessionRepo{
		replaceUserAvailabilityFn: func(_ context.Context, _, _, tz string, blocks []AvailabilityBlock) error {
			called, gotTZ, gotBlocks = true, tz, len(blocks)
			return nil
		},
	}
	svc := NewSessionService(repo, nil)
	if err := svc.SaveMyAvailability(context.Background(), "c1", "u1",
		SaveAvailabilityRequest{TZ: "America/New_York", Blocks: nil}); err != nil {
		t.Fatalf("saving an empty grid failed: %v", err)
	}
	if !called {
		t.Fatal("an empty save never reached the store — 'I am never free' would stay indistinguishable from silence")
	}
	if gotBlocks != 0 {
		t.Fatalf("expected 0 blocks, got %d", gotBlocks)
	}
	if gotTZ != "America/New_York" {
		t.Fatalf("the zone must ride separately so an empty answer still records one; got %q", gotTZ)
	}
}

func TestSaveMyAvailability_RejectsAnUnknownCadence(t *testing.T) {
	svc := NewSessionService(&mockSessionRepo{}, nil)
	err := svc.SaveMyAvailability(context.Background(), "c1", "u1", SaveAvailabilityRequest{
		TZ: "UTC",
		Blocks: []AvailabilityBlockDTO{{
			DayOfWeek: 1, StartMinute: 60, EndMinute: 120, State: AvailAvailable, WeekCadence: 7,
		}},
	})
	if err == nil {
		t.Fatal("an unknown cadence was accepted; it would be stored and then silently treated as every-week")
	}
}

// Two blocks that differ ONLY by track must both survive the save. Before
// cadence the dedupe key was (day, start, end), which would have collapsed
// these into one and thrown away half the member's answer.
func TestSaveMyAvailability_SameSlotOnBothTracksSurvives(t *testing.T) {
	var saved []AvailabilityBlock
	repo := &mockSessionRepo{
		replaceUserAvailabilityFn: func(_ context.Context, _, _, _ string, blocks []AvailabilityBlock) error {
			saved = blocks
			return nil
		},
	}
	svc := NewSessionService(repo, nil)
	err := svc.SaveMyAvailability(context.Background(), "c1", "u1", SaveAvailabilityRequest{
		TZ: "UTC",
		Blocks: []AvailabilityBlockDTO{
			{DayOfWeek: 1, StartMinute: 1080, EndMinute: 1320, State: AvailAvailable, WeekCadence: CadenceWeekA},
			{DayOfWeek: 1, StartMinute: 1080, EndMinute: 1320, State: AvailPreferred, WeekCadence: CadenceWeekB},
		},
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("got %d stored blocks, want 2 — the dedupe key dropped a track", len(saved))
	}
}

// --- the nudge --------------------------------------------------------------

func TestNudge_AsksOnlyTheSilentOnes(t *testing.T) {
	answered := map[string]time.Time{"u_answered": time.Now().UTC()}
	var notified []string
	repo := &mockSessionRepo{
		listAnsweredUserIDsFn: func(_ context.Context, _ string) (map[string]time.Time, error) {
			return answered, nil
		},
		createNotificationFn: func(_ context.Context, n *Notification) error {
			notified = append(notified, n.UserID)
			if n.Type != NotifAvailabilityNudge {
				t.Fatalf("wrong notification type %q", n.Type)
			}
			return nil
		},
	}
	svc := NewSessionService(repo, nil)
	res, err := svc.NudgeUnansweredAvailability(context.Background(), "c1", "/link", []overlayMemberInput{
		{UserID: "u_answered", Name: "Ada"},
		{UserID: "u_silent", Name: "Bo"},
		{UserID: "", Name: "blank"},
	})
	if err != nil {
		t.Fatalf("nudge failed: %v", err)
	}
	if len(notified) != 1 || notified[0] != "u_silent" {
		t.Fatalf("notified %v, want exactly [u_silent] — nudging people who already answered trains the table to ignore the bell", notified)
	}
	if res.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", res.Skipped)
	}
	if len(res.Notified) != 1 || res.Notified[0] != "Bo" {
		t.Fatalf("result names %v, want [Bo]; the Director needs names to answer 'did you ping me?'", res.Notified)
	}
}

func TestNudge_WithNobodySilentSendsNothing(t *testing.T) {
	sent := 0
	repo := &mockSessionRepo{
		listAnsweredUserIDsFn: func(_ context.Context, _ string) (map[string]time.Time, error) {
			return map[string]time.Time{"u1": time.Now().UTC()}, nil
		},
		createNotificationFn: func(_ context.Context, _ *Notification) error { sent++; return nil },
	}
	svc := NewSessionService(repo, nil)
	res, err := svc.NudgeUnansweredAvailability(context.Background(), "c1", "/link",
		[]overlayMemberInput{{UserID: "u1", Name: "Ada"}})
	if err != nil {
		t.Fatalf("nudge failed: %v", err)
	}
	if sent != 0 {
		t.Fatalf("sent %d notifications when nobody was silent", sent)
	}
	if len(res.Notified) != 0 || res.Skipped != 1 {
		t.Fatalf("got %+v, want nothing notified and 1 skipped", res)
	}
}

// The overlay roster must carry the answered flag, because an empty lane list
// is what "never free" and "never asked" BOTH look like.
func TestOverlay_CarriesHasAnswered(t *testing.T) {
	repo := &mockSessionRepo{
		listAnsweredUserIDsFn: func(_ context.Context, _ string) (map[string]time.Time, error) {
			return map[string]time.Time{"u_answered": time.Now().UTC()}, nil
		},
	}
	svc := NewSessionService(repo, nil)
	ov, err := svc.BuildOverlay(context.Background(), "c1", []overlayMemberInput{
		{UserID: "u_answered", Name: "Ada"},
		{UserID: "u_silent", Name: "Bo"},
	}, "2026-08-17", "UTC", true)
	if err != nil {
		t.Fatalf("BuildOverlay: %v", err)
	}
	if len(ov.Members) != 2 {
		t.Fatalf("got %d roster rows, want 2", len(ov.Members))
	}
	byName := map[string]OverlayMember{}
	for _, m := range ov.Members {
		byName[m.Name] = m
	}
	if !byName["Ada"].HasAnswered {
		t.Fatal("a member who answered is reported as not having answered")
	}
	if byName["Bo"].HasAnswered {
		t.Fatal("a member who never answered is reported as having answered — the ambiguity this feature removes is back")
	}
}

// BuildOverlay must not mutate the caller's roster slice: the handler reuses it
// for other surfaces, and a stamped-in-place copy would leak the flag to
// callers that never asked for it.
func TestOverlay_DoesNotMutateTheCallersRoster(t *testing.T) {
	repo := &mockSessionRepo{
		listAnsweredUserIDsFn: func(_ context.Context, _ string) (map[string]time.Time, error) {
			return map[string]time.Time{"u1": time.Now().UTC()}, nil
		},
	}
	svc := NewSessionService(repo, nil)
	roster := []overlayMemberInput{{UserID: "u1", Name: "Ada"}}
	if _, err := svc.BuildOverlay(context.Background(), "c1", roster, "2026-08-17", "UTC", true); err != nil {
		t.Fatalf("BuildOverlay: %v", err)
	}
	if roster[0].HasAnswered {
		t.Fatal("BuildOverlay wrote HasAnswered back into the caller's slice")
	}
}

// The picker's labels must be derived in the MEMBER'S zone. A member in UTC+13
// on a Sunday morning is still Saturday by UTC, so a UTC-derived "today" hands
// them the previous week's Sunday and silently swaps which track their picker
// calls A — they pick a date and get the other fortnight.
func TestGetMyAvailability_LabelsAreDerivedInTheMembersZone(t *testing.T) {
	repo := &mockSessionRepo{
		listUserAvailabilityFn: func(_ context.Context, _, _ string) ([]AvailabilityBlock, error) {
			return []AvailabilityBlock{{
				DayOfWeek: 1, StartMinute: 60, EndMinute: 120,
				State: AvailAvailable, TZ: "Pacific/Auckland",
			}}, nil
		},
	}
	svc := NewSessionService(repo, nil)
	resp, err := svc.GetMyAvailability(context.Background(), "c1", "u1")
	if err != nil {
		t.Fatalf("GetMyAvailability: %v", err)
	}
	if resp.WeekALabel == "" || resp.WeekBLabel == "" {
		t.Fatal("the picker got no track labels; it would have to invent dates")
	}

	// Whatever the labels are, they must be self-consistent: each must be a
	// Sunday genuinely in the track it claims, and one of them must be the week
	// the member is standing in RIGHT NOW, in their own zone.
	a, err := timeutil.ParseCivilDate(resp.WeekALabel)
	if err != nil {
		t.Fatalf("week A label %q is not a date: %v", resp.WeekALabel, err)
	}
	b, err := timeutil.ParseCivilDate(resp.WeekBLabel)
	if err != nil {
		t.Fatalf("week B label %q is not a date: %v", resp.WeekBLabel, err)
	}
	if WeekCadenceFor(a) != CadenceWeekA || WeekCadenceFor(b) != CadenceWeekB {
		t.Fatalf("labels are in the wrong tracks: A=%s B=%s", resp.WeekALabel, resp.WeekBLabel)
	}

	now := time.Now().In(timeutil.LoadLocation("Pacific/Auckland"))
	local := timeutil.CivilDate{Year: now.Year(), Month: now.Month(), Day: now.Day()}
	ownSunday := local.AddDays(-int(local.Weekday())).String()
	if resp.WeekALabel != ownSunday && resp.WeekBLabel != ownSunday {
		t.Fatalf("neither label (%s, %s) is the member's current local week %s — the labels were "+
			"derived in the wrong zone", resp.WeekALabel, resp.WeekBLabel, ownSunday)
	}
}

// --- the nudge's permission gate -------------------------------------------
//
// The nudge is the ONE availability action that writes into other people's
// notification lists, so an ungated version is a campaign-wide broadcast handed
// to anyone who can reach the URL. The route is Player+ (this group decides
// entitlement by role in a handler, not by route), which makes the handler's own
// check the only thing standing between a player and everybody's bell.

func nudgeRequest(t *testing.T, h *Handler, role campaigns.Role, isCoDM bool) int {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/c1/availability/nudge", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "c1"},
		MemberRole:  role,
		IsDmGranted: isCoDM,
	})
	if err := h.NudgeAvailabilityAPI(c); err != nil {
		t.Fatalf("handler returned err: %v", err)
	}
	return rec.Code
}

func TestNudge_IsRefusedToAPlainPlayer(t *testing.T) {
	h := NewHandler(NewSessionService(&mockSessionRepo{}, nil))
	if code := nudgeRequest(t, h, campaigns.RolePlayer, false); code != http.StatusForbidden {
		t.Fatalf("a plain player got %d, want 403 — they could bell the whole campaign", code)
	}
}

func TestNudge_IsAllowedToTheOwnerAndToACoDM(t *testing.T) {
	h := NewHandler(NewSessionService(&mockSessionRepo{}, nil))
	if code := nudgeRequest(t, h, campaigns.RoleOwner, false); code == http.StatusForbidden {
		t.Fatal("the owner was refused their own nudge")
	}
	// A co-DM is a capability laid over a role, not a role — gating on role
	// alone would refuse them (the WG-4 mistake, in the other direction).
	if code := nudgeRequest(t, h, campaigns.RolePlayer, true); code == http.StatusForbidden {
		t.Fatal("a co-DM was refused; IsDmGranted is not being consulted")
	}
}

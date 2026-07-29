// bench_ask_control_test.go — the RSVP panel's ask control in all three of its
// states, and the proof that none of them reaches a player
// (C-CALV4-RSVP-P8B stage 4, rulings [PB-5], [PB-8], [PB-9]).
//
// The three states are the whole point of the slice's honesty work: a control
// that is live when it can send, disabled with a reason when it cannot, and
// never a silent success.
package calendar

import (
	"strings"
	"testing"
	"time"
)

// benchFxAskInput is benchFxRsvpInput plus whichever ask-control state this
// case is about. It goes through the SAME pure builder the resolver feeds, so
// these tests exercise the shipped arithmetic and not a parallel one.
func benchFxAskInput(isGM bool, mailConfigured bool, last time.Time) benchRsvpInput {
	in := benchFxRsvpInput(isGM)
	in.MailConfigured = mailConfigured
	in.AskState = ScheduleAskState{LastAskedAt: last, Ready: true}
	if !last.IsZero() {
		if elapsed := time.Since(last); elapsed < scheduleAskCampaignCooldown {
			in.AskState.Ready = false
			in.AskState.RetryAfter = scheduleAskCampaignCooldown - elapsed
		}
	}
	return in
}

// benchFxDataRsvpAsk is a full Bench render whose panel is in a chosen
// ask-control state. Built from the shipped fixtures so the DOM under test is
// the DOM production renders.
func benchFxDataRsvpAsk(mailConfigured bool) BenchData {
	d := benchFxDataRsvp(true, true)
	d.Rsvp = benchRsvpBuild(benchFxAskInput(true, mailConfigured, time.Time{}))
	return d
}

// TestBenchAsk_ThreeStates is §8's table, asserted state by state.
func TestBenchAsk_ThreeStates(t *testing.T) {
	t.Run("configured and never asked: LIVE, no badge, no extra line", func(t *testing.T) {
		p := benchRsvpBuild(benchFxAskInput(true, true, time.Time{}))
		if p.Ask == nil {
			t.Fatal("the Director's panel must carry the ask control")
		}
		if p.Ask.Disabled {
			t.Error("a configured mail server outside the cooldown must leave the control LIVE")
		}
		if p.Ask.Badge != "" {
			t.Errorf("a live control carries no badge; got %q", p.Ask.Badge)
		}
		if p.Ask.Label != "Nudge" {
			t.Errorf("the label stays %q, inherited from P8A; got %q", "Nudge", p.Ask.Label)
		}
		if p.Ask.Action != "/campaigns/camp-1/calendar/ask" {
			t.Errorf("the control posts to the signed path; got %q", p.Ask.Action)
		}
		if p.Ask.CSRFToken != "fx-csrf" {
			t.Error("a POST control must carry the CSRF token")
		}
		// SILENCE IS THE TRUE STATE. No caption is added when there is nothing
		// to explain.
		for _, c := range p.Captions {
			if strings.Contains(c, "You can ask again") || strings.Contains(c, "not configured") {
				t.Errorf("the askable state added a caption it should not have: %q", c)
			}
		}
	})

	t.Run("configured but cooling down: disabled with the wait stated", func(t *testing.T) {
		p := benchRsvpBuild(benchFxAskInput(true, true, time.Now().UTC().Add(-2*time.Hour)))
		if p.Ask == nil || !p.Ask.Disabled {
			t.Fatal("a campaign inside its cooldown must not have a live ask control")
		}
		if p.Ask.Badge != "" {
			t.Errorf("a cooldown is not a deployment fact and carries no badge; got %q", p.Ask.Badge)
		}
		var line string
		for _, c := range p.Captions {
			if strings.Contains(c, "You can ask again") {
				line = c
			}
		}
		if line == "" {
			t.Fatalf("the cooldown must be stated in the panel's foot; captions = %v", p.Captions)
		}
		if !strings.Contains(line, "Asked 2 hours ago") || !strings.Contains(line, "in 4 hours") {
			t.Errorf("the cooldown line must say when and how long; got %q", line)
		}
		// The same sentence is in `title`, but never ONLY there.
		if !strings.Contains(p.Ask.Title, "You can ask again") {
			t.Errorf("title should carry the reason too; got %q", p.Ask.Title)
		}
	})

	t.Run("SMTP unconfigured: disabled, .badge.warn, the verbatim sentence", func(t *testing.T) {
		p := benchRsvpBuild(benchFxAskInput(true, false, time.Time{}))
		if p.Ask == nil || !p.Ask.Disabled {
			t.Fatal("with no mail server the control must be disabled")
		}
		if p.Ask.Badge != "email not configured" {
			t.Errorf("Badge = %q, want the [PB-8] warn text", p.Ask.Badge)
		}
		found := false
		for _, c := range p.Captions {
			if c == mailNotConfiguredLine {
				found = true
			}
		}
		if !found {
			t.Errorf("the unconfigured sentence must appear VERBATIM in the foot; captions = %v", p.Captions)
		}
	})
}

// TestBenchAsk_UnconfiguredSentenceIsOneSharedConstant pins that the Bench and
// the endpoint say the same thing, character for character, because they say it
// from the same place. Part B's drawing renders this sentence too; one fact,
// one sentence, every surface.
func TestBenchAsk_UnconfiguredSentenceIsOneSharedConstant(t *testing.T) {
	const want = "Email is not configured on this server — answers still work in-app; nobody was emailed."
	if mailNotConfiguredLine != want {
		t.Errorf("the shared constant drifted from spec ledger item 11's wording:\n got  %q\n want %q",
			mailNotConfiguredLine, want)
	}
	html := renderBench(t, benchFxDataRsvpAsk(false))
	if !strings.Contains(html, "Email is not configured on this server") {
		t.Error("the rendered panel must carry the unconfigured sentence")
	}
	if !strings.Contains(html, `class="badge warn">email not configured`) {
		t.Error("the unconfigured state must carry .badge.warn, not .badge.need")
	}
	if strings.Contains(html, `class="badge need">email not configured`) {
		t.Error(".badge.need was diluted — its text is always the literal `needs backend`")
	}
}

// TestBenchAsk_LiveControlIsAPostFormWithCSRF is [PB-5] item 5: the control
// must be a POST form, never an <a>. Mailing a roster from a GET is one link
// prefetcher away from a fan-out nobody clicked.
func TestBenchAsk_LiveControlIsAPostFormWithCSRF(t *testing.T) {
	html := renderBench(t, benchFxDataRsvpAsk(true))
	if !strings.Contains(html, `method="post" action="/campaigns/camp-1/calendar/ask"`) {
		t.Error("the ask control must be a POST form to the signed path")
	}
	if !strings.Contains(html, `name="csrf_token"`) {
		t.Error("the ask form must carry the CSRF token as a hidden field")
	}
	if strings.Contains(html, `href="/campaigns/camp-1/calendar/ask"`) {
		t.Error("the ask control must never render as a link")
	}
	// NO JS IS ADDED TO THE BENCH: hx-* attributes are markup, and the control
	// carries no inline handler of its own. (The page's one <script> is wave
	// 1's calendar_permissions.js, which this slice does not touch.)
	if !strings.Contains(html, `hx-post="/campaigns/camp-1/calendar/ask"`) {
		t.Error("the control should swap in place via hx-post")
	}
	for _, forbidden := range []string{"onclick=", "onsubmit=", "javascript:"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("the Bench grew an inline handler (%q)", forbidden)
		}
	}
}

// TestBenchAsk_ProposeKeepsItsChipAndTheReasonShrank is [PB-5] item 4. The old
// ActionsWhy ended "…and RSVP mail fans out only at the moment collection is
// switched on"; keeping that sentence after this slice would be a NEW false
// honesty state.
func TestBenchAsk_ProposeKeepsItsChipAndTheReasonShrank(t *testing.T) {
	p := benchRsvpBuild(benchFxAskInput(true, true, time.Time{}))
	if len(p.Actions) != 1 || p.Actions[0].Label != "Propose" {
		t.Fatalf("the panel's chipped controls should be Propose alone; got %+v", p.Actions)
	}
	if !p.Actions[0].NeedsBackend {
		t.Error("Propose keeps its chip — there is still no propose-from-window write path")
	}
	if strings.Contains(p.ActionsWhy, "Nudge") {
		t.Errorf("ActionsWhy still names Nudge, which is live now: %q", p.ActionsWhy)
	}
	if strings.Contains(p.ActionsWhy, "fans out only") {
		t.Errorf("ActionsWhy still claims mail only fans out on the collection toggle: %q", p.ActionsWhy)
	}
	if !strings.Contains(p.ActionsWhy, "propose-from-window") {
		t.Errorf("ActionsWhy must still name what Propose cannot do: %q", p.ActionsWhy)
	}
}

// TestBenchAsk_BothTileNudgesAreRetired is [PB-5] items 2 and 3, and PB-9's
// divergences 2 and 3. RETIRED, not made live: two live buttons mailing one
// roster from one page is a double-send affordance.
func TestBenchAsk_BothTileNudgesAreRetired(t *testing.T) {
	live := benchSessionTileLive(benchSessionTileInput{
		IsGM: true, CampaignID: "camp-1", CalendarID: "cal-1", EventID: "evt-1",
		Name: "Session 41", When: "today", Answered: 3, Total: 5,
	})
	if len(live.Actions) != 0 {
		t.Errorf("the live session tile's GM branch must render no action; got %+v", live.Actions)
	}
	if live.RSVPForm != nil {
		t.Error("the GM branch has no answer trio either — the Director is not an invitee here")
	}

	notScheduled := benchSessionTile(true)
	for _, a := range notScheduled.Actions {
		if a.Label == "Nudge" {
			t.Error(`the "not scheduled here yet" tile still carries a Nudge with no referent`)
		}
	}

	// And the panel is where the one live control lives.
	html := renderBench(t, benchFxDataRsvpAsk(true))
	if n := strings.Count(html, `>Nudge<`); n != 1 {
		t.Errorf("the page carries %d Nudge controls, want exactly 1 (the panel's)", n)
	}
}

// --- the screenshot-gate fixtures (C-CALV4-RSVP-P8B) ------------------------
//
// The three control states plus the player's absence proof, fed through the
// SAME builder every other Bench render uses.

type benchFxAskCase int

const (
	// benchFxAskNever is a campaign that has never been asked.
	benchFxAskNever benchFxAskCase = iota
	// benchFxAskCooling is a campaign asked two hours ago.
	benchFxAskCooling
	// benchFxAskPlayer renders the panel at PLAYER role, to prove absence.
	benchFxAskPlayer
)

func benchFxDataRsvpAskShot(mailConfigured bool, c benchFxAskCase) BenchData {
	isGM := c != benchFxAskPlayer
	last := time.Time{}
	if c == benchFxAskCooling {
		last = time.Now().UTC().Add(-2 * time.Hour)
	}
	d := benchFxDataRsvp(isGM, isGM)
	d.Rsvp = benchRsvpBuild(benchFxAskInput(isGM, mailConfigured, last))
	return d
}

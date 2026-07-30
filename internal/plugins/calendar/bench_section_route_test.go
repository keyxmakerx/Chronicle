// bench_section_route_test.go — the `section=` branch of the EXISTING prefs
// route (C-CALV4-BENCH-R2 slice R2-1, [BR2-5] SIGNED).
//
// It inherits LAYERS-P9 §12.1's security review verbatim — no response body, no
// fragment, no OOB target; user_id from the session only; campaign_id from :id,
// already authorised by RequireCampaignAccess; the primary key is (session
// user, authorised campaign) and no parameter can move it. THE ONE DELTA is a
// second accepted field, `section`, drawn from a four-key closed registry and
// rejected 400 when unknown. That is the whole new surface, and the assertions
// below are its proof.
//
// The harness is servePrefs from block_prefs_route_test.go — deliberately the
// same one, because the claim is that this is the same route.
package calendar

import (
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// assertStatus proves the REJECTION CODE, not merely that something failed.
// "400, not dropped" is an acceptance item and a 422 would satisfy a bare
// non-nil check while breaking the contract.
func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error %v is not an *apperror.AppError; the route must speak the "+
			"domain error vocabulary, never a raw error", err)
	}
	if ae.Code != want {
		t.Errorf("status = %d, want %d (%s)", ae.Code, want, ae.Message)
	}
}

// THE ASYMMETRY IS LOAD-BEARING and this is the assertion that says so. A
// disclosure has ALREADY opened or closed in the browser by the time the flip
// arrives; refreshing the page would fight the register's own animation, re-run
// both Blocks' renders, and visibly undo what the viewer just did.
func TestBenchSectionPrefs_AnswersNoContentAndDoesNotRefresh(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	rec, err := servePrefs(h, "section=rsvp", "user-1")
	if err != nil {
		t.Fatalf("BlockPrefsAPI: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("response body = %q, want empty — there is no fragment to leak into", body)
	}
	if got := rec.Header().Get("HX-Refresh"); got != "" {
		t.Errorf("HX-Refresh = %q, want absent — a page refresh per chevron would undo "+
			"the disclosure the viewer just opened, before they saw it happen", got)
	}
}

// The other half of the asymmetry: the layers branch is UNCHANGED. A Block
// genuinely must re-render for a layer change ([LYR-1]).
func TestBenchSectionPrefs_LayersBranchStillRefreshes(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	rec, err := servePrefs(h, "layers=moons", "user-1")
	if err != nil {
		t.Fatalf("BlockPrefsAPI: %v", err)
	}
	if got := rec.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("HX-Refresh = %q, want \"true\" — the section branch must not have "+
			"leaked its no-refresh rule onto the layer branch", got)
	}
}

// Exactly one of the two. Both present is a 400 because one write per request
// means a partial failure cannot half-apply.
func TestBenchSectionPrefs_BothFieldsRejects(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	rec, err := servePrefs(h, "layers=moons&section=rsvp", "user-1")
	if err == nil {
		t.Fatalf("both fields accepted (status %d); one write per request is the rule", rec.Code)
	}
	assertStatus(t, err, http.StatusBadRequest)
}

// Neither present stays a 400, with the layers reason intact: `layers=` is how
// the empty set is spelled, so an absent field is malformed, not empty.
func TestBenchSectionPrefs_NeitherFieldRejects(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	if _, err := servePrefs(h, "", "user-1"); err == nil {
		t.Fatal("a request with neither field was accepted")
	} else {
		assertStatus(t, err, http.StatusBadRequest)
	}
}

// The registry is CLOSED and an unknown key is REJECTED, not dropped — 400, at
// the request boundary, because a malformed field belongs there.
func TestBenchSectionPrefs_UnknownSectionRejects400(t *testing.T) {
	for _, key := range []string{"stack", "caption", "phead", "sechead", "", "rsvp,rows"} {
		h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
		_, err := servePrefs(h, "section="+key, "user-1")
		if err == nil {
			t.Errorf("section=%q was accepted; the registry is four keys and it is closed", key)
			continue
		}
		assertStatus(t, err, http.StatusBadRequest)
	}
}

// Every real key is accepted, so the registry and the route cannot drift apart.
func TestBenchSectionPrefs_EveryRegistryKeyIsAccepted(t *testing.T) {
	for _, key := range benchSectionKeys {
		h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
		rec, err := servePrefs(h, "section="+key, "user-1")
		if err != nil {
			t.Errorf("section=%s: %v", key, err)
			continue
		}
		if rec.Code != http.StatusNoContent {
			t.Errorf("section=%s: status = %d, want 204", key, rec.Code)
		}
	}
}

// §12.1, inherited: user_id comes from the SESSION and nowhere else. With no
// session the write cannot happen and the route says so rather than writing to
// a row it invented.
func TestBenchSectionPrefs_AnonymousCannotWrite(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	if _, err := servePrefs(h, "section=rsvp", ""); err == nil {
		t.Fatal("an anonymous section toggle was accepted")
	}
}

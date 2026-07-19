// recurrence_type_xss_test.go — regression pins for C-SEC-XSS-JSATTR-SWEEP-R1
// sink 2: a session's RecurrenceType flowed verbatim into the edit-session
// modal's Alpine `x-data` expression (`recType: '%s'`), and — unlike
// CreateSession — UpdateSession never validated it, while the JSON PUT handler
// binds the body unchecked. So an attacker could persist `');<payload>//` as a
// RecurrenceType; the edit modal (rendered for every isScribe viewer) then
// executed it. Cross-user stored XSS.
//
// The fix is two layers, pinned here:
//   1. Write path — UpdateSession rejects any non-nil RecurrenceType outside the
//      legitimate enum set (service-level test below).
//   2. Sink — jsEsc escapes the value in the edit modal, so even a hostile value
//      that reached storage through some other path cannot break out (render
//      test below). See parent_selector_xss_test.go for why the rendered
//      discriminator is `\&#39;` (escaped) vs `&#39;&#39;` (broken out).

package sessions

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestUpdateSession_RefusesUnknownRecurrenceType pins the write-path validation:
// UpdateSession must reject an unknown/hostile RecurrenceType with a 400 and must
// not persist it (repo.Update is never reached).
func TestUpdateSession_RefusesUnknownRecurrenceType(t *testing.T) {
	cases := map[string]string{
		"xss_payload":  `');alert(1)//`,
		"unknown_enum": "daily", // looks plausible, is not a legitimate value
	}
	for name, badType := range cases {
		t.Run(name, func(t *testing.T) {
			bad := badType
			updateCalled := false
			repo := &mockSessionRepo{
				findByIDFn: func(_ context.Context, id string) (*Session, error) {
					return &Session{ID: id, Name: "Game", Status: StatusPlanned}, nil
				},
				updateFn: func(_ context.Context, _ *Session) error {
					updateCalled = true
					return nil
				},
			}
			svc := newTestSessionService(repo)

			_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
				Name:           "Game",
				Status:         StatusPlanned,
				RecurrenceType: &bad,
			})
			assertAppError(t, err, 400)
			if updateCalled {
				t.Errorf("UpdateSession persisted a hostile RecurrenceType (%q); it must reject before repo.Update", bad)
			}
		})
	}
}

// TestUpdateSession_AcceptsValidAndNilRecurrenceType pins that the new validation
// does not regress legitimate updates: every enum value passes, and a nil type
// (turning recurrence off, the common case) passes.
func TestUpdateSession_AcceptsValidAndNilRecurrenceType(t *testing.T) {
	valid := []*string{nil}
	for _, rt := range []string{RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly, RecurrenceCustom} {
		v := rt
		valid = append(valid, &v)
	}
	for _, rt := range valid {
		repo := &mockSessionRepo{
			findByIDFn: func(_ context.Context, id string) (*Session, error) {
				return &Session{ID: id, Name: "Game", Status: StatusPlanned}, nil
			},
			updateFn: func(_ context.Context, _ *Session) error { return nil },
		}
		svc := newTestSessionService(repo)

		_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
			Name:           "Game",
			Status:         StatusPlanned,
			IsRecurring:    rt != nil,
			RecurrenceType: rt,
		})
		if err != nil {
			got := "nil"
			if rt != nil {
				got = *rt
			}
			t.Errorf("UpdateSession rejected a legitimate RecurrenceType (%s): %v", got, err)
		}
	}
}

// TestEditSessionModal_EscapesHostileRecurrenceType pins the sink escape (layer
// 2): even if a hostile RecurrenceType reached storage, the edit modal must
// render it escaped so it cannot break out of the recType JS string literal.
func TestEditSessionModal_EscapesHostileRecurrenceType(t *testing.T) {
	payload := `');alert(1)//`
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}}
	session := &Session{ID: "sess-1", CampaignID: "camp-1", Name: "Game", Status: StatusPlanned, RecurrenceType: &payload}

	var buf bytes.Buffer
	if err := editSessionModal(cc, session, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "&#39;&#39;);alert(1)") {
		t.Errorf("edit-session modal let a hostile RecurrenceType break out of recType; jsEsc missing at the sink\nrendered: %s", html)
	}
	if !strings.Contains(html, `\&#39;);alert(1)`) {
		t.Errorf("expected the injected quote to be backslash-escaped (jsEsc) in recType; not found\nrendered: %s", html)
	}
}

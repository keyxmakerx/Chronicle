package campaigns

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

type testMailerStub struct {
	configured bool
	sends      int
	to         []string
	subject    string
	plain      string
	html       string
}

func (m *testMailerStub) SendHTMLMail(_ context.Context, to []string, subject, plain, html string) error {
	m.sends++
	m.to, m.subject, m.plain, m.html = to, subject, plain, html
	return nil
}

func (m *testMailerStub) IsConfigured(context.Context) bool { return m.configured }

func TestSendTestEmailAPI(t *testing.T) {
	owner := &auth.Session{UserID: "u1", Email: "owner@x.test", Name: "Kara"}
	cases := []struct {
		name     string
		role     Role
		session  *auth.Session
		body     map[string]any
		mailer   *testMailerStub
		wantCode int // 0 = sent
	}{
		{"a player can't send one", RolePlayer, owner, map[string]any{"layout": "note"}, &testMailerStub{configured: true}, http.StatusForbidden},
		{"a scribe can't send one", RoleScribe, owner, map[string]any{"layout": "note"}, &testMailerStub{configured: true}, http.StatusForbidden},
		{"an unknown layout is refused", RoleOwner, owner, map[string]any{"layout": "fancy"}, &testMailerStub{configured: true}, http.StatusBadRequest},
		{"no address on the account", RoleOwner, &auth.Session{UserID: "u1"}, map[string]any{"layout": "note"}, &testMailerStub{configured: true}, http.StatusBadRequest},
		{"the site can't send email", RoleOwner, owner, map[string]any{"layout": "note"}, &testMailerStub{}, http.StatusBadRequest},
		{"no mailer wired", RoleOwner, owner, map[string]any{"layout": "note"}, nil, http.StatusBadRequest},
		{"sent in the picked layout", RoleOwner, owner, map[string]any{"layout": "ticket", "accent": "#2563eb"}, &testMailerStub{configured: true}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&appearanceSvcStub{})
			h.baseURL = "https://x.test"
			if tc.mailer != nil {
				h.SetTestMailer(tc.mailer)
			}
			raw, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/appearance/email-test", bytes.NewReader(raw))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set(contextKeyCampaign, ownerContext(tc.role, "", nil))
			auth.SetSession(c, tc.session)

			err := h.SendTestEmailAPI(c)
			if tc.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want code %d", err, tc.wantCode)
				}
				if tc.mailer != nil && tc.mailer.sends != 0 {
					t.Error("a refused test was still sent")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			m := tc.mailer
			if m.sends != 1 || len(m.to) != 1 || m.to[0] != "owner@x.test" {
				t.Fatalf("sent %d to %v, want once to the owner's own address", m.sends, m.to)
			}
			if !strings.Contains(rec.Body.String(), `"to":"owner@x.test"`) {
				t.Errorf("response = %s, want it to name the address", rec.Body.String())
			}
			if m.subject != "Test: how Test's emails look" {
				t.Errorf("subject = %q", m.subject)
			}
			// The Ticket layout draws the stub in the picked colour.
			for _, w := range []string{"background:#2563eb", "FRI", "7pm"} {
				if !strings.Contains(m.html, w) {
					t.Errorf("HTML is missing %q", w)
				}
			}
			if strings.Contains(m.html, "/rsvp/") || strings.Contains(m.plain, "/rsvp/") {
				t.Error("a test must not carry a real answer link")
			}
		})
	}
}

func TestSampleEmail(t *testing.T) {
	h := &Handler{baseURL: "https://x.test"}
	c := &Campaign{ID: "c1", Name: "Therin", Settings: `{"accent_color":"#10b981"}`}
	for _, tc := range []struct {
		name    string
		now     time.Time
		wantDay string
	}{
		{"a Tuesday points at that week's Friday", time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC), "Friday 6 November"},
		{"a Friday points at the next one", time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), "Friday 13 November"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := h.sampleEmail(c, "Kara", "DM", "", tc.now)
			if !strings.HasPrefix(m.Lines[0], tc.wantDay+", 7pm") {
				t.Errorf("first line = %q, want %s", m.Lines[0], tc.wantDay)
			}
			if m.Accent != "#10b981" {
				t.Errorf("no colour sent should use the saved accent, got %q", m.Accent)
			}
			if m.Sender == nil || m.Sender.Name != "Kara" || m.Sender.Role != "your DM" {
				t.Errorf("sender = %+v", m.Sender)
			}
			for _, a := range append(m.Answers, m.Links...) {
				if a.URL != "https://x.test/campaigns/c1/customize/look#email" {
					t.Errorf("%s goes to %q, want back to Customize", a.Label, a.URL)
				}
			}
		})
	}
	if m := h.sampleEmail(c, " ", "GM", "#2563eb", time.Now()); m.Sender != nil || m.Accent != "#2563eb" {
		t.Errorf("no name means no sender, and a sent colour wins: %+v %q", m.Sender, m.Accent)
	}
}

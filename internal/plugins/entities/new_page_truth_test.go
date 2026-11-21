// new_page_truth_test.go pins that the sentence Create page shows about who
// will see the new page is true to what Create then saves. The form used to
// say "Everyone" while a campaign whose pages start hidden saved the page
// hidden, and an explicit "show it to players" was lost on the way.
package entities

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var (
	isPrivateField = regexp.MustCompile(`name="is_private" value="(true|false)"`)
	// The sentence as drawn, not the script text that also holds both.
	whoSentence = regexp.MustCompile(`data-np-who-text>([^<]*)<`)
)

// campaignContextFor is a campaign whose settings carry defaultVis, seen by a
// member of the given role.
func campaignContextFor(defaultVis string, role campaigns.Role) *campaigns.CampaignContext {
	settings := "{}"
	if defaultVis != "" {
		settings = `{"default_visibility":"` + defaultVis + `"}`
	}
	return &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Test", Settings: settings},
		MemberRole: role,
		IsMember:   true,
	}
}

func echoContext(method, target, body string, cc *campaigns.CampaignContext) *echo.Context {
	req := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
	if body != "" {
		req.Header.Set("Content-Type", echo.MIMEApplicationForm)
	}
	c := echo.New().NewContext(req, httptest.NewRecorder())
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set("campaign_context", cc)
	return c
}

func TestNewPage_SentenceMatchesWhatIsSaved(t *testing.T) {
	cases := []struct {
		name       string
		defaultVis string
		role       campaigns.Role
		// flip presses the quiet word once before saving.
		flip bool
		// wantSaid is the sentence the form opens with; wantSaved what is stored.
		wantHiddenLine bool
		wantSaved      bool
	}{
		{"owner, pages start shown", "", campaigns.RoleOwner, false, false, false},
		{"owner, pages start hidden", "dm_only", campaigns.RoleOwner, false, true, true},
		{"owner, pages start hidden, private word", "private", campaigns.RoleOwner, false, true, true},
		{"owner shows a page in a campaign that starts hidden", "dm_only", campaigns.RoleOwner, true, true, false},
		{"owner hides a page in a campaign that starts shown", "", campaigns.RoleOwner, true, false, true},
		{"scribe, campaign starts hidden: the page is open to players anyway", "dm_only", campaigns.RoleScribe, false, false, false},
		{"scribe, campaign starts shown", "", campaigns.RoleScribe, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubCreateCapturingSvc{}
			h := &Handler{service: svc}
			cc := campaignContextFor(tc.defaultVis, tc.role)

			// The form, as the sheet fetches it.
			get := echoContext(http.MethodGet, "/campaigns/camp-1/entities/new?sheet=1&type=7", "", cc)
			if err := h.NewForm(get); err != nil {
				t.Fatalf("NewForm: %v", err)
			}
			html := get.Response().(interface{ Unwrap() http.ResponseWriter }).Unwrap().(*httptest.ResponseRecorder).Body.String()

			said := whoSentence.FindStringSubmatch(html)
			if said == nil {
				t.Fatalf("the form carries no sentence about who sees the page")
			}
			saysHidden := said[1] == newPageHiddenLine
			if !saysHidden && said[1] != newPageShownLine {
				t.Fatalf("unexpected sentence %q", said[1])
			}
			if saysHidden != tc.wantHiddenLine {
				t.Fatalf("form says hidden = %v, want %v", saysHidden, tc.wantHiddenLine)
			}

			// What the form would post: its own field, or the other value if
			// the quiet word was pressed once.
			form := url.Values{"name": {"Sister Odalys"}, "entity_type_id": {"7"}}
			m := isPrivateField.FindStringSubmatch(html)
			if tc.role >= campaigns.RoleOwner {
				if m == nil {
					t.Fatalf("an owner's form must carry is_private")
				}
				if (m[1] == "true") != saysHidden {
					t.Fatalf("the hidden field (%s) disagrees with the sentence (hidden=%v)", m[1], saysHidden)
				}
				v := m[1] == "true"
				if tc.flip {
					v = !v
				}
				form.Set("is_private", map[bool]string{true: "true", false: "false"}[v])
			} else if m != nil {
				t.Fatalf("a scribe's form must not carry is_private: they cannot hide a page")
			}

			post := echoContext(http.MethodPost, "/campaigns/camp-1/entities", form.Encode(), cc)
			if err := h.Create(post); err != nil && !strings.Contains(err.Error(), "redirect") {
				t.Fatalf("Create: %v", err)
			}
			if len(svc.created) != 1 {
				t.Fatalf("expected 1 Create call, got %d", len(svc.created))
			}
			if got := svc.created[0].IsPrivate; got != tc.wantSaved {
				t.Errorf("saved IsPrivate = %v, want %v", got, tc.wantSaved)
			}
		})
	}
}

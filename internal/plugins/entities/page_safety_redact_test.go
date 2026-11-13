package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// hideAllMentions stands in for the entities service: it reads every mention
// as a page the viewer can't open.
type hideAllMentions struct{ EntityService }

func (hideAllMentions) HideUnviewableMentions(_ context.Context, _ string, _ int, _ string, texts ...*string) (int, error) {
	n := 0
	for _, t := range texts {
		if t != nil && strings.Contains(*t, "data-mention-id") {
			*t = "a hidden page"
			n++
		}
	}
	return n, nil
}

func TestHistoryRedactFor(t *testing.T) {
	h := &PageSafetyHandler{entities: hideAllMentions{}}
	cc := func(role campaigns.Role) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c"}, MemberRole: role}
	}
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())

	if h.redactFor(c, cc(campaigns.RoleOwner)) != nil {
		t.Fatal("the owner reads versions whole")
	}
	redact := h.redactFor(c, cc(campaigns.RoleScribe))
	if redact == nil {
		t.Fatal("a scribe must get a redactor")
	}
	html := `<p>open <span data-secret="true">SECRETWORD</span></p>`
	v := &EntityVersion{EntryHTML: &html}
	redact(v)
	if strings.Contains(*v.EntryHTML, "SECRETWORD") {
		t.Errorf("secret passage survived: %s", *v.EntryHTML)
	}
	if !strings.Contains(*v.EntryHTML, "open") {
		t.Errorf("visible text was lost: %s", *v.EntryHTML)
	}
	mention := `<a data-mention-id="x" href="/p">HIDDENNAME</a>`
	m := &EntityVersion{EntryHTML: &mention}
	redact(m)
	if strings.Contains(*m.EntryHTML, "HIDDENNAME") {
		t.Errorf("hidden page's name survived: %s", *m.EntryHTML)
	}
}

package rolltables

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func ctxFor(role campaigns.Role, dmGrant bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  role,
		IsMember:    true,
		IsDmGranted: dmGrant,
	}
}

func call(h *Handler, method string, cc *campaigns.CampaignContext, body string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/camp-1/roll-tables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	if cc != nil {
		c.Set("campaign_context", cc)
	}
	if method == http.MethodPut {
		return rec, h.Put(c)
	}
	return rec, h.Get(c)
}

const sampleBody = `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"Gold","brief":"A purse","weight":3}]}]}`

// Reads are open to scribes and the DM team; writes are DM-team only, and a
// refused PUT stores nothing.
func TestHandler_Gating(t *testing.T) {
	tests := []struct {
		name    string
		cc      *campaigns.CampaignContext
		wantGet int
		wantPut int
	}{
		{"player", ctxFor(campaigns.RolePlayer, false), http.StatusForbidden, http.StatusForbidden},
		{"scribe without dm access", ctxFor(campaigns.RoleScribe, false), http.StatusOK, http.StatusForbidden},
		{"no campaign context", nil, http.StatusInternalServerError, http.StatusInternalServerError},
		{"player with dm access", ctxFor(campaigns.RolePlayer, true), http.StatusOK, http.StatusOK},
		{"owner", ctxFor(campaigns.RoleOwner, false), http.StatusOK, http.StatusOK},
	}
	for _, tt := range tests {
		for method, want := range map[string]int{http.MethodGet: tt.wantGet, http.MethodPut: tt.wantPut} {
			t.Run(tt.name+" "+method, func(t *testing.T) {
				repo := newFakeRepo()
				h := NewHandler(NewService(repo))
				rec, err := call(h, method, tt.cc, sampleBody)
				got := rec.Code
				if err != nil {
					got = errCode(err)
				}
				if got != want {
					t.Fatalf("status = %d (err %v), want %d", got, err, want)
				}
				if method == http.MethodPut && want != http.StatusOK && repo.puts != 0 {
					t.Errorf("a refused request wrote")
				}
			})
		}
	}
}

func TestHandler_HappyPath(t *testing.T) {
	h := NewHandler(NewService(newFakeRepo()))
	owner := ctxFor(campaigns.RoleOwner, false)

	rec, err := call(h, http.MethodGet, owner, "")
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("empty get: %d %v", rec.Code, err)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"tables":[]}` {
		t.Errorf("empty get body = %s", got)
	}

	rec, err = call(h, http.MethodPut, owner, sampleBody)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("put: %d %v", rec.Code, err)
	}
	want := `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"Gold","brief":"A purse","weight":3}]}]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("put body = %s", got)
	}

	rec, _ = call(h, http.MethodGet, owner, "")
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("get after put = %s", got)
	}
}

func TestHandler_PutInvalidBody(t *testing.T) {
	h := NewHandler(NewService(newFakeRepo()))
	_, err := call(h, http.MethodPut, ctxFor(campaigns.RoleOwner, false), `{"tables":[{"id":"BAD","name":"x","entries":[{"name":"a"}]}]}`)
	if errCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d (err %v), want 422", errCode(err), err)
	}
}

package armory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeBuySvc struct {
	in    BuyInput
	actor Actor
	calls int
	err   error
	// answered is the request id of the last approve or decline.
	answered     int64
	answerStatus string
}

func (f *fakeBuySvc) Buyers(_ context.Context, _, _ string, a Actor) (*BuyersView, error) {
	f.actor = a
	return &BuyersView{Buyers: []Buyer{}}, f.err
}

func (f *fakeBuySvc) Buy(_ context.Context, _, _ string, a Actor, in BuyInput) (*BuyResult, error) {
	f.calls++
	f.in, f.actor = in, a
	return &BuyResult{Status: BuyStatusRequested}, f.err
}

func (f *fakeBuySvc) ApproveRequest(_ context.Context, _ string, a Actor, id int64) (*PurchaseRequest, error) {
	f.actor, f.answered = a, id
	return &PurchaseRequest{ID: id, Status: f.answerStatus}, f.err
}

func (f *fakeBuySvc) DeclineRequest(_ context.Context, _ string, a Actor, id int64) (*PurchaseRequest, error) {
	f.actor, f.answered = a, id
	return &PurchaseRequest{ID: id, Status: PurchaseDeclined}, f.err
}

func TestShopBuyHandler_Buy(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		err      error
		wantCode int
		wantCall bool
	}{
		{"string and number ids", `{"buyerEntityId":"c1","items":[{"relationId":"7","quantity":2},{"relationId":8,"quantity":1}]}`, nil, 200, true},
		{"malformed json", `{`, nil, 400, false},
		{"fractional quantity", `{"buyerEntityId":"c1","items":[{"relationId":1,"quantity":1.5}]}`, nil, 400, false},
		{"bad relation id", `{"buyerEntityId":"c1","items":[{"relationId":"abc","quantity":1}]}`, nil, 400, false},
		{"oversize body", `{"buyerEntityId":"` + strings.Repeat("a", maxBuyBodyBytes) + `"}`, nil, 400, false},
		{"service refusal passes through", `{"buyerEntityId":"c1","items":[]}`, apperror.NewForbidden("no"), 403, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeBuySvc{err: tc.err}
			h := NewShopBuyHandler(svc)
			c, rec := roomCtx(http.MethodPost, tc.body, campaigns.RolePlayer)
			err := h.Buy(c)
			got := rec.Code
			if err != nil {
				got = code(err)
			}
			if got != tc.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tc.wantCode)
			}
			if (svc.calls == 1) != tc.wantCall {
				t.Errorf("service calls = %d", svc.calls)
			}
		})
	}

	t.Run("binding", func(t *testing.T) {
		svc := &fakeBuySvc{}
		c, _ := roomCtx(http.MethodPost, `{"buyerEntityId":"c1","items":[{"relationId":"7","quantity":2},{"relationId":8,"quantity":1}]}`, campaigns.RolePlayer)
		if err := NewShopBuyHandler(svc).Buy(c); err != nil {
			t.Fatal(err)
		}
		if svc.in.BuyerEntityID != "c1" || len(svc.in.Items) != 2 || svc.in.Items[0].RelationID != 7 || svc.in.Items[0].Quantity != 2 || svc.in.Items[1].RelationID != 8 {
			t.Errorf("bound %+v", svc.in)
		}
	})
}

// A DM-granted co-DM reaches the service as an Owner-visibility actor.
func TestShopBuyHandler_ActorRole(t *testing.T) {
	svc := &fakeBuySvc{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
	})
	if err := NewShopBuyHandler(svc).Buyers(c); err != nil {
		t.Fatal(err)
	}
	if svc.actor.Role != rOwner {
		t.Errorf("role = %d, want Owner visibility", svc.actor.Role)
	}
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"buyers":[]`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

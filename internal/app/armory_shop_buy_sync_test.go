package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
)

// shopBuyMembers is a fixed campaign roster for the acting-as rule.
type shopBuyMembers map[string]int

func (m shopBuyMembers) MemberRole(_ context.Context, _, userID string) (int, bool, error) {
	r, ok := m[userID]
	return r, ok, nil
}

func (m shopBuyMembers) IsDmGranted(context.Context, string, string) (bool, error) { return false, nil }

// shopBuyRecorder records the actor and basket each call was made with.
type shopBuyRecorder struct {
	actor armory.Actor
	in    armory.BuyInput
	src   changesource.Source
	calls int
}

func (r *shopBuyRecorder) Buyers(_ context.Context, _, _ string, a armory.Actor) (*armory.BuyersView, error) {
	r.calls++
	r.actor = a
	return &armory.BuyersView{Buyers: []armory.Buyer{}}, nil
}

func (r *shopBuyRecorder) Buy(ctx context.Context, _, _ string, a armory.Actor, in armory.BuyInput) (*armory.BuyResult, error) {
	r.calls++
	r.src, _ = changesource.From(ctx)
	r.actor, r.in = a, in
	return &armory.BuyResult{Status: armory.BuyStatusBought}, nil
}

// The Foundry buy path runs as the named member under their own role, and
// only a GM's key may name someone else.
func TestSyncShopBuyAdapter_ActsAsMember(t *testing.T) {
	members := shopBuyMembers{"gm": permissions.RoleOwner, "p1": permissions.RolePlayer, "p2": permissions.RolePlayer}
	body := json.RawMessage(`{"actingUserId":"p1","buyerEntityId":"char-1","items":[{"relationId":"7","quantity":2}]}`)
	cases := []struct {
		name      string
		key       string
		acting    string
		wantCode  int
		wantActor armory.Actor
	}{
		{"GM key buys as the named player, with the player's role", "gm", "p1", 0, armory.Actor{UserID: "p1", Role: permissions.RolePlayer}},
		{"GM key without a name buys as the GM", "gm", "", 0, armory.Actor{UserID: "gm", Role: permissions.RoleOwner}},
		{"a player's key cannot name someone else", "p2", "p1", 403, armory.Actor{}},
		{"a name that is not a member is not found", "gm", "stranger", 404, armory.Actor{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &shopBuyRecorder{}
			a := &syncShopBuyAPIAdapter{actors: armory.NewStashAPI(nil, members), buy: rec}
			for _, call := range []func() error{
				func() error { _, err := a.Buyers(context.Background(), "c", tc.key, tc.acting, "shop"); return err },
				func() error { _, err := a.Buy(context.Background(), "c", tc.key, tc.acting, "shop", body); return err },
			} {
				err := call()
				if tc.wantCode != 0 {
					var appErr *apperror.AppError
					if !errors.As(err, &appErr) || appErr.Code != tc.wantCode {
						t.Fatalf("want %d, got %v", tc.wantCode, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if rec.actor != tc.wantActor {
					t.Fatalf("actor = %+v, want %+v", rec.actor, tc.wantActor)
				}
			}
			if tc.wantCode != 0 && rec.calls != 0 {
				t.Fatalf("service ran on a refused call")
			}
			if tc.wantCode == 0 && (rec.in.BuyerEntityID != "char-1" || len(rec.in.Items) != 1 || rec.in.Items[0].RelationID != 7 || rec.in.Items[0].Quantity != 2) {
				t.Fatalf("basket = %+v", rec.in)
			}
			if tc.wantCode == 0 && (rec.src.Kind != changesource.KindShop || rec.src.UserID != tc.wantActor.UserID || rec.src.Label != foundryShopLabel) {
				t.Fatalf("purchase source = %+v, want a Foundry shop purchase by %s", rec.src, tc.wantActor.UserID)
			}
		})
	}
}

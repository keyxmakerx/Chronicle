package entities

import (
	"context"
	"strings"
	"testing"
)

// EnsureShopGood reuses a shop good of the same name and never takes over an
// ordinary page; a good sold to players never sits on a page hidden from them.
func TestEnsureShopGood(t *testing.T) {
	type call struct {
		created   *Entity
		unhidden  bool
		lookedUp  string
		campaign  string
		createdAt int
	}
	tests := []struct {
		name         string
		existing     *Entity
		goodName     string
		hidden       bool
		wantErr      int
		wantCreated  bool
		wantPrivate  bool
		wantUnhidden bool
	}{
		{name: "makes a good with no page yet", goodName: "Crowbar", wantCreated: true},
		{name: "a hidden listing makes a hidden page", goodName: "Crowbar", hidden: true, wantCreated: true, wantPrivate: true},
		{name: "two shops share one page", existing: &Entity{ID: "g1", Name: "Crowbar", ShopGood: true}, goodName: " crowbar "},
		{name: "a shown listing un-hides a hidden page", existing: &Entity{ID: "g1", Name: "Crowbar", ShopGood: true, IsPrivate: true}, goodName: "Crowbar", wantUnhidden: true},
		{name: "a hidden listing leaves a shown page", existing: &Entity{ID: "g1", Name: "Crowbar", ShopGood: true}, goodName: "Crowbar", hidden: true},
		{name: "no name", goodName: "   ", wantErr: 400},
		{name: "name too long", goodName: strings.Repeat("a", 201), wantErr: 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c call
			repo := &mockEntityRepo{
				findShopGoodFn: func(_ context.Context, campaignID, name string) (*Entity, error) {
					c.lookedUp, c.campaign = name, campaignID
					return tc.existing, nil
				},
				createFn: func(_ context.Context, e *Entity) error { c.created = e; return nil },
				updatePrivateFn: func(_ context.Context, _ string, private bool) error {
					c.unhidden = !private
					return nil
				},
			}
			types := &mockEntityTypeRepo{findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
				return &EntityType{ID: id, CampaignID: "camp-1", Slug: "item"}, nil
			}}
			svc := newTestService(repo, types)
			got, err := svc.EnsureShopGood(context.Background(), "camp-1", "user-1", 3, tc.goodName, tc.hidden)
			if tc.wantErr != 0 {
				assertAppError(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.campaign != "camp-1" {
				t.Errorf("lookup must be scoped to the campaign, got %q", c.campaign)
			}
			if (c.created != nil) != tc.wantCreated {
				t.Fatalf("created = %v, want %v", c.created != nil, tc.wantCreated)
			}
			if tc.wantCreated {
				if !c.created.ShopGood || c.created.IsPrivate != tc.wantPrivate || got.ID != c.created.ID {
					t.Errorf("created page = %+v", c.created)
				}
			} else if got.ID != "g1" {
				t.Errorf("reused page = %q", got.ID)
			}
			if c.unhidden != tc.wantUnhidden {
				t.Errorf("unhidden = %v, want %v", c.unhidden, tc.wantUnhidden)
			}
		})
	}
}

func TestSetShopGood_PassesCampaign(t *testing.T) {
	var gotCampaign string
	var gotMark = true
	repo := &mockEntityRepo{setShopGoodFn: func(_ context.Context, campaignID, _ string, mark bool) error {
		gotCampaign, gotMark = campaignID, mark
		return nil
	}}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	if err := svc.SetShopGood(context.Background(), "camp-1", "e1", false); err != nil {
		t.Fatal(err)
	}
	if gotCampaign != "camp-1" || gotMark {
		t.Errorf("repo called with %q, %v", gotCampaign, gotMark)
	}
}

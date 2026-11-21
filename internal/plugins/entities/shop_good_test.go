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
		created  *Entity
		unhidden bool
		lookedUp string
		campaign string
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
		{name: "an existing page keeps its visibility here; listings reconcile it", existing: &Entity{ID: "g1", Name: "Crowbar", ShopGood: true, IsPrivate: true}, goodName: "Crowbar"},
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

// A page used only by hidden listings is hidden; one public listing anywhere
// makes it public; the last public listing going hides it again. Ordinary
// pages are never touched.
func TestReconcileShopGoodVisibility(t *testing.T) {
	tests := []struct {
		name       string
		page       *Entity
		public     int
		wantWrite  bool
		wantHidden bool
	}{
		{"only hidden listings hides a public page", &Entity{ID: "g", CampaignID: "c", ShopGood: true}, 0, true, true},
		{"a public listing shows a hidden page", &Entity{ID: "g", CampaignID: "c", ShopGood: true, IsPrivate: true}, 1, true, false},
		{"two public listings keep it public", &Entity{ID: "g", CampaignID: "c", ShopGood: true}, 2, false, false},
		{"already hidden and no public listing", &Entity{ID: "g", CampaignID: "c", ShopGood: true, IsPrivate: true}, 0, false, true},
		{"an ordinary page is untouched", &Entity{ID: "g", CampaignID: "c"}, 0, false, false},
		{"another campaign's page is untouched", &Entity{ID: "g", CampaignID: "other", ShopGood: true}, 0, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrote, hidden := false, false
			repo := &mockEntityRepo{
				findByIDFn: func(context.Context, string) (*Entity, error) { return tc.page, nil },
				updatePrivateFn: func(_ context.Context, _ string, private bool) error {
					wrote, hidden = true, private
					return nil
				},
			}
			svc := newTestService(repo, &mockEntityTypeRepo{})
			if err := svc.ReconcileShopGoodVisibility(context.Background(), "c", "g", tc.public); err != nil {
				t.Fatal(err)
			}
			if wrote != tc.wantWrite || (wrote && hidden != tc.wantHidden) {
				t.Errorf("wrote=%v hidden=%v, want wrote=%v hidden=%v", wrote, hidden, tc.wantWrite, tc.wantHidden)
			}
		})
	}
}

type eventLog struct {
	NoopEntityEventPublisher
	got []string
}

func (l *eventLog) PublishEntityEvent(kind, _, id string, _ *Entity) {
	l.got = append(l.got, kind+":"+id)
}

// Sync hears nothing about a shop good until it is promoted to a full page,
// which it hears as a new page; making a page a shop good withdraws it.
func TestShopGoodEvents(t *testing.T) {
	page := &Entity{ID: "g1", CampaignID: "c", ShopGood: true}
	repo := &mockEntityRepo{
		createFn:       func(context.Context, *Entity) error { return nil },
		findShopGoodFn: func(context.Context, string, string) (*Entity, error) { return nil, nil },
		setShopGoodFn: func(_ context.Context, _, _ string, mark bool) error {
			page.ShopGood = mark
			return nil
		},
		findByIDFn: func(context.Context, string) (*Entity, error) { c := *page; return &c, nil },
	}
	types := &mockEntityTypeRepo{findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
		return &EntityType{ID: id, CampaignID: "c", Slug: "item"}, nil
	}}
	svc := newTestService(repo, types)
	log := &eventLog{}
	svc.SetEventPublisher(log)

	if _, err := svc.EnsureShopGood(context.Background(), "c", "u", 1, "Crowbar", false); err != nil {
		t.Fatal(err)
	}
	if len(log.got) != 0 {
		t.Fatalf("making a shop good announced %v", log.got)
	}
	// An ordinary rename of a shop good is silent too.
	svc.(*entityService).publishEntity(context.Background(), "updated", "c", "g1", page)
	if len(log.got) != 0 {
		t.Fatalf("an update of a shop good announced %v", log.got)
	}
	if err := svc.SetShopGood(context.Background(), "c", "g1", false); err != nil {
		t.Fatal(err)
	}
	if len(log.got) != 1 || log.got[0] != "created:g1" {
		t.Fatalf("promotion announced %v, want one created", log.got)
	}
	log.got = nil
	if err := svc.SetShopGood(context.Background(), "c", "g1", true); err != nil {
		t.Fatal(err)
	}
	if len(log.got) != 1 || log.got[0] != "deleted:g1" {
		t.Fatalf("demotion announced %v, want one deleted", log.got)
	}
}

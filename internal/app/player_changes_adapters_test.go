package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/playerchanges"
)

// recordingChanges is a playerchanges.Service that only keeps Record calls.
type recordingChanges struct {
	playerchanges.Service
	got []playerchanges.Change
}

func (r *recordingChanges) Record(_ context.Context, c playerchanges.Change) error {
	r.got = append(r.got, c)
	return nil
}

type pcMembers map[string]campaigns.Role

func (f pcMembers) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	role, ok := f[userID]
	if !ok {
		return nil, nil
	}
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: role, DisplayName: "Name " + userID}, nil
}

type pcTypes struct{}

func (pcTypes) GetEntityTypeByID(context.Context, int) (*entities.EntityType, error) {
	return &entities.EntityType{Fields: []entities.FieldDefinition{{Key: "hp", Label: "Stamina"}}}, nil
}

func newRecorder() (*playerChangeRecorder, *recordingChanges) {
	svc := &recordingChanges{}
	return &playerChangeRecorder{svc: svc, types: pcTypes{},
		members: pcMembers{"bea": campaigns.RolePlayer, "km": campaigns.RoleOwner, "sam": campaigns.RoleScribe}}, svc
}

func TestPlayerChangeRecorder_FieldsChanged(t *testing.T) {
	page := &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tyne"}
	tests := []struct {
		name     string
		src      *changesource.Source
		old, new map[string]any
		want     int
		wantSrc  string
	}{
		{"a player's web edit", &changesource.Source{Kind: changesource.KindWeb, UserID: "bea"},
			map[string]any{"hp": 10.0}, map[string]any{"hp": 8.0}, 1, playerchanges.SourceWeb},
		{"a player's AI", &changesource.Source{Kind: changesource.KindAI, UserID: "bea", Label: "Claude"},
			map[string]any{"hp": 10.0}, map[string]any{"hp": 8.0}, 1, playerchanges.SourceAI},
		{"the owner is not recorded", &changesource.Source{Kind: changesource.KindWeb, UserID: "km"},
			map[string]any{"hp": 10.0}, map[string]any{"hp": 8.0}, 0, ""},
		{"a scribe is not recorded", &changesource.Source{Kind: changesource.KindWeb, UserID: "sam"},
			map[string]any{"hp": 10.0}, map[string]any{"hp": 8.0}, 0, ""},
		{"a shop's side effect is not recorded", &changesource.Source{Kind: changesource.KindShop, UserID: "bea"},
			map[string]any{"gp": 10.0}, map[string]any{"gp": 8.0}, 0, ""},
		{"no source is not recorded", nil, map[string]any{"hp": 10.0}, map[string]any{"hp": 8.0}, 0, ""},
		{"unchanged fields are skipped", &changesource.Source{Kind: changesource.KindWeb, UserID: "bea"},
			map[string]any{"hp": 10.0, "class": "Fury"}, map[string]any{"hp": 10.0, "class": "Tactician"}, 1, playerchanges.SourceWeb},
		{"internal keys are skipped", &changesource.Source{Kind: changesource.KindWeb, UserID: "bea"},
			map[string]any{"_sync": 1.0}, map[string]any{"_sync": 2.0}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, svc := newRecorder()
			ctx := context.Background()
			if tt.src != nil {
				ctx = changesource.With(ctx, *tt.src)
			}
			if err := r.FieldsChanged(ctx, page, tt.old, tt.new); err != nil {
				t.Fatal(err)
			}
			if len(svc.got) != tt.want {
				t.Fatalf("recorded %d, want %d: %+v", len(svc.got), tt.want, svc.got)
			}
			if tt.want > 0 && svc.got[0].Source != tt.wantSrc {
				t.Errorf("source = %q, want %q", svc.got[0].Source, tt.wantSrc)
			}
		})
	}
}

func TestPlayerChangeRecorder_LabelsAndValues(t *testing.T) {
	r, svc := newRecorder()
	ctx := changesource.With(context.Background(), changesource.Source{Kind: changesource.KindWeb, UserID: "bea"})
	_ = r.FieldsChanged(ctx, &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tyne"},
		map[string]any{"hp": 10.0}, map[string]any{"hp": 8.0})
	c := svc.got[0]
	if c.FieldLabel != "Stamina" || c.OldText != "10" || c.NewText != "8" || c.OldJSON != "10" || c.NewJSON != "8" || c.UserName != "Name bea" {
		t.Errorf("change = %+v", c)
	}
}

func TestPlayerChangeRecorder_TextChanged(t *testing.T) {
	html := func(s string) *string { return &s }
	before := &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tyne", EntryHTML: html("<p>Born in the hills.</p>")}
	after := &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tyne Ash", EntryHTML: html("<p>Born in the green hills.</p>")}
	r, svc := newRecorder()
	if err := r.TextChanged(context.Background(), before, after, "bea"); err != nil {
		t.Fatal(err)
	}
	if len(svc.got) != 2 {
		t.Fatalf("recorded %d, want the name and the text", len(svc.got))
	}
	if n := svc.got[0]; n.FieldKey != "name" || n.OldText != "Tyne" || n.NewText != "Tyne Ash" {
		t.Errorf("name change = %+v", n)
	}
	if b := svc.got[1]; b.Kind != playerchanges.KindText || b.NewText != "green" || b.OldText != "" {
		t.Errorf("text change = %+v, want just the word added", b)
	}

	r, svc = newRecorder()
	_ = r.TextChanged(context.Background(), before, after, "km")
	if len(svc.got) != 0 {
		t.Error("the owner's own saves are not recorded")
	}
}

func TestPlayerChangeRecorder_PageTrashed(t *testing.T) {
	r, svc := newRecorder()
	_ = r.PageTrashed(context.Background(), &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tyne"}, "bea")
	if len(svc.got) != 1 || svc.got[0].Kind != playerchanges.KindRemoved || svc.got[0].PageName != "Tyne" {
		t.Fatalf("got %+v", svc.got)
	}
}

func TestTextDiff(t *testing.T) {
	tests := []struct {
		name, older, newer, gone, came string
	}{
		{"a word added", "the hills", "the green hills", "", "green"},
		{"a word taken out", "the green hills", "the hills", "green", ""},
		{"two runs read as excerpts", "a b c d e", "a x c d y", "b … e", "x … y"},
		{"neighbouring words stay one run", "a b c", "a d e c", "b", "d e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gone, came := textDiff(tt.older, tt.newer)
			if gone != tt.gone || came != tt.came {
				t.Errorf("textDiff = %q, %q; want %q, %q", gone, came, tt.gone, tt.came)
			}
		})
	}
}

// fieldObservers must hand page saves only to observers that want them.
func TestFieldObservers_FanOut(t *testing.T) {
	r, svc := newRecorder()
	plain := &pcCountingObserver{}
	f := fieldObservers{plain, r}
	ctx := changesource.With(context.Background(), changesource.Source{Kind: changesource.KindWeb, UserID: "bea"})
	e := &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tyne"}
	_ = f.FieldsChanged(ctx, e, map[string]any{"hp": 1.0}, map[string]any{"hp": 2.0})
	_ = f.PageTrashed(ctx, e, "bea")
	if plain.n != 1 || len(svc.got) != 2 {
		t.Fatalf("plain = %d, recorder = %d", plain.n, len(svc.got))
	}
}

type pcCountingObserver struct{ n int }

func (c *pcCountingObserver) FieldsChanged(context.Context, *entities.Entity, map[string]any, map[string]any) error {
	c.n++
	return nil
}

func TestKeyLabel(t *testing.T) {
	for in, want := range map[string]string{"culture": "Culture", "hero_tokens": "Hero tokens", "kit-name": "Kit name", "": ""} {
		if got := keyLabel(in); got != want {
			t.Errorf("keyLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

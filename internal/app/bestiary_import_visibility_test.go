// bestiary_import_visibility_test.go pins that
// bestiaryEntityCreatorAdapter.CreateFromStatblock sets IsPrivate from the
// campaign's DefaultVisibility setting: the bestiary plugin's EntityCreator
// interface has no is_private parameter or per-import visibility control, so
// the campaign default is the only input there is. Omitting it would import
// every creature visible to players even in a "DM Only" campaign.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubImportEntitySvc captures the CreateEntityInput the adapter builds.
type stubImportEntitySvc struct {
	entities.EntityService
	created []entities.CreateEntityInput
}

func (s *stubImportEntitySvc) GetEntityTypesByPresetCategory(_ context.Context, campaignID, _ string) ([]entities.EntityType, error) {
	return []entities.EntityType{{ID: 5, CampaignID: campaignID, Slug: "creature", Enabled: true}}, nil
}

func (s *stubImportEntitySvc) Create(_ context.Context, campaignID, _ string, input entities.CreateEntityInput) (*entities.Entity, error) {
	s.created = append(s.created, input)
	return &entities.Entity{ID: "ent-new", CampaignID: campaignID, Name: input.Name}, nil
}

// stubImportCampaignSvc serves a campaign whose settings JSON carries the
// default_visibility under test, or an error to exercise the fail-closed path.
type stubImportCampaignSvc struct {
	campaigns.CampaignService
	defaultVis string
	err        error
	nilCamp    bool
}

func (s *stubImportCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.nilCamp {
		return nil, nil
	}
	settings := "{}"
	if s.defaultVis != "" {
		settings = `{"default_visibility":"` + s.defaultVis + `"}`
	}
	return &campaigns.Campaign{ID: id, Name: "Test", Settings: settings}, nil
}

func TestBestiaryImport_HonoursCampaignDefaultVisibility(t *testing.T) {
	cases := []struct {
		name       string
		defaultVis string
		want       bool
		why        string
	}{
		{
			name:       "dm_only default imports the creature hidden",
			defaultVis: "dm_only",
			want:       true,
			why:        "an imported boss statblock must not be visible to the party the moment it lands",
		},
		{
			name:       "private default imports the creature hidden",
			defaultVis: "private",
			want:       true,
			why:        "\"private\" is the other value the campaign settings page writes",
		},
		{
			name:       "no default leaves the import public",
			defaultVis: "",
			want:       false,
			why:        "a campaign that never set the default keeps today's behaviour",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			esvc := &stubImportEntitySvc{}
			a := &bestiaryEntityCreatorAdapter{
				svc:         esvc,
				campaignSvc: &stubImportCampaignSvc{defaultVis: tc.defaultVis},
			}

			id, err := a.CreateFromStatblock(context.Background(), "camp-1", "user-1", "Goblin Boss", "",
				json.RawMessage(`{"hp":21}`))
			if err != nil {
				t.Fatalf("CreateFromStatblock: %v", err)
			}
			if id != "ent-new" {
				t.Fatalf("returned id = %q, want ent-new", id)
			}
			if len(esvc.created) != 1 {
				t.Fatalf("expected 1 Create call, got %d", len(esvc.created))
			}
			if got := esvc.created[0].IsPrivate; got != tc.want {
				t.Errorf("IsPrivate = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}
}

// An unreadable campaign must not fall open. The import still succeeds — a
// transient settings read failure is no reason to break creature import —
// but it succeeds HIDDEN, because an unknown default cannot justify
// publishing. Over-hiding is a toggle away; a leak is not recoverable.
func TestBestiaryImport_UnreadableCampaignFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		svc  *stubImportCampaignSvc
	}{
		{"settings read errors", &stubImportCampaignSvc{err: errors.New("db down")}},
		{"campaign not found", &stubImportCampaignSvc{nilCamp: true}},
		{"no campaign service wired", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			esvc := &stubImportEntitySvc{}
			a := &bestiaryEntityCreatorAdapter{svc: esvc}
			if tc.svc != nil {
				a.campaignSvc = tc.svc
			}

			if _, err := a.CreateFromStatblock(context.Background(), "camp-1", "user-1", "Goblin Boss", "",
				json.RawMessage(`{"hp":21}`)); err != nil {
				t.Fatalf("CreateFromStatblock: %v", err)
			}
			if len(esvc.created) != 1 {
				t.Fatalf("expected 1 Create call, got %d", len(esvc.created))
			}
			if !esvc.created[0].IsPrivate {
				t.Error("IsPrivate = false when the campaign default could not be read; an unknown default must fail closed, not publish")
			}
		})
	}
}

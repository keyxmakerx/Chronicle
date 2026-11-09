// create_default_visibility_test.go pins that both REST creation doors —
// POST /api/v1/campaigns/:id/entities and the batch-sync "create" branch —
// apply the campaign's DefaultVisibility setting when is_private is omitted,
// rather than defaulting to public. See entity_partial_update_test.go for
// the related partial-update contract.
package syncapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubCampaignSvcWithSettings answers resolveRole's membership lookup AND
// the settings read the create paths now make.
type stubCampaignSvcWithSettings struct {
	campaigns.CampaignService
	defaultVis string
	getByIDErr error
	getByIDs   int
}

func (s *stubCampaignSvcWithSettings) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: campaigns.RoleOwner}, nil
}

func (s *stubCampaignSvcWithSettings) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	s.getByIDs++
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	settings := "{}"
	if s.defaultVis != "" {
		settings = `{"default_visibility":"` + s.defaultVis + `"}`
	}
	return &campaigns.Campaign{ID: id, Name: "Test", Settings: settings}, nil
}

// stubEntitySvcCapturingCreate records every CreateEntityInput built.
type stubEntitySvcCapturingCreate struct {
	entities.EntityService
	created []entities.CreateEntityInput
}

func (s *stubEntitySvcCapturingCreate) Create(_ context.Context, campaignID, _ string, input entities.CreateEntityInput) (*entities.Entity, error) {
	s.created = append(s.created, input)
	return &entities.Entity{ID: "ent-new", CampaignID: campaignID, Name: input.Name, IsPrivate: input.IsPrivate}, nil
}

func newCreateAPIContext(path, body string) *echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set(apiKeyContextKey, &APIKey{
		ID:         synthKeySessionID,
		CampaignID: "camp-1",
		UserID:     "user-1",
		IsActive:   true,
	})
	return c
}

// --- POST /api/v1/campaigns/:id/entities -----------------------------------

func TestCreateEntity_HonoursCampaignDefaultVisibility(t *testing.T) {
	cases := []struct {
		name       string
		defaultVis string
		body       string
		want       bool
		why        string
	}{
		{
			name:       "absent is_private under dm_only default starts private",
			defaultVis: "dm_only",
			body:       `{"name":"Hidden Contact","entity_type_id":3}`,
			want:       true,
			why:        "Foundry sync omits is_private; a value-typed bool decoded that as false and published the entity",
		},
		{
			name:       "absent is_private under private default starts private",
			defaultVis: "private",
			body:       `{"name":"Hidden Contact","entity_type_id":3}`,
			want:       true,
			why:        "\"private\" is the other value the campaign settings page writes",
		},
		{
			name:       "explicit false under dm_only default stays public",
			defaultVis: "dm_only",
			body:       `{"name":"Town Crier","entity_type_id":3,"is_private":false}`,
			want:       false,
			why:        "a client that deliberately sends is_private:false must be obeyed; that is why the field is three-state",
		},
		{
			name:       "explicit true with no default is private",
			defaultVis: "",
			body:       `{"name":"Secret Cache","entity_type_id":3,"is_private":true}`,
			want:       true,
			why:        "an explicit private request is honoured with or without a campaign default",
		},
		{
			name:       "absent with no default stays public",
			defaultVis: "",
			body:       `{"name":"Village Well","entity_type_id":3}`,
			want:       false,
			why:        "no default set means today's behaviour is unchanged",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			esvc := &stubEntitySvcCapturingCreate{}
			h := NewAPIHandler(nil, esvc, &stubCampaignSvcWithSettings{defaultVis: tc.defaultVis}, nil)

			c := newCreateAPIContext("/api/v1/campaigns/camp-1/entities", tc.body)
			if err := h.CreateEntity(c); err != nil {
				t.Fatalf("CreateEntity: %v", err)
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

// A campaign whose settings cannot be read must fail closed: an unknown
// default resolves to private, since recovering from a leak is not possible
// but recovering from an over-hidden entity is just a toggle.
func TestCreateEntity_UnreadableCampaignFailsClosed(t *testing.T) {
	esvc := &stubEntitySvcCapturingCreate{}
	csvc := &stubCampaignSvcWithSettings{getByIDErr: context.DeadlineExceeded}
	h := NewAPIHandler(nil, esvc, csvc, nil)

	c := newCreateAPIContext("/api/v1/campaigns/camp-1/entities", `{"name":"Unknown","entity_type_id":3}`)
	if err := h.CreateEntity(c); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if len(esvc.created) != 1 {
		t.Fatalf("expected 1 Create call, got %d", len(esvc.created))
	}
	if !esvc.created[0].IsPrivate {
		t.Error("IsPrivate = false with unreadable campaign settings; an unknown default must fail closed, not publish")
	}
}

// An explicit is_private:false is a decision the client made, and it does not
// need the campaign settings to be readable in order to stand.
func TestCreateEntity_ExplicitValueSkipsTheSettingsRead(t *testing.T) {
	esvc := &stubEntitySvcCapturingCreate{}
	csvc := &stubCampaignSvcWithSettings{defaultVis: "dm_only"}
	h := NewAPIHandler(nil, esvc, csvc, nil)

	c := newCreateAPIContext("/api/v1/campaigns/camp-1/entities", `{"name":"Public Notice","entity_type_id":3,"is_private":false}`)
	if err := h.CreateEntity(c); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if csvc.getByIDs != 0 {
		t.Errorf("campaign settings were read %d time(s) for a request that already stated is_private; the read is only needed for the absent case", csvc.getByIDs)
	}
}

// --- POST /api/v1/campaigns/:id/sync (batch create) ------------------------

func TestSync_BatchCreateHonoursCampaignDefaultVisibility(t *testing.T) {
	cases := []struct {
		name       string
		defaultVis string
		change     string
		want       bool
		why        string
	}{
		{
			name:       "absent is_private under dm_only default starts private",
			defaultVis: "dm_only",
			change:     `{"action":"create","entity_type_id":3,"name":"Batch Contact"}`,
			want:       true,
			why:        ".Val(false) threw away the absent/false distinction the field already carried",
		},
		{
			name:       "absent is_private under private default starts private",
			defaultVis: "private",
			change:     `{"action":"create","entity_type_id":3,"name":"Batch Contact"}`,
			want:       true,
			why:        "\"private\" is the other value the campaign settings page writes",
		},
		{
			name:       "explicit false under dm_only default stays public",
			defaultVis: "dm_only",
			change:     `{"action":"create","entity_type_id":3,"name":"Batch Notice","is_private":false}`,
			want:       false,
			why:        "the batch door must obey a deliberate public exactly like the single-entity door",
		},
		{
			name:       "explicit true with no default is private",
			defaultVis: "",
			change:     `{"action":"create","entity_type_id":3,"name":"Batch Secret","is_private":true}`,
			want:       true,
			why:        "an explicit private request is honoured with or without a campaign default",
		},
		{
			name:       "absent with no default stays public",
			defaultVis: "",
			change:     `{"action":"create","entity_type_id":3,"name":"Batch Well"}`,
			want:       false,
			why:        "no default set means today's behaviour is unchanged",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			esvc := &stubEntitySvcCapturingCreate{}
			h := NewAPIHandler(nil, esvc, &stubCampaignSvcWithSettings{defaultVis: tc.defaultVis}, nil)

			body := `{"changes":[` + tc.change + `]}`
			c := newCreateAPIContext("/api/v1/campaigns/camp-1/sync", body)
			if err := h.Sync(c); err != nil {
				t.Fatalf("Sync: %v", err)
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

// A batch of several creates must not re-read the campaign settings per
// change. One read per request is enough and the default cannot change
// mid-batch.
func TestSync_BatchCreateReadsCampaignSettingsOnce(t *testing.T) {
	esvc := &stubEntitySvcCapturingCreate{}
	csvc := &stubCampaignSvcWithSettings{defaultVis: "dm_only"}
	h := NewAPIHandler(nil, esvc, csvc, nil)

	body := `{"changes":[` +
		`{"action":"create","entity_type_id":3,"name":"A"},` +
		`{"action":"create","entity_type_id":3,"name":"B"},` +
		`{"action":"create","entity_type_id":3,"name":"C"}]}`
	c := newCreateAPIContext("/api/v1/campaigns/camp-1/sync", body)
	if err := h.Sync(c); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(esvc.created) != 3 {
		t.Fatalf("expected 3 Create calls, got %d", len(esvc.created))
	}
	for i, in := range esvc.created {
		if !in.IsPrivate {
			t.Errorf("change %d: IsPrivate = false, want true", i)
		}
	}
	if csvc.getByIDs != 1 {
		t.Errorf("campaign settings read %d times for one batch, want exactly 1", csvc.getByIDs)
	}
}

// A batch with no create action must not spend a campaign read at all.
func TestSync_NoCreateChangesSkipsTheSettingsRead(t *testing.T) {
	esvc := &stubEntitySvcCapturingCreate{}
	csvc := &stubCampaignSvcWithSettings{defaultVis: "dm_only"}
	h := NewAPIHandler(nil, esvc, csvc, nil)

	c := newCreateAPIContext("/api/v1/campaigns/camp-1/sync", `{"changes":[]}`)
	if err := h.Sync(c); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if csvc.getByIDs != 0 {
		t.Errorf("campaign settings read %d time(s) for a batch with no creates, want 0", csvc.getByIDs)
	}
}

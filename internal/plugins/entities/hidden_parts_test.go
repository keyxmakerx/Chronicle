package entities

// A Scribe sees what players see: they write only to pages they can see and
// edit, and a save of theirs must never erase the hidden parts their copy of
// the page left out.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const (
	plainHTML  = `<p>The harbor at dawn.</p>`
	secretHTML = `<p>The harbor at dawn.</p><p><span data-secret="true">The mayor smuggles.</span></p>`
)

// hiddenPartsSvc is a page in campaign c1 whose access and stored text the
// test sets, recording what reaches the store.
type hiddenPartsSvc struct {
	EntityService
	entity   *Entity
	access   EffectivePermission
	et       *EntityType
	saved    bool
	replaced map[string]any
	merged   map[string]any
}

func (s *hiddenPartsSvc) GetByID(_ context.Context, _ string) (*Entity, error) {
	e := *s.entity
	return &e, nil
}

func (s *hiddenPartsSvc) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*EffectivePermission, error) {
	if role >= int(campaigns.RoleOwner) {
		return &EffectivePermission{CanView: true, CanEdit: true}, nil
	}
	a := s.access
	return &a, nil
}

func (s *hiddenPartsSvc) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	if s.et == nil {
		return &EntityType{}, nil
	}
	return s.et, nil
}

func (s *hiddenPartsSvc) EntryRev(_ context.Context, _ string) (int, error) { return 1, nil }

func (s *hiddenPartsSvc) SaveEntry(_ context.Context, _, _, _ string, _ *int) (int, *EntryConflict, error) {
	s.saved = true
	return 2, nil, nil
}

func (s *hiddenPartsSvc) UpdateFields(_ context.Context, _ string, f map[string]any) error {
	s.replaced = f
	return nil
}

func (s *hiddenPartsSvc) MergeFields(_ context.Context, _ string, f map[string]any) error {
	s.merged = f
	return nil
}

func scribeCC(dm bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe, IsDmGranted: dm, IsMember: true}
}

func hiddenPartsCtx(cc *campaigns.CampaignContext, method, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/c1/entities/e1/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")
	c.Set("auth_user_id", "u1")
	c.Set("campaign_context", cc)
	return c, rec
}

func TestRequirePageWrite(t *testing.T) {
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		access EffectivePermission
		want   int // 0 = passed through
	}{
		{"owner passes without a check", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true}, EffectivePermission{}, 0},
		{"co-dm passes without a check", scribeCC(true), EffectivePermission{}, 0},
		{"scribe on a page they edit", scribeCC(false), EffectivePermission{CanView: true, CanEdit: true}, 0},
		{"scribe on a page they only read", scribeCC(false), EffectivePermission{CanView: true}, http.StatusForbidden},
		{"scribe on a hidden page: not found", scribeCC(false), EffectivePermission{}, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{service: &hiddenPartsSvc{entity: &Entity{ID: "e1", CampaignID: "c1"}, access: tt.access}}
			reached := false
			c, _ := hiddenPartsCtx(tt.cc, http.MethodPut, "")
			err := h.requirePageWrite(func(echo.Context) error { reached = true; return nil })(c)
			if tt.want == 0 {
				if err != nil || !reached {
					t.Fatalf("err = %v, reached = %v; want passed through", err, reached)
				}
				return
			}
			assertAppError(t, err, tt.want)
			if reached {
				t.Fatal("a refused write reached the handler")
			}
		})
	}
}

func TestEntryWithHiddenParts(t *testing.T) {
	tests := []struct {
		name       string
		cc         *campaigns.CampaignContext
		stored     string
		wantLocked bool
	}{
		{"scribe, text with a secret", scribeCC(false), secretHTML, true},
		{"scribe, plain text", scribeCC(false), plainHTML, false},
		{"co-dm, text with a secret", scribeCC(true), secretHTML, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := tt.stored
			svc := &hiddenPartsSvc{entity: &Entity{ID: "e1", CampaignID: "c1", EntryHTML: &stored}, access: EffectivePermission{CanView: true, CanEdit: true}}
			h := &Handler{service: svc}

			// The editor is told the text is locked, and the read leaves the
			// secret out for a Scribe.
			c, rec := hiddenPartsCtx(tt.cc, http.MethodGet, "")
			if err := h.GetEntry(c); err != nil {
				t.Fatalf("GetEntry: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if locked, _ := got["locked"].(bool); locked != tt.wantLocked {
				t.Errorf("locked = %v, want %v", locked, tt.wantLocked)
			}
			if tt.wantLocked && strings.Contains(rec.Body.String(), "smuggles") {
				t.Error("a Scribe was served the secret")
			}

			// The save over it is refused, and nothing is stored.
			c, _ = hiddenPartsCtx(tt.cc, http.MethodPut, `{"entry_html":"<p>The harbor at dawn.</p>"}`)
			err := h.UpdateEntryAPI(c)
			if tt.wantLocked {
				assertAppError(t, err, http.StatusForbidden)
				if svc.saved {
					t.Fatal("a locked save reached the store")
				}
				return
			}
			if err != nil || !svc.saved {
				t.Fatalf("err = %v, saved = %v; want saved", err, svc.saved)
			}
		})
	}
}

func TestFieldsSaveKeepsHiddenFields(t *testing.T) {
	et := &EntityType{Fields: []FieldDefinition{{Key: "might"}, {Key: "gm_notes", GMOnly: true}, {Key: "backstory", OwnerOnly: true}}}
	stored := map[string]any{"might": 1, "gm_notes": "the mayor smuggles", "backstory": "an orphan"}

	t.Run("a whole-form save keeps the stored hidden values", func(t *testing.T) {
		svc := &hiddenPartsSvc{entity: &Entity{ID: "e1", CampaignID: "c1", FieldsData: stored}, access: EffectivePermission{CanView: true, CanEdit: true}, et: et}
		c, _ := hiddenPartsCtx(scribeCC(false), http.MethodPut, `{"fields_data":{"might":3,"gm_notes":"","backstory":""}}`)
		if err := (&Handler{service: svc}).UpdateFieldsAPI(c); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"might": float64(3), "gm_notes": "the mayor smuggles", "backstory": "an orphan"}
		if !reflect.DeepEqual(svc.replaced, want) {
			t.Errorf("saved %v, want %v", svc.replaced, want)
		}
	})
	t.Run("a partial save drops the hidden keys", func(t *testing.T) {
		svc := &hiddenPartsSvc{entity: &Entity{ID: "e1", CampaignID: "c1", FieldsData: stored}, access: EffectivePermission{CanView: true, CanEdit: true}, et: et}
		c, _ := hiddenPartsCtx(scribeCC(false), http.MethodPut, `{"fields_patch":{"might":3,"gm_notes":null}}`)
		if err := (&Handler{service: svc}).UpdateFieldsAPI(c); err != nil {
			t.Fatal(err)
		}
		if want := map[string]any{"might": float64(3)}; !reflect.DeepEqual(svc.merged, want) {
			t.Errorf("merged %v, want %v", svc.merged, want)
		}
	})
	t.Run("a co-dm's save is theirs whole", func(t *testing.T) {
		svc := &hiddenPartsSvc{entity: &Entity{ID: "e1", CampaignID: "c1", FieldsData: stored}, et: et}
		c, _ := hiddenPartsCtx(scribeCC(true), http.MethodPut, `{"fields_patch":{"gm_notes":null}}`)
		if err := (&Handler{service: svc}).UpdateFieldsAPI(c); err != nil {
			t.Fatal(err)
		}
		if want := map[string]any{"gm_notes": nil}; !reflect.DeepEqual(svc.merged, want) {
			t.Errorf("merged %v, want %v", svc.merged, want)
		}
	})
}

func TestBulkVisibility_ScribeRefused(t *testing.T) {
	c, _ := hiddenPartsCtx(scribeCC(false), http.MethodPost, `{"entity_ids":["e1"],"private":false}`)
	h := &Handler{service: &hiddenPartsSvc{entity: &Entity{ID: "e1", CampaignID: "c1", IsPrivate: true}}}
	assertAppError(t, h.BulkVisibilityAPI(c), http.StatusForbidden)
}

func TestRequireCreatableType(t *testing.T) {
	adv := &EntityType{ID: 9, PresetCategory: advPreset()}
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		et   *EntityType
		want int
	}{
		{"scribe, ordinary category", scribeCC(false), &EntityType{ID: 1}, 0},
		{"scribe, DM-only category", scribeCC(false), adv, http.StatusForbidden},
		{"co-dm, DM-only category", scribeCC(true), adv, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := hiddenPartsCtx(tt.cc, http.MethodPost, "")
			err := (&Handler{service: &hiddenPartsSvc{et: tt.et}}).requireCreatableType(c, tt.cc, tt.et.ID)
			if tt.want == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			assertAppError(t, err, tt.want)
		})
	}
}

func TestServedCopy(t *testing.T) {
	stored := secretHTML
	et := &EntityType{Fields: []FieldDefinition{{Key: "gm_notes", GMOnly: true}}}
	e := &Entity{ID: "e1", EntryHTML: &stored, FieldsData: map[string]any{"gm_notes": "x", "might": 2}}

	got := servedCopy(scribeCC(false), e, et, "u1")
	if strings.Contains(*got.EntryHTML, "smuggles") {
		t.Error("the served copy kept the secret")
	}
	if _, ok := got.FieldsData["gm_notes"]; ok {
		t.Error("the served copy kept the GM-only field")
	}
	if *e.EntryHTML != secretHTML || e.FieldsData["gm_notes"] != "x" {
		t.Error("the stored entity was changed")
	}
	if servedCopy(scribeCC(true), e, et, "u1") != e {
		t.Error("a co-dm is served the page as stored")
	}
}

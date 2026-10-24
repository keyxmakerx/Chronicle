package syncapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestCheckFieldPatch(t *testing.T) {
	defs := []entities.FieldDefinition{{Key: "ev", Type: "number"}, {Key: "role", Type: "text"}}
	sys := map[string]bool{"stamina_max": true}
	stored := map[string]any{"ev": float64(12), "legacy_junk": "kept", "old_ev": "lots"}
	cases := []struct {
		name  string
		patch map[string]any
		want  []string // substrings, one per problem; none means it may be saved
	}{
		{"declared keys", map[string]any{"ev": float64(16), "role": "Brute"}, nil},
		{"number as text", map[string]any{"ev": "16"}, nil},
		{"emptied number box", map[string]any{"ev": ""}, nil},
		{"null clears anything", map[string]any{"bogus": nil, "ev": nil}, nil},
		{"system sheet key", map[string]any{"stamina_max": float64(40)}, nil},
		{"unchanged stored values resent", map[string]any{"legacy_junk": "kept", "ev": float64(12)}, nil},
		{"unknown keys", map[string]any{"bogus_key": 1, "made_up": "x"}, []string{`"bogus_key" is not one`, `"made_up" is not one`}},
		{"not a number", map[string]any{"ev": "lots"}, []string{`"ev" needs a number`}},
		{"changed junk key", map[string]any{"legacy_junk": "changed"}, []string{`"legacy_junk" is not one`}},
	}
	for _, c := range cases {
		got := checkFieldPatch(defs, sys, stored, c.patch)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %q, want %d problems", c.name, got, len(c.want))
			continue
		}
		for i, w := range c.want {
			if !strings.Contains(got[i], w) {
				t.Errorf("%s: problem %d = %q, want %q", c.name, i, got[i], w)
			}
		}
	}
}

// The update doors answer 400 naming the bad keys and save nothing.
func TestUpdateEntity_RefusesUndeclaredFields(t *testing.T) {
	svc := &stubEntityServiceForPartialUpdate{}
	h := NewAPIHandler(nil, svc, stubCampaignSvcOwner{}, nil)
	c, _ := newPartialUpdateContext(http.MethodPut, "/api/v1/campaigns/camp-1/entities/ent-1", `{"fields_data":{"bogus_key":1,"hp":"not a number"}}`)
	err := h.UpdateEntity(c)
	ae, ok := err.(*apperror.AppError)
	if !ok || ae.Code != http.StatusBadRequest {
		t.Fatalf("want a 400, got %v", err)
	}
	for _, want := range []string{"bogus_key", `"hp" needs a number`} {
		if !strings.Contains(ae.Message, want) {
			t.Errorf("message %q does not name %s", ae.Message, want)
		}
	}
	if len(svc.updates) != 0 {
		t.Errorf("saved despite the refusal: %+v", svc.updates)
	}

	c, rec := newPartialUpdateContext(http.MethodPost, "/api/v1/campaigns/camp-1/sync",
		`{"changes":[{"action":"update","entity_id":"ent-1","fields_data":{"bogus_key":1}}]}`)
	if err := h.Sync(c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "bogus_key") || len(svc.updates) != 0 {
		t.Errorf("batch row not refused: %s", rec.Body.String())
	}
}

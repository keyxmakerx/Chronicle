package campaigns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// layoutRecorder captures what the importer saves for the campaign home.
type layoutRecorder struct {
	CampaignService
	raw    *string
	single *DashboardLayout
}

func (r *layoutRecorder) UpdateDashboardLayoutRaw(_ context.Context, _ string, j *string) error {
	r.raw = j
	return nil
}

func (r *layoutRecorder) UpdateDashboardLayout(_ context.Context, _ string, l *DashboardLayout) error {
	r.single = l
	return nil
}

// A role-keyed home must survive import; read as a single layout it
// decodes to no rows and every home goes blank.
func TestImportDashboardLayout(t *testing.T) {
	roleKeyed := `{"director":{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"welcome_banner"}]}]}]}}`
	single := `{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"welcome_banner"}]}]}]}`
	bad := `{"player":{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"no_such_block"}]}]}]}}`

	t.Run("role keyed", func(t *testing.T) {
		rec := &layoutRecorder{}
		if err := (&ExportImportService{campaigns: rec}).importDashboardLayout(context.Background(), "c", json.RawMessage(roleKeyed)); err != nil {
			t.Fatal(err)
		}
		if rec.raw == nil || rec.single != nil {
			t.Fatalf("want raw save only, got raw=%v single=%v", rec.raw, rec.single)
		}
		var got RoleDashboardLayouts
		if err := json.Unmarshal([]byte(*rec.raw), &got); err != nil || got.Director == nil || len(got.Director.Rows) != 1 {
			t.Fatalf("director home lost: %s (%v)", *rec.raw, err)
		}
	})
	t.Run("links point at the new campaign", func(t *testing.T) {
		rec := &layoutRecorder{}
		linked := `{"director":{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"text_block","config":{"content":"<a href=\"/campaigns/2cb96801-23fd-431d-9388-fb4d174d728f/members\">Invite</a>"}}]}]}]}}`
		if err := (&ExportImportService{campaigns: rec}).importDashboardLayout(context.Background(), "new-campaign", json.RawMessage(linked)); err != nil {
			t.Fatal(err)
		}
		if rec.raw == nil || !strings.Contains(*rec.raw, "/campaigns/new-campaign/members") || strings.Contains(*rec.raw, "2cb96801") {
			t.Fatalf("link not rewritten: %v", rec.raw)
		}
	})
	t.Run("single layout", func(t *testing.T) {
		rec := &layoutRecorder{}
		if err := (&ExportImportService{campaigns: rec}).importDashboardLayout(context.Background(), "c", json.RawMessage(single)); err != nil {
			t.Fatal(err)
		}
		if rec.single == nil || len(rec.single.Rows) != 1 {
			t.Fatalf("single layout lost: %+v", rec.single)
		}
	})
	t.Run("invalid role layout is refused", func(t *testing.T) {
		rec := &layoutRecorder{}
		if err := (&ExportImportService{campaigns: rec}).importDashboardLayout(context.Background(), "c", json.RawMessage(bad)); err == nil {
			t.Fatal("unsupported block type was accepted")
		}
		if rec.raw != nil {
			t.Fatal("invalid layout was saved")
		}
	})
}

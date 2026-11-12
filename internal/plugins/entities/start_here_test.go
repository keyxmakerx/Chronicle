package entities

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestStartHereFirstLine(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"first paragraph only", "<p>What the party knows.</p><p>More later.</p>", "What the party knows."},
		{"skips a heading", "<h2>Overview</h2><p>The toll, the Watch &amp; the lamp.</p>", "The toll, the Watch & the lamp."},
		{"skips empty blocks", "<p> </p><p><br></p><ul><li>First <b>item</b></li></ul>", "First item"},
		{"line break ends the line", "<p>One<br>Two</p>", "One"},
		{"nothing", "", ""},
		{"long cut at a word", "<p>" + repeatWords("tide", 60) + "</p>", repeatWords("tide", 36) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startHereFirstLine(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func repeatWords(w string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += " "
		}
		out += w
	}
	return out
}

// The block shows the picked page only to a viewer who can open it, with its
// secrets taken out for players; the owner gets the slot when there is
// nothing to show, and everyone else an empty answer.
func TestStartHereHandler(t *testing.T) {
	secretHTML := `<p>The lamp went dark. <span data-secret="true">The keeper did it.</span></p>`
	pages := map[string]*Entity{
		"open":   {ID: "open", CampaignID: "c1", EntityTypeID: 3, Name: "Arriving in Highwater", Visibility: VisibilityDefault, EntryHTML: &secretHTML},
		"hidden": {ID: "hidden", CampaignID: "c1", EntityTypeID: 3, Name: "The Drowned Saint", Visibility: VisibilityDefault, IsPrivate: true},
		"other":  {ID: "other", CampaignID: "c2", EntityTypeID: 3, Name: "Elsewhere", Visibility: VisibilityDefault},
	}
	tests := []struct {
		name     string
		role     campaigns.Role
		page     string
		want     []string
		wantNot  []string
		wantNone bool
	}{
		{"player, open page", campaigns.RolePlayer, "open", []string{"Arriving in Highwater", "The lamp went dark."}, []string{"keeper", "data-dm-only"}, false},
		{"player, hidden page", campaigns.RolePlayer, "hidden", nil, nil, true},
		{"player, nothing picked", campaigns.RolePlayer, "", nil, nil, true},
		{"owner, hidden page is marked", campaigns.RoleOwner, "hidden", []string{"The Drowned Saint", "data-dm-only"}, nil, false},
		{"owner, nothing picked", campaigns.RoleOwner, "", []string{"Pick the page a new reader should open first", "/campaigns/c1/customize/home"}, nil, false},
		{"owner, page deleted", campaigns.RoleOwner, "gone", []string{"was deleted"}, nil, false},
		{"owner, another campaign's page", campaigns.RoleOwner, "other", []string{"was deleted"}, []string{"Elsewhere"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et := &EntityType{ID: 3, CampaignID: "c1", Slug: "place", Name: "Place", Enabled: true}
			typeRepo := &mockEntityTypeRepo{findByIDFn: func(context.Context, int) (*EntityType, error) { cp := *et; return &cp, nil }}
			entityRepo := &mockEntityRepo{findByIDFn: func(_ context.Context, eid string) (*Entity, error) {
				e, ok := pages[eid]
				if !ok {
					return nil, apperror.NewNotFound("entity not found")
				}
				cp := *e
				return &cp, nil
			}}
			h := &Handler{service: NewEntityService(entityRepo, typeRepo, &mockPermissionRepo{})}
			c, rec := advCtx(tt.role, false, http.MethodGet, "/campaigns/c1/start-here?page="+tt.page)
			if err := h.StartHere(c); err != nil {
				t.Fatalf("StartHere err = %v", err)
			}
			body := rec.Body.String()
			if tt.wantNone != (body == "") {
				t.Fatalf("empty = %v, want %v: %q", body == "", tt.wantNone, body)
			}
			for _, w := range tt.want {
				if !strings.Contains(body, w) {
					t.Errorf("body missing %q:\n%s", w, body)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(body, w) {
					t.Errorf("body should not hold %q:\n%s", w, body)
				}
			}
		})
	}
}

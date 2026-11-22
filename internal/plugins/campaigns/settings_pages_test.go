// settings_pages_test.go pins Campaign settings' banner and its Campaign,
// Connections and Data tabs: what each is built from, what each draws, and
// that old addresses and the Danger zone's steps still reach their routes.

package campaigns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func renderPart(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	ctx := layouts.SetCampaignRole(layouts.SetCampaignID(layouts.SetIsAuthenticated(context.Background(), true), "c1"), 3)
	if err := c.Render(ctx, &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// The banner names the campaign, its pictures, whether it is private or
// archived, its system and head count; what could not be read is left out.
func TestMastOf(t *testing.T) {
	cover := " /media/cover.png "
	now := time.Now()
	two := []CampaignMember{{Role: RoleOwner}, {Role: RolePlayer}}
	tests := []struct {
		name      string
		c         *Campaign
		in        DeskInputs
		wantFacts string
		wantPub   bool
		wantArch  bool
	}{
		{"bare", &Campaign{Name: "Therin", Settings: "{}"}, DeskInputs{}, "", false, false},
		{"system and people", &Campaign{Name: "Therin", IsPublic: true, Settings: "{}"}, DeskInputs{SystemName: "Draw Steel", Members: two}, "Draw Steel,2 members", true, false},
		{"a system of their own is left out", &Campaign{Name: "Therin", Settings: "{}"}, DeskInputs{SystemID: "custom:x", Members: two[:1]}, "1 member", false, false},
		{"archived", &Campaign{Name: "Therin", Settings: "{}", ArchivedAt: &now}, DeskInputs{}, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mastOf(tt.c, tt.in)
			if m.Name != "Therin" || strings.Join(m.Facts, ",") != tt.wantFacts || m.Public != tt.wantPub || m.Archived != tt.wantArch {
				t.Errorf("mast = %+v", m)
			}
		})
	}
	m := mastOf(&Campaign{Name: "Therin", BackdropPath: &cover, Settings: `{"brand_logo":" /media/logo.png "}`}, DeskInputs{})
	if m.Cover != "/media/cover.png" || m.Emblem != "/media/logo.png" {
		t.Errorf("pictures are trimmed paths: %q / %q", m.Cover, m.Emblem)
	}
}

// The Campaign tab reuses the board's blanks, lists the Customize areas after
// the Look strip, and its card shows at most five faces.
func TestCampaignTabOf(t *testing.T) {
	desc := "  The last free city  "
	c := &Campaign{ID: "c1", Name: "Therin", Description: &desc, IsPublic: true, Settings: "{}"}
	var team []CampaignMember
	for i := 0; i < 8; i++ {
		team = append(team, CampaignMember{UserID: "u" + itoa(i), DisplayName: "P" + itoa(i)})
	}
	tab := campaignTabOf(c, DeskInputs{Members: team, Tabs: []DeskTab{{ID: "dice", Label: "Dice"}}})
	if tab.Name.URL != "/campaigns/c1/details" || tab.Name.Field != "name" || tab.Description.Field != "description" || tab.Description.Max != 5000 {
		t.Errorf("fields save through the details route: %+v / %+v", tab.Name, tab.Description)
	}
	if tab.Access.URL != "/campaigns/c1/details" || !tab.Access.Bool || tab.Pages.URL != "/campaigns/c1/default-visibility" {
		t.Errorf("choices save through their routes: %+v / %+v", tab.Access, tab.Pages)
	}
	var ids []string
	for _, l := range tab.Customize {
		ids = append(ids, l.ID)
	}
	if strings.Join(ids, ",") != "categories,homes,x-dice,brush" || tab.CustomizerHref != "/campaigns/c1/customize" || tab.PicturesHref != "/campaigns/c1/customize/look" {
		t.Errorf("customize rows = %v, links = %q / %q", ids, tab.CustomizerHref, tab.PicturesHref)
	}
	if tab.Card.Name != "Therin" || tab.Card.Description != "The last free city" || !tab.Card.Public || len(tab.Card.Faces) != campaignCardFaces {
		t.Errorf("card = %+v", tab.Card)
	}
	if !choiceOn(tab.Access, DeskOption{Value: "true"}) || choiceOn(tab.Access, DeskOption{Value: "false"}) || choiceOn(nil, DeskOption{}) {
		t.Error("choiceOn reports the stored option only")
	}
}

// The Campaign tab draws each setting once, as a field or a choice that saves
// through its route, beside the card people see; the game system lives on
// Game, not here. An archived campaign's fields are off.
func TestCampaignTabPage(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	cc := &CampaignContext{Campaign: c, MemberRole: RoleOwner}
	html := renderPart(t, CampaignTabPage(cc, campaignTabOf(c, DeskInputs{})))
	for _, want := range []string{
		`id="sb-f-name"`, `maxlength="200"`, `data-url="/campaigns/c1/details"`, `data-field="name"`,
		`<textarea id="sb-f-description"`, `maxlength="5000"`, `data-field="description"`,
		`id="line-public"`, `id="line-pages"`, `data-sb-choice`, `aria-pressed="true"`, `aria-pressed="false"`,
		`data-url="/campaigns/c1/default-visibility"`, `href="/campaigns/c1/customize/look"`, `href="/campaigns/c1/customize"`, `Open the Customizer`,
		`data-sb-bind="name"`, `data-sb-bind="description"`, `id="line-preview"`, `role="status" data-sb-status`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Campaign tab is missing %q", want)
		}
	}
	for _, gone := range []string{"data-game-system-card", "Game system", "Danger zone", "settings?tab="} {
		if strings.Contains(html, gone) {
			t.Errorf("Campaign tab holds %q, which lives elsewhere", gone)
		}
	}
	now := time.Now()
	archived := &Campaign{ID: "c1", Name: "Therin", Settings: "{}", ArchivedAt: &now}
	off := renderPart(t, CampaignTabPage(&CampaignContext{Campaign: archived, MemberRole: RoleOwner}, campaignTabOf(archived, DeskInputs{})))
	if strings.Count(off, " disabled") < 2 || !strings.Contains(off, deskArchivedOff) {
		t.Error("an archived campaign's fields are off and say why")
	}
}

// Connections names Foundry only while Foundry sync is on, and otherwise says
// where it is switched on.
func TestConnectionsTabOf(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	now := time.Now()
	off := connectionsTabOf(c, map[string]bool{}, DeskFacts{Keys: &DeskKeysFacts{Count: 2}}, "https://x.test", now)
	if off.Foundry != nil || off.FoundryOffHref != "/campaigns/c1/extensions" || off.Keys.Count != 2 || off.BaseURL != "https://x.test" || off.AI.Href != "/campaigns/c1/ai" {
		t.Errorf("foundry off = %+v", off)
	}
	on := connectionsTabOf(c, map[string]bool{"sync-api": true}, DeskFacts{Foundry: &DeskFoundryFacts{}}, "", now)
	if on.Foundry == nil || on.Foundry.Value.Text != "Not set up" || on.FoundryOffHref != "" {
		t.Errorf("foundry on = %+v", on)
	}
	html := renderPart(t, ConnectionsTabPage(&CampaignContext{Campaign: c, MemberRole: RoleOwner}, off))
	for _, want := range []string{`id="line-foundry"`, `>Off<`, `href="/campaigns/c1/extensions"`, `id="line-ai"`, `id="keys"`, `2 keys`, `hx-get="/campaigns/c1/integrations/keys"`, `https://x.test`} {
		if !strings.Contains(html, want) {
			t.Errorf("Connections is missing %q", want)
		}
	}
}

// Data's figures lead to their own pages, the storage figure only while Media
// is on; it carries other plugins' parts but never the parts it draws itself.
func TestDataTabOf(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	plain := dataTabOf(c, map[string]bool{}, DeskFacts{}, nil)
	if plain.Storage != nil || plain.Trash.Label != "Trash" || plain.Trash.Href != "/campaigns/c1/trash" || plain.Trash.Value.Href != "/campaigns/c1/trash" || plain.Transfer != "/campaigns/c1/members#transfer" {
		t.Errorf("data without media = %+v", plain)
	}
	media := dataTabOf(c, map[string]bool{"media-gallery": true}, DeskFacts{Media: &DeskMediaFacts{Files: 3, Bytes: 2048}}, nil)
	if media.Storage == nil || media.Storage.Label != "Pictures and files" || media.Storage.Href != "/campaigns/c1/media" || media.Storage.Value.Text != "2 KB" || media.Storage.Hint != "3 files" {
		t.Errorf("data with media = %+v", media.Storage)
	}
	if mediaHint(nil) != "" || mediaHint(&DeskMediaFacts{}) != "Nothing uploaded yet" {
		t.Error("media hint words")
	}
	got := tabIDs(dataSections([]SettingsTab{{ID: "general"}, {ID: "api-keys"}, {ID: "data"}, {ID: "import"}, {ID: "activity"}, {ID: "vault"}}))
	if strings.Join(got, ",") != "import,vault" {
		t.Errorf("data sections = %v", got)
	}
}

// Data draws the copies, each plugin's part under its own anchor, the activity
// log and the Danger zone, where nothing acts in one press: archiving opens a
// step, transfer leads to People, and deleting asks for the name, sent without
// the CSRF token in the address.
func TestDataTabPage(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	cc := &CampaignContext{Campaign: c, MemberRole: RoleOwner}
	sections := []SettingsTab{{ID: "import", Label: "Import", Content: templ.Raw(`<p>vault</p>`)}}
	html := renderPart(t, DataTabPage(cc, dataTabOf(c, map[string]bool{}, DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, sections), "tok"))
	for _, want := range []string{
		`id="export"`, `href="/campaigns/c1/export?include_media=1"`, `href="/campaigns/c1/export"`,
		`<h2 class="sb-h" id="import">Import</h2>`, `<p>vault</p>`, `id="activity"`, `hx-get="/campaigns/c1/activity/fragment"`,
		`id="danger"`, `<summary class="sb-word">Archive…</summary>`, `hx-post="/campaigns/c1/archive"`, `name="csrf_token" value="tok"`,
		`href="/campaigns/c1/members#transfer"`, `hx-delete="/campaigns/c1"`, `name="confirm_name"`, `data-name="Therin"`,
		`id="line-trash"`, `2 items`, `href="/campaigns/c1/trash"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Data is missing %q", want)
		}
	}
	del := html[strings.Index(html, `id="line-delete"`):]
	if strings.Contains(del, "csrf_token") || strings.Contains(del, `name="_method"`) {
		t.Error("the delete form must not put the CSRF token in the address")
	}
	now := time.Now()
	archived := &Campaign{ID: "c1", Name: "Therin", Settings: "{}", ArchivedAt: &now}
	if out := renderPart(t, DataTabPage(&CampaignContext{Campaign: archived, MemberRole: RoleOwner}, dataTabOf(archived, map[string]bool{}, DeskFacts{}, nil), "tok")); !strings.Contains(out, `hx-post="/campaigns/c1/unarchive"`) || strings.Contains(out, `/archive"`) {
		t.Error("an archived campaign offers Unarchive only")
	}
}

// settingsOwnerRequest runs a Campaign settings handler as the given role.
func settingsOwnerRequest(h *Handler, method, target string, role Role, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
	c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}, MemberRole: role, IsMember: true})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	return rec, fn(h, c)
}

// An old Details and data tab address lands on the tab that holds it now, and
// only the owner opens any of the three tabs.
func TestSettingsTabsHandlers(t *testing.T) {
	h := NewHandler(deskStubService{})
	rec, err := settingsOwnerRequest(h, http.MethodGet, "/campaigns/c1/settings?tab=api-keys", RoleOwner, (*Handler).Settings)
	if err != nil || rec.Code != http.StatusFound || rec.Header().Get("Location") != "/campaigns/c1/connections#keys" {
		t.Errorf("old keys tab: err=%v code=%d location=%q", err, rec.Code, rec.Header().Get("Location"))
	}
	for name, fn := range map[string]func(*Handler, *echo.Context) error{
		"campaign": (*Handler).Settings, "connections": (*Handler).ConnectionsPage, "data": (*Handler).DataPage,
	} {
		t.Run(name, func(t *testing.T) {
			rec, err := settingsOwnerRequest(h, http.MethodGet, "/campaigns/c1/"+name, RoleOwner, fn)
			if err != nil || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Therin") {
				t.Fatalf("owner: err=%v code=%d", err, rec.Code)
			}
			for _, role := range []Role{RolePlayer, RoleScribe} {
				_, err := settingsOwnerRequest(h, http.MethodGet, "/campaigns/c1/"+name, role, fn)
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
					t.Errorf("role %d: want forbidden, got %v", role, err)
				}
			}
		})
	}
}

// trashService records MoveToTrash.
type trashService struct {
	CampaignService
	moved bool
}

func (s *trashService) MoveToTrash(context.Context, string, string) error {
	s.moved = true
	return nil
}

// htmx sends a DELETE's values in the address: the typed name is read from the
// query, and a wrong name deletes nothing.
func TestDelete_ReadsTheTypedNameFromTheAddress(t *testing.T) {
	for _, tt := range []struct {
		query     string
		wantMoved bool
	}{
		{"?confirm_name=Therin", true},
		{"?confirm_name=therin", false},
		{"", false},
	} {
		svc := &trashService{}
		_, err := settingsOwnerRequest(NewHandler(svc), http.MethodDelete, "/campaigns/c1"+tt.query, RoleOwner, (*Handler).Delete)
		if svc.moved != tt.wantMoved || (err == nil) != tt.wantMoved {
			t.Errorf("%q: moved=%v err=%v", tt.query, svc.moved, err)
		}
	}
}

// desk_test.go pins the desk's wording and its provider seam: every sentence
// is built without a database or a socket, a failing, panicking or stuck
// provider only plain-sentences its own line, and a lower role is refused.

package campaigns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

func deskText(l DeskLine) string {
	var sb strings.Builder
	for _, p := range l.Parts {
		switch {
		case p.GM:
			sb.WriteString("DM")
		default:
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func TestDeskSentences(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		line  DeskLine
		want  string
		state string
	}{
		{"people unread", peopleLine(c, nil, 0), "Everyone in the campaign, and what each may do, is on the People page.", ""},
		{"people alone", peopleLine(c, []CampaignMember{{Role: RoleOwner}}, 0), "Just you, so far.", ""},
		{"people players", peopleLine(c, []CampaignMember{{Role: RoleOwner}, {Role: RolePlayer}, {Role: RolePlayer}}, 0), "3 people: you and 2 players.", ""},
		{"people one player one scribe", peopleLine(c, []CampaignMember{{Role: RoleOwner}, {Role: RolePlayer}, {Role: RoleScribe}}, 0), "3 people: you, 1 scribe and a player.", ""},
		{"changes no provider", changesLine(c, nil), "What players changed on your pages can be looked at, and undone, on its own page.", ""},
		{"changes none", changesLine(c, &DeskChangesFacts{}), "No player has changed anything this week.", ""},
		{"changes one", changesLine(c, &DeskChangesFacts{Changes: 1, Players: 1}), "A player changed 1 thing this week.", ""},
		{"changes many", changesLine(c, &DeskChangesFacts{Changes: 12, Players: 3}), "3 players changed 12 things this week.", ""},
		{"media no provider", mediaLine(c, nil), "Its pictures and files are kept in Media.", ""},
		{"media empty", mediaLine(c, &DeskMediaFacts{}), "Media holds no pictures or files yet.", ""},
		{"media some", mediaLine(c, &DeskMediaFacts{Files: 2, Bytes: 5 << 20}), "Holds 2 pictures and files, 5 MB.", ""},
		{"trash no provider", trashLine(c, nil), "Deleted pages wait in the Trash, where you can bring them back.", ""},
		{"trash empty", trashLine(c, &DeskTrashFacts{Days: 30}), "Trash is empty.", ""},
		{"trash some", trashLine(c, &DeskTrashFacts{Items: 1, Days: 30}), "Trash holds 1 item for 30 days.", ""},
		{"ai no provider", aiLine(c, nil), "Connecting an AI app, and what it did, are on the Your AI page.", ""},
		{"ai none", aiLine(c, &DeskAIFacts{State: DeskAINone}), "No AI is connected yet.", ""},
		{"ai connected", aiLine(c, &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true}), "Your AI (Claude) is connected and may look and ask for changes.", "ok"},
		{"ai look only", aiLine(c, &DeskAIFacts{State: DeskAIConnected}), "Your AI is connected and may only look.", "ok"},
		{"ai paused", aiLine(c, &DeskAIFacts{State: DeskAIPaused}), "Your AI is paused, so it can do nothing until you resume it.", ""},
		{"game neither", gameLine(c, DeskInputs{}), "Its game system and features are chosen on their own page.", ""},
		{"game both", gameLine(c, DeskInputs{SystemName: "Draw Steel", FeaturesOn: 3, FeaturesTotal: 5}), "Plays Draw Steel, with 3 of 5 features on.", ""},
		{"foundry no provider", foundryLine(c, nil, now), "Foundry's link to this campaign, its module and its players are on the Foundry page.", ""},
		{"foundry never set up", foundryLine(c, &DeskFoundryFacts{}, now), "Foundry is not set up yet.", ""},
		{"foundry live", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, HubLastSeen: &now}}, now), "Foundry is connected and answering now.", "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deskText(tt.line); got != tt.want {
				t.Errorf("sentence = %q, want %q", got, tt.want)
			}
			if tt.line.State != tt.state {
				t.Errorf("state = %q, want %q", tt.line.State, tt.state)
			}
		})
	}
}

// The row words: every line's name and short state for each data state, so
// the page never shows more than the facts it was given.
func TestDeskRows_NameAndState(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	two := now.Add(-2 * time.Hour)
	members := func(n int) []CampaignMember {
		out := []CampaignMember{{Role: RoleOwner}}
		for i := 1; i < n; i++ {
			out = append(out, CampaignMember{Role: RolePlayer})
		}
		return out
	}
	tests := []struct {
		name  string
		line  DeskLine
		row   string
		short string
		href  string
		state string
	}{
		{"people unread", peopleLine(c, nil, 0), "People", "", "/campaigns/c1/members", ""},
		{"people alone", peopleLine(c, members(1), 0), "People", "Just you", "/campaigns/c1/members", ""},
		{"people five", peopleLine(c, members(5), 0), "People", "5 people", "/campaigns/c1/members", ""},
		{"people with invites", peopleLine(c, members(5), 2), "People", "5 people, 2 invited", "/campaigns/c1/members", ""},
		{"changes no provider", changesLine(c, nil), "What players changed", "", "/campaigns/c1/players/changes", ""},
		{"changes none", changesLine(c, &DeskChangesFacts{}), "What players changed", "Nothing this week", "/campaigns/c1/players/changes", ""},
		{"changes one", changesLine(c, &DeskChangesFacts{Changes: 1, Players: 1}), "What players changed", "1 change by 1 player this week", "/campaigns/c1/players/changes", ""},
		{"changes many", changesLine(c, &DeskChangesFacts{Changes: 12, Players: 3}), "What players changed", "12 changes by 3 players this week", "/campaigns/c1/players/changes", ""},
		{"game neither", gameLine(c, DeskInputs{}), "Game & features", "", "/campaigns/c1/extensions", ""},
		{"game system only", gameLine(c, DeskInputs{SystemName: "Draw Steel"}), "Game & features", "Draw Steel", "/campaigns/c1/extensions", ""},
		{"game features only", gameLine(c, DeskInputs{FeaturesOn: 1, FeaturesTotal: 4}), "Game & features", "No game system, 1 of 4 features on", "/campaigns/c1/extensions", ""},
		{"game both", gameLine(c, DeskInputs{SystemName: "Draw Steel", FeaturesOn: 12, FeaturesTotal: 15}), "Game & features", "Draw Steel, 12 of 15 features on", "/campaigns/c1/extensions", ""},
		{"media no provider", mediaLine(c, nil), "Media", "", "/campaigns/c1/media", ""},
		{"media empty", mediaLine(c, &DeskMediaFacts{}), "Media", "No pictures or files yet", "/campaigns/c1/media", ""},
		{"media some", mediaLine(c, &DeskMediaFacts{Files: 212, Bytes: 310 << 20}), "Media", "212 pictures and files, 310 MB", "/campaigns/c1/media", ""},
		{"foundry no provider", foundryLine(c, nil, now), "Foundry", "", "/campaigns/c1/foundry", ""},
		{"foundry never set up", foundryLine(c, &DeskFoundryFacts{}, now), "Foundry", "Not set up yet", "/campaigns/c1/foundry", ""},
		{"foundry key, no answer", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true}}, now), "Foundry", "Key made, no answer yet", "/campaigns/c1/foundry", ""},
		{"foundry live", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, HubLastSeen: &now}}, now), "Foundry", "Connected now", "/campaigns/c1/foundry", "ok"},
		{"foundry live with module", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, HubLastSeen: &now, ModuleVersion: "1.2.0"}}, now), "Foundry", "Connected now, module 1.2.0", "/campaigns/c1/foundry", "ok"},
		{"foundry seen", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &two}}, now), "Foundry", "Last answered 2 hours ago", "/campaigns/c1/foundry", "ok"},
		{"ai no provider", aiLine(c, nil), "Your AI", "", "/campaigns/c1/ai", ""},
		{"ai none", aiLine(c, &DeskAIFacts{State: DeskAINone}), "Your AI", "Not connected yet", "/campaigns/c1/ai", ""},
		{"ai connected", aiLine(c, &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true}), "Your AI", "Claude, may ask for changes", "/campaigns/c1/ai", "ok"},
		{"ai look only", aiLine(c, &DeskAIFacts{State: DeskAIConnected}), "Your AI", "Connected, may only look", "/campaigns/c1/ai", "ok"},
		{"ai paused", aiLine(c, &DeskAIFacts{State: DeskAIPaused}), "Your AI", "Paused", "/campaigns/c1/ai", ""},
		{"ai ended", aiLine(c, &DeskAIFacts{State: DeskAIEnded}), "Your AI", "Key ran out", "/campaigns/c1/ai", ""},
		{"keys", keysLine(c), "Keys", "For other tools", "/campaigns/c1/settings?tab=api-keys", ""},
		{"export", exportLine(c), "Export a copy", "", "/campaigns/c1/settings?tab=data", ""},
		{"import", importLine(c), "Import", "", "/campaigns/c1/settings?tab=import", ""},
		{"activity", activityLine(c), "Activity log", "", "/campaigns/c1/settings?tab=activity", ""},
		{"trash no provider", trashLine(c, nil), "Trash", "", "/campaigns/c1/trash", ""},
		{"trash empty", trashLine(c, &DeskTrashFacts{Days: 30}), "Trash", "Empty", "/campaigns/c1/trash", ""},
		{"trash one", trashLine(c, &DeskTrashFacts{Items: 1, Days: 30}), "Trash", "1 thing, kept 30 days", "/campaigns/c1/trash", ""},
		{"trash some", trashLine(c, &DeskTrashFacts{Items: 3, Days: 30}), "Trash", "3 things, kept 30 days", "/campaigns/c1/trash", ""},
		{"archive", endLine(c), "Archive this campaign", "", "/campaigns/c1/settings#danger", ""},
		{"delete", deleteLine(c), "Delete this campaign", "", "/campaigns/c1/settings#danger", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := tt.line
			if l.Name != tt.row || l.Short != tt.short || l.Href != tt.href || l.State != tt.state {
				t.Errorf("row = %q / %q / %q / %q, want %q / %q / %q / %q", l.Name, l.Short, l.Href, l.State, tt.row, tt.short, tt.href, tt.state)
			}
			for _, bad := range []string{"GM Only", "dm_only", "The table"} {
				if strings.Contains(l.Name+l.Short, bad) {
					t.Errorf("row says %q", bad)
				}
			}
		})
	}
}

func TestDeskRows_ArchivedAndLook(t *testing.T) {
	now := time.Now()
	archived := &Campaign{ID: "c1", Settings: "{}", ArchivedAt: &now}
	if l := endLine(archived); l.Name != "Unarchive this campaign" || l.Href != "/campaigns/c1/settings#danger" {
		t.Errorf("archived end row = %+v", l)
	}
	if l := deleteLine(archived); !l.Danger {
		t.Error("the delete row must be in the danger tone")
	}
	tests := []struct {
		name     string
		settings string
		want     string
	}{
		{"standard", `{}`, "Standard"},
		{"material default kit", `{"appearance":{"page_look":"annals"}}`, "Material, Material kit"},
		{"material named kit", `{"appearance":{"page_look":"annals","style_kit":"paper"}}`, "Material, Paper kit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lookLine(&Campaign{ID: "c1", Settings: tt.settings})
			if l.Name != "Customize" || l.Short != tt.want || !l.Swatch || l.Href != "/campaigns/c1/customize" {
				t.Errorf("look row = %+v, want short %q", l, tt.want)
			}
		})
	}
}

func TestDeskRows_DashboardsPostTheViewAsForm(t *testing.T) {
	tests := []struct {
		name      string
		line      DeskLine
		wantAs    string
		wantShort string
		wantOff   bool
	}{
		{"players", playersHomeLine(), "player", "See it", false},
		{"guests, public campaign", guestsHomeLine(&Campaign{ID: "c", IsPublic: true}), "guest", "See it", false},
		{"guests, private campaign", guestsHomeLine(&Campaign{ID: "c"}), "guest", "Private campaign: guests can't open it", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := tt.line
			if l.View == nil || l.View.As != tt.wantAs || l.View.Next != "home" || l.Short != tt.wantShort || (l.View.Off != "") != tt.wantOff {
				t.Fatalf("line = %+v", l)
			}
			var sb strings.Builder
			if err := deskRow(l).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			html := sb.String()
			if tt.wantOff {
				if strings.Contains(html, "<form") || strings.Contains(html, "<button") {
					t.Errorf("a row that cannot open must offer nothing to press: %s", html)
				}
				return
			}
			for _, want := range []string{`method="POST"`, `/view-as"`, `name="as" value="` + tt.wantAs + `"`, `name="next" value="home"`, `name="csrf_token"`, `type="submit" class="set-row"`, `id="line-` + l.ID + `"`} {
				if !strings.Contains(html, want) {
					t.Errorf("form is missing %q: %s", want, html)
				}
			}
		})
	}
}

func TestDeskRows_ChangeInPlace(t *testing.T) {
	tests := []struct {
		name      string
		c         *Campaign
		wantVis   string
		wantPub   string
		wantShown string
		off       bool
	}{
		{"shown and private", &Campaign{ID: "c", Name: "Therin", Settings: "{}"}, "Shown to players", "Private: members only", "Therin", false},
		{"hidden and public", &Campaign{ID: "c", Name: "Therin", IsPublic: true, Settings: `{"default_visibility":"dm_only"}`}, "Hidden from players", "Public: anyone with the link", "Therin", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vis, pub, name := pagesLine(tt.c), publicLine(tt.c), nameLine(tt.c)
			if vis.Name != "New pages start" || pub.Name != "Who can read it" || name.Name != "Name" {
				t.Errorf("names = %q / %q / %q", vis.Name, pub.Name, name.Name)
			}
			if vis.Blank == nil || pub.Blank == nil || name.Blank == nil {
				t.Fatal("all three rows change in place")
			}
			if vis.Blank.Shown != tt.wantVis || pub.Blank.Shown != tt.wantPub || name.Blank.Shown != tt.wantShown {
				t.Errorf("got %q / %q / %q", vis.Blank.Shown, pub.Blank.Shown, name.Blank.Shown)
			}
			if vis.Blank.URL != "/campaigns/c/default-visibility" || pub.Blank.URL != "/campaigns/c/details" || name.Blank.URL != "/campaigns/c/details" || !pub.Blank.Bool {
				t.Errorf("blanks must save through their own routes, got %q, %q and %q", vis.Blank.URL, pub.Blank.URL, name.Blank.URL)
			}
			// A pick shows exactly the words the menu offered.
			for _, b := range []*DeskBlank{vis.Blank, pub.Blank} {
				found := false
				for _, o := range b.Options {
					found = found || o.Label == b.Shown
				}
				if !found {
					t.Errorf("%q shows %q, which is not one of its options %+v", b.ID, b.Shown, b.Options)
				}
			}
			if (vis.Blank.Off != "") != tt.off {
				t.Errorf("off = %q", vis.Blank.Off)
			}
		})
	}
}

func TestDeskBlanks_OffWhenArchived(t *testing.T) {
	now := time.Now()
	c := &Campaign{ID: "c", Settings: "{}", ArchivedAt: &now}
	for _, l := range []DeskLine{nameLine(c), pagesLine(c), publicLine(c)} {
		if l.Blank == nil || l.Blank.Off == "" {
			t.Errorf("row %q must be off on an archived campaign", l.ID)
		}
	}
}

func TestNeedsYou(t *testing.T) {
	c := &Campaign{ID: "c1"}
	now := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	old := now.Add(-10 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)
	tests := []struct {
		name string
		f    DeskFacts
		want int
	}{
		{"nothing measured", DeskFacts{}, 0},
		{"foundry quiet a few days", DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &recent}}}, 0},
		{"foundry silent over a week", DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, 1},
		{"foundry connected", DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, KeyLastUsed: &old}}}, 0},
		{"ai waiting", DeskFacts{AI: &DeskAIFacts{State: DeskAIConnected, Waiting: 2}}, 1},
		{"both", DeskFacts{AI: &DeskAIFacts{Waiting: 1}, Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(needsYou(c, tt.f, now)); got != tt.want {
				t.Errorf("rows = %d, want %d", got, tt.want)
			}
		})
	}
}

// stubProvider is a desk provider whose Facts is a function.
type stubProvider struct {
	slot DeskSlot
	fn   func(ctx context.Context) (DeskFacts, error)
}

func (s stubProvider) Slot() DeskSlot { return s.slot }
func (s stubProvider) Facts(ctx context.Context, _ DeskRequest) (DeskFacts, error) {
	return s.fn(ctx)
}

func lineByID(d *Desk, id string) *DeskLine {
	for gi := range d.Groups {
		for li := range d.Groups[gi].Lines {
			if d.Groups[gi].Lines[li].ID == id {
				return &d.Groups[gi].Lines[li]
			}
		}
	}
	return nil
}

func TestDeskService_Providers(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	in := DeskInputs{Campaign: c, Addons: map[string]bool{"media-gallery": true, "sync-api": true}}
	good := stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 3, Days: 30}}, nil
	}}
	tests := []struct {
		name      string
		bad       DeskProvider
		wantMedia string
	}{
		{"no provider", nil, "Its pictures and files are kept in Media."},
		{"provider errors", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { return DeskFacts{}, errors.New("db down") }}, "Its pictures and files are kept in Media."},
		{"provider panics", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { panic("boom") }}, "Its pictures and files are kept in Media."},
		{"provider fills another slot's pointer", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Trash: &DeskTrashFacts{Items: 99}}, nil
		}}, "Its pictures and files are kept in Media."},
		{"provider works", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Media: &DeskMediaFacts{Files: 1, Bytes: 10}}, nil
		}}, "Holds 1 picture or file, 10 B."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDeskService()
			s.Register(good)
			s.Register(nil) // a nil provider is ignored
			if tt.bad != nil {
				s.Register(tt.bad)
			}
			d := s.Build(context.Background(), DeskRequest{CampaignID: "c1"}, in)
			if got := deskText(*lineByID(d, "media")); got != tt.wantMedia {
				t.Errorf("media = %q, want %q", got, tt.wantMedia)
			}
			if got := deskText(*lineByID(d, "trash")); got != "Trash holds 3 items for 30 days." {
				t.Errorf("a failing neighbour changed the trash line: %q", got)
			}
		})
	}
}

func TestDeskService_SameSlotReplaces(t *testing.T) {
	s := NewDeskService()
	mk := func(n int) DeskProvider {
		return stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Trash: &DeskTrashFacts{Items: n, Days: 30}}, nil
		}}
	}
	s.Register(mk(1))
	s.Register(mk(2))
	d := s.Build(context.Background(), DeskRequest{}, DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}})
	if got := deskText(*lineByID(d, "trash")); got != "Trash holds 2 items for 30 days." {
		t.Errorf("second provider must replace the first, got %q", got)
	}
}

// A provider that ignores its context must not hold the desk shut.
func TestCallProvider_StuckProviderIsAbandoned(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	p := stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { <-release; return DeskFacts{}, nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := callProvider(ctx, p, DeskRequest{}); err == nil {
		t.Fatal("a stuck provider must come back as an error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("desk waited %v on a stuck provider", time.Since(start))
	}
}

func TestDeskService_DisabledFoundryDropsLine(t *testing.T) {
	s := NewDeskService()
	s.Register(stubProvider{DeskFoundry, func(context.Context) (DeskFacts, error) {
		old := time.Now().Add(-30 * 24 * time.Hour)
		return DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, nil
	}})
	d := s.Build(context.Background(), DeskRequest{Now: time.Now()}, DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}, Addons: map[string]bool{}})
	if lineByID(d, "foundry") != nil || len(d.Needs) != 0 {
		t.Error("with Foundry sync off the desk must neither draw nor flag Foundry")
	}
}

// deskStubService answers only what the desk handler reads.
type deskStubService struct{ CampaignService }

func (deskStubService) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return nil, nil
}

func deskRequestAs(h *Handler, role Role, dmGranted bool) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/dashboard", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set(contextKeyCampaign, &CampaignContext{
		Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: role, IsDmGranted: dmGranted, IsMember: true,
	})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	return rec, h.OwnerDashboard(c)
}

func TestOwnerDashboard_OnlyTheOwnerOpensTheDesk(t *testing.T) {
	tests := []struct {
		name      string
		role      Role
		dmGranted bool
		wantOpen  bool
	}{
		{"player", RolePlayer, false, false},
		{"scribe", RoleScribe, false, false},
		{"co-dm", RolePlayer, true, false},
		{"owner", RoleOwner, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(deskStubService{})
			// Providers are the owner's numbers: a lower role must never
			// reach them, so a provider that records its call proves it.
			called := false
			h.RegisterDeskProvider(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
				called = true
				return DeskFacts{Trash: &DeskTrashFacts{Items: 7, Days: 30}}, nil
			}})
			rec, err := deskRequestAs(h, tt.role, tt.dmGranted)
			if tt.wantOpen {
				if err != nil || rec.Code != http.StatusOK {
					t.Fatalf("owner: err=%v code=%d", err, rec.Code)
				}
				body := rec.Body.String()
				// "Needs you" is drawn only when something needs the owner.
				for _, want := range []string{
					"Campaign settings", "People", "What players and guests see", "Look and game", "Connections", "Safety net", "Archive or delete",
					"7 things, kept 30 days", `href="/campaigns/camp-1/extensions"`, `id="line-pages"`, `id="line-settings"`,
				} {
					if !strings.Contains(body, want) {
						t.Errorf("desk is missing %q", want)
					}
				}
				if strings.Contains(body, "The table") {
					t.Error("no group may be called The table: that is the dice sheet's name")
				}
				if strings.Count(body, "data-manage-find") > 1 {
					t.Errorf("the page draws at most one Find, got %d", strings.Count(body, "data-manage-find"))
				}
				if strings.Contains(body, "settings?tab=features") {
					t.Error("desk must not link to the retired Features settings tab")
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
				t.Fatalf("want forbidden, got %v", err)
			}
			if called || strings.Contains(rec.Body.String(), "7 things") {
				t.Error("a lower role reached the desk's numbers")
			}
		})
	}
}

func TestUpdateDetails_PartialSemantics(t *testing.T) {
	stored := func() *Campaign {
		return &Campaign{ID: "c1", Name: "Therin", Slug: "therin", IsPublic: true, Description: strPtr("kept"), Settings: "{}"}
	}
	type write struct {
		name, slug *string
		pub        *bool
	}
	tests := []struct {
		name    string
		input   UpdateCampaignDetailsInput
		want    *write // nil: no write at all
		wantErr bool
	}{
		{"absent everything writes nothing", UpdateCampaignDetailsInput{}, nil, false},
		{"name only leaves public alone", UpdateCampaignDetailsInput{Name: patch.Of("Shattered")}, &write{name: strPtr("Shattered"), slug: strPtr("shattered")}, false},
		{"public only leaves the name alone", UpdateCampaignDetailsInput{IsPublic: patch.Of(false)}, &write{pub: boolPtr(false)}, false},
		{"same value writes nothing", UpdateCampaignDetailsInput{IsPublic: patch.Of(true), Name: patch.Of("Therin")}, nil, false},
		{"empty name refused", UpdateCampaignDetailsInput{Name: patch.Of("   ")}, nil, true},
		{"explicit null name preserves", UpdateCampaignDetailsInput{Name: patch.Null[string]()}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *write
			repo := &mockCampaignRepo{
				findByIDFn: func(context.Context, string) (*Campaign, error) { return stored(), nil },
				updateDetailsFn: func(_ context.Context, _ string, n, s *string, p *bool) error {
					got = &write{n, s, p}
					return nil
				},
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})
			_, err := svc.UpdateDetails(context.Background(), "c1", tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("wrote %+v, want no write", got)
			case tt.want != nil && got == nil:
				t.Fatal("want a write, got none")
			case tt.want != nil:
				if !eqStr(got.name, tt.want.name) || !eqStr(got.slug, tt.want.slug) || (got.pub == nil) != (tt.want.pub == nil) || (got.pub != nil && *got.pub != *tt.want.pub) {
					t.Errorf("write = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func eqStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// deskDetailsService records what UpdateDetails was asked.
type deskDetailsService struct {
	CampaignService
	got   *UpdateCampaignDetailsInput
	after Campaign
}

func (s *deskDetailsService) UpdateDetails(_ context.Context, _ string, in UpdateCampaignDetailsInput) (*Campaign, error) {
	s.got = &in
	c := s.after
	return &c, nil
}

func TestUpdateDetailsAPI(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		body     string
		wantCode int
		wantCall bool
	}{
		{"player refused", RolePlayer, `{"isPublic":true}`, http.StatusForbidden, false},
		{"scribe refused", RoleScribe, `{"name":"x"}`, http.StatusForbidden, false},
		{"owner empty body", RoleOwner, `{}`, http.StatusBadRequest, false},
		{"owner public only", RoleOwner, `{"isPublic":false}`, http.StatusOK, true},
		{"owner name only", RoleOwner, `{"name":"New"}`, http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &deskDetailsService{after: Campaign{ID: "camp-1", Name: "New"}}
			h := NewHandler(svc)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/details", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1", Settings: "{}"}, MemberRole: tt.role, IsMember: true})
			auth.SetSession(c, &auth.Session{UserID: "u1"})

			err := h.UpdateDetailsAPI(c)
			code := rec.Code
			var ae *apperror.AppError
			if errors.As(err, &ae) {
				code = ae.Code
			} else if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if (svc.got != nil) != tt.wantCall {
				t.Errorf("service called = %v, want %v", svc.got != nil, tt.wantCall)
			}
			if tt.name == "owner public only" && (svc.got.Name.Present() || !svc.got.IsPublic.Present()) {
				t.Error("only the key sent may reach the service")
			}
		})
	}
}

func TestDeskPage_RendersRowsNeedsYouAndBlanks(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: RoleOwner}
	d := &Desk{
		CampaignName: "Therin",
		Needs: []NeedsYouItem{
			{Lead: "Foundry", Rest: "Stopped answering 9 days ago", URL: "/campaigns/camp-1/foundry", State: "warn"},
		},
		Groups: []DeskGroup{
			{Name: "People", Lines: []DeskLine{peopleLine(cc.Campaign, nil, 0)}},
			{Name: "Look and game", Lines: []DeskLine{nameLine(cc.Campaign), pagesLine(cc.Campaign), lookLine(cc.Campaign), deleteLine(cc.Campaign)}},
		},
	}
	var sb strings.Builder
	if err := DeskPage(cc, d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		`data-desk-needs`, `Warning: `, `Foundry`, `Stopped answering 9 days ago`, `href="/campaigns/camp-1/foundry"`,
		// A navigating row is one link over the whole row.
		`<a class="set-row" href="/campaigns/camp-1/members">`, `class="set-name">People`,
		`class="set-row set-danger"`,
		// A row that changes in place has a blank that answers both the pointer
		// and the keyboard from its own attributes, and the Saved tick's host.
		`role="button"`, `onclick=`, `onkeydown=`, `aria-haspopup="menu"`, `aria-expanded="false"`, `class="desk-acts"`,
		`id="line-pages"`, `id="line-name"`, `id="line-delete"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("desk is missing %q", want)
		}
	}
	if regexp.MustCompile(`<button[^>]*desk-b`).MatchString(html) {
		t.Error("a blank must not be a real button: buttons cannot wrap across lines")
	}
	for _, gone := range []string{"desk-chev", "data-fold", "data-carry", "Open People"} {
		if strings.Contains(html, gone) {
			t.Errorf("the rows no longer carry %q", gone)
		}
	}
}

func TestManageSentenceSwapRedrawsInPlace(t *testing.T) {
	var buf strings.Builder
	line := DeskLine{ID: "people", Parts: []DeskPart{txt("Just you, so far.")}}
	if err := manageSentenceSwap("people", line).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="manage-sentence"`, `hx-swap-oob="true"`, "Just you, so far."} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("swap lacks %q: %s", want, buf.String())
		}
	}
	buf.Reset()
	if err := manageSentence("people", line).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "hx-swap-oob") || !strings.Contains(buf.String(), `id="manage-sentence"`) {
		t.Errorf("the page's own sentence must carry the id and no swap: %s", buf.String())
	}
}

// The page's groups and rows, in order, with the add-on rows present only while
// their add-on is on. Line ids matter: links point at "#line-<id>".
func TestDeskService_Build_GroupsAndRows(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	ids := func(d *Desk) map[string][]string {
		out := map[string][]string{}
		for _, g := range d.Groups {
			for _, l := range g.Lines {
				out[g.Name] = append(out[g.Name], l.ID)
			}
		}
		return out
	}
	tests := []struct {
		name   string
		addons map[string]bool
		want   map[string][]string
	}{
		{"add-ons off", map[string]bool{}, map[string][]string{
			"People":                      {"people", "changes"},
			"What players and guests see": {"players-home", "guests-home", "pages", "public"},
			"Look and game":               {"name", "look", "game"},
			"Connections":                 {"ai", "keys"},
			"Safety net":                  {"export", "import", "activity", "trash"},
			"Archive or delete":           {"end", "delete"},
		}},
		{"media and Foundry on", map[string]bool{"media-gallery": true, "sync-api": true}, map[string][]string{
			"People":                      {"people", "changes"},
			"What players and guests see": {"players-home", "guests-home", "pages", "public"},
			"Look and game":               {"name", "look", "game", "media"},
			"Connections":                 {"foundry", "ai", "keys"},
			"Safety net":                  {"export", "import", "activity", "trash"},
			"Archive or delete":           {"end", "delete"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No provider answers: every row still draws, with its name alone.
			d := NewDeskService().Build(context.Background(), DeskRequest{Now: time.Now()}, DeskInputs{Campaign: c, Addons: tt.addons})
			got := ids(d)
			for group, want := range tt.want {
				if strings.Join(got[group], ",") != strings.Join(want, ",") {
					t.Errorf("%s = %v, want %v", group, got[group], want)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("groups = %v", got)
			}
			if l := lineByID(d, "changes"); l == nil || l.Short != "" || l.Name != "What players changed" {
				t.Errorf("a provider that did not answer leaves the row's name alone, got %+v", l)
			}
			for _, g := range d.Groups {
				if g.Name == "The table" {
					t.Error("a group is called The table, the dice sheet's name")
				}
			}
		})
	}
}

// The tool pages' sentence is still worded from the same lines: every line a
// page opens with keeps its sentence.
func TestDeskService_LineKeepsToolPageSentences(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	in := DeskInputs{Campaign: c, Members: members(2, RolePlayer), Addons: map[string]bool{"media-gallery": true, "sync-api": true}}
	for _, id := range []string{"people", "changes", "game", "media", "foundry", "ai", "trash", "settings"} {
		l, ok := NewDeskService().Line(context.Background(), DeskRequest{Now: time.Now()}, in, id)
		if !ok || len(l.Parts) == 0 {
			t.Errorf("line %s lost its sentence: ok=%v parts=%d", id, ok, len(l.Parts))
		}
	}
	if l, _ := NewDeskService().Line(context.Background(), DeskRequest{}, in, "settings"); !strings.Contains(deskText(l), "on this page") {
		t.Errorf("the Details and data sentence must say it is on this page: %q", deskText(l))
	}
}

// updateService records what Update was asked.
type updateService struct {
	CampaignService
	got *UpdateCampaignInput
}

func (s *updateService) Update(_ context.Context, _ string, in UpdateCampaignInput) (*Campaign, error) {
	s.got = &in
	return &Campaign{ID: "camp-1"}, nil
}

// The Details tab posts only the description. The handler must hand the service
// a description and nothing else, so the name and the public flag stay as they
// are; a body that names them still reaches the service.
func TestUpdate_FormBodyKeepsAbsentKeysAbsent(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantName   bool
		wantPublic bool
	}{
		{"description only", "description=New+words", false, false},
		{"old full form", "name=Therin&description=x&is_public=true", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &updateService{}
			h := NewHandler(svc)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1", Settings: "{}"}, MemberRole: RoleOwner, IsMember: true})
			auth.SetSession(c, &auth.Session{UserID: "u1"})
			if err := h.Update(c); err != nil {
				t.Fatalf("update: %v", err)
			}
			if svc.got == nil || svc.got.Description == nil {
				t.Fatalf("the description must reach the service: %+v", svc.got)
			}
			if (svc.got.Name != nil) != tt.wantName || (svc.got.IsPublic != nil) != tt.wantPublic {
				t.Errorf("name present = %v, public present = %v", svc.got.Name != nil, svc.got.IsPublic != nil)
			}
		})
	}
}

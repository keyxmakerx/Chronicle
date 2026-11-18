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
		case p.Blank != nil:
			sb.WriteString("[" + p.Blank.Shown + "]")
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
		{"people unread", peopleLine(c, nil), "Everyone in the campaign, and what each may do, is on the People page.", ""},
		{"people alone", peopleLine(c, []CampaignMember{{Role: RoleOwner}}), "Just you, so far.", ""},
		{"people players", peopleLine(c, []CampaignMember{{Role: RoleOwner}, {Role: RolePlayer}, {Role: RolePlayer}}), "3 people: you and 2 players.", ""},
		{"people one player one scribe", peopleLine(c, []CampaignMember{{Role: RoleOwner}, {Role: RolePlayer}, {Role: RoleScribe}}), "3 people: you, 1 scribe and a player.", ""},
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

func TestDeskPagesLine_Blanks(t *testing.T) {
	tests := []struct {
		name      string
		c         *Campaign
		wantVis   string
		wantPub   string
		wantAfter string
		off       bool
	}{
		{"open and private", &Campaign{ID: "c", Settings: "{}"}, "open to players", "private", deskPrivateAfter, false},
		{"hidden and public", &Campaign{ID: "c", IsPublic: true, Settings: `{"default_visibility":"dm_only"}`}, "hidden from players", "public", deskPublicAfter, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := pagesLine(tt.c)
			var vis, pub *DeskBlank
			var after string
			for _, p := range l.Parts {
				switch {
				case p.Blank != nil && p.Blank.ID == "visibility":
					vis = p.Blank
				case p.Blank != nil && p.Blank.ID == "public":
					pub = p.Blank
				case p.After:
					after = p.Text
				}
			}
			if vis == nil || pub == nil {
				t.Fatal("both blanks must be on the line")
			}
			if vis.Shown != tt.wantVis || pub.Shown != tt.wantPub || after != tt.wantAfter {
				t.Errorf("got %q / %q / %q", vis.Shown, pub.Shown, after)
			}
			if vis.URL != "/campaigns/c/default-visibility" || pub.URL != "/campaigns/c/details" || !pub.Bool {
				t.Errorf("blanks must save through their own routes, got %q and %q", vis.URL, pub.URL)
			}
			if (vis.Off != "") != tt.off {
				t.Errorf("off = %q", vis.Off)
			}
		})
	}
}

func TestDeskBlanks_OffWhenArchived(t *testing.T) {
	now := time.Now()
	c := &Campaign{ID: "c", Settings: "{}", ArchivedAt: &now}
	for _, l := range []DeskLine{nameLine(c), pagesLine(c)} {
		for _, p := range l.Parts {
			if p.Blank != nil && p.Blank.Off == "" {
				t.Errorf("blank %q must be off on an archived campaign", p.Blank.ID)
			}
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
				for _, want := range []string{"Needs you", "The table", "Trash holds 7 items", `href="/campaigns/camp-1/extensions"`} {
					if want == "Needs you" {
						continue // only when something needs the owner
					}
					if !strings.Contains(body, want) {
						t.Errorf("desk is missing %q", want)
					}
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
			if called || strings.Contains(rec.Body.String(), "Trash holds") {
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

func TestDeskPage_RendersNeedsYouAndBlanks(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: RoleOwner}
	d := &Desk{
		CampaignName: "Therin",
		Needs: []NeedsYouItem{
			{Lead: "Foundry stopped answering", Rest: " 9 days ago.", Action: "See why", URL: "/campaigns/camp-1/foundry", State: "warn"},
		},
		Groups: []DeskGroup{{Name: "The campaign", Lines: []DeskLine{nameLine(cc.Campaign), pagesLine(cc.Campaign)}}},
	}
	var sb strings.Builder
	if err := DeskPage(cc, d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		`data-desk-needs`, `Warning: `, `Foundry stopped answering`, `href="/campaigns/camp-1/foundry"`,
		// A blank is a span with a button role so it wraps like text, and it
		// answers both the pointer and the keyboard from its own attributes.
		`role="button"`, `onclick=`, `onkeydown=`, `aria-haspopup="menu"`, `aria-expanded="false"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("desk is missing %q", want)
		}
	}
	if regexp.MustCompile(`<button[^>]*desk-b`).MatchString(html) {
		t.Error("a blank must not be a real button: buttons cannot wrap across lines")
	}
}

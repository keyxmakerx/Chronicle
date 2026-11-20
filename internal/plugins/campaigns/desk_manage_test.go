// desk_manage_test.go pins the figures a Manage tool page draws beside its
// title: the People figure and its plurals, the service's per-page figures,
// the frame's figure resolver and the Needs-you count.

package campaigns

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func members(n int, role Role) []CampaignMember {
	out := []CampaignMember{{UserID: "owner", Role: RoleOwner, DisplayName: "Key"}}
	for i := 0; i < n; i++ {
		out = append(out, CampaignMember{UserID: "u" + string(rune('a'+i)), Role: role, DisplayName: "Person " + string(rune('A'+i))})
	}
	return out
}

func TestPeopleFigure(t *testing.T) {
	tests := []struct {
		name    string
		members []CampaignMember
		invited int
		read    bool
		want    string
	}{
		{"members unread", nil, 0, true, ""},
		{"alone, nobody waiting", members(0, RolePlayer), 0, true, "1 member"},
		{"three, nobody waiting", members(2, RolePlayer), 0, true, "3 members"},
		{"one waiting", members(2, RolePlayer), 1, true, "3 members · 1 invited"},
		{"alone with some waiting", members(0, RolePlayer), 2, true, "1 member · 2 invited"},
		{"invitations unread leave out the invited part", members(2, RolePlayer), 0, false, "3 members"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := peopleFigure(tt.members, DeskInputs{Invited: tt.invited, InvitedRead: tt.read})
			if got.Text() != tt.want || got.Empty() != (tt.want == "") {
				t.Errorf("figure = %q, want %q", got.Text(), tt.want)
			}
		})
	}
}

func TestDeskService_Figure(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	old := now.Add(-24 * 24 * time.Hour)
	s := NewDeskService()
	s.Register(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, nil
	}})
	s.Register(stubProvider{DeskChanges, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Changes: &DeskChangesFacts{Changes: 3, Players: 2}}, nil
	}})
	s.Register(stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Media: &DeskMediaFacts{Files: 212, Bytes: 310 << 20}}, nil
	}})
	s.Register(stubProvider{DeskFoundry, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, nil
	}})
	s.Register(stubProvider{DeskAI, func(context.Context) (DeskFacts, error) {
		return DeskFacts{AI: &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true}}, nil
	}})
	in := DeskInputs{
		Campaign: c, Members: members(4, RolePlayer), Invited: 1, InvitedRead: true,
		SystemID: "drawsteel", SystemName: "Draw Steel", FeaturesOn: 12, FeaturesTotal: 15,
		Addons: map[string]bool{"media-gallery": true, "sync-api": true},
	}
	tests := []struct {
		id        string
		want      string
		wantState string
		wantOK    bool
	}{
		{"people", "5 members · 1 invited", "", true},
		{"changes", "3 this week", "", true},
		{"media", "212 files · 310 MB", "", true},
		{"game", "Draw Steel · 12 of 15 on", "", true},
		{"foundry", "No answer · 24 days", "warn", true},
		{"ai", "Claude · can ask", "ok", true},
		{"trash", "2 items · kept 30 days", "", true},
		{"settings", "", "", true}, // Details and data shows nothing beside its title
		{"nonsense", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			f, ok := s.Figure(context.Background(), DeskRequest{Now: now}, in, tt.id)
			if ok != tt.wantOK || f.Text() != tt.want || f.State != tt.wantState {
				t.Errorf("got %q state %q ok=%v, want %q state %q ok=%v", f.Text(), f.State, ok, tt.want, tt.wantState, tt.wantOK)
			}
		})
	}
	// A page whose add-on is off has no figure at all, and a figure that
	// cannot be read is empty: the header shows nothing, never a placeholder.
	off := in
	off.Addons = map[string]bool{}
	for _, id := range []string{"media", "foundry"} {
		if _, ok := s.Figure(context.Background(), DeskRequest{Now: now}, off, id); ok {
			t.Errorf("%s has a figure with its add-on off", id)
		}
	}
	bare := NewDeskService()
	for _, id := range []string{"changes", "media", "foundry", "ai", "trash"} {
		if f, _ := bare.Figure(context.Background(), DeskRequest{Now: now}, in, id); !f.Empty() {
			t.Errorf("%s figure with no provider = %q, want nothing", id, f.Text())
		}
	}
}

// deskFigureService answers what ManageFigure reads.
type deskFigureService struct{ CampaignService }

func (deskFigureService) GetByID(_ context.Context, id string) (*Campaign, error) {
	return &Campaign{ID: id, Name: "Therin", Settings: "{}"}, nil
}
func (deskFigureService) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return members(3, RolePlayer), nil
}

func figureCtx(role int, in bool) context.Context {
	ctx := layouts.SetUserID(context.Background(), "owner")
	if in {
		ctx = layouts.SetCampaignID(ctx, "c1")
		ctx = layouts.SetCampaignRole(ctx, role)
	}
	return ctx
}

// A Manage page's own figures are the owner's alone and are the same figures
// the desk's rows carry, from the same provider.
func TestManageFigure(t *testing.T) {
	h := NewHandler(deskFigureService{})
	h.RegisterDeskProvider(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, nil
	}})
	tests := []struct {
		name string
		ctx  context.Context
		line string
		want string // "" means nothing at all
	}{
		{"owner, people", figureCtx(int(RoleOwner), true), "people", "4 members"},
		{"owner, trash", figureCtx(int(RoleOwner), true), "trash", "2 items · kept 30 days"},
		{"owner, a row with no page", figureCtx(int(RoleOwner), true), "keys", ""},
		{"owner, Details and data", figureCtx(int(RoleOwner), true), "settings", ""},
		{"scribe", figureCtx(int(RoleScribe), true), "people", ""},
		{"player", figureCtx(int(RolePlayer), true), "people", ""},
		{"owner viewing as a player reads as player", figureCtx(int(RolePlayer), true), "trash", ""},
		{"outside a campaign", figureCtx(0, false), "people", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := h.ManageFigure(tt.ctx, tt.line)
			if tt.want == "" {
				if comp != nil {
					t.Fatal("this viewer or page must get no figure")
				}
				return
			}
			if comp == nil {
				t.Fatal("want a figure")
			}
			var sb strings.Builder
			if err := comp.Render(tt.ctx, &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if !strings.Contains(out, tt.want) || !strings.Contains(out, `data-manage-figure`) || !strings.Contains(out, `data-line="`+tt.line+`"`) || !strings.Contains(out, `id="manage-figure"`) {
				t.Errorf("figure = %s, want it to say %q", out, tt.want)
			}
			if strings.Contains(out, "hx-swap-oob") {
				t.Errorf("the page's own figure must carry no swap: %s", out)
			}
		})
	}
}

// After a change the response redraws the figure in place, and a figure that
// can no longer be read is swapped for an empty one rather than left stale.
func TestManageFigureSwap(t *testing.T) {
	h := NewHandler(deskFigureService{})
	var sb strings.Builder
	if err := h.manageFigureSwap("people").Render(figureCtx(int(RoleOwner), true), &sb); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="manage-figure"`, `hx-swap-oob="true"`, "4 members"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("swap lacks %q: %s", want, sb.String())
		}
	}
	sb.Reset()
	if err := manageFigureSwap("people", DeskFigure{}).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `hx-swap-oob="true"`) || strings.Contains(sb.String(), "member") {
		t.Errorf("an unreadable figure swaps in empty: %s", sb.String())
	}
}

func TestDeskService_NeedsCount(t *testing.T) {
	tests := []struct {
		name   string
		addons map[string]bool
		ai     int
		want   int
	}{
		{"nothing waits", map[string]bool{}, 0, 0},
		{"the AI has a request waiting", map[string]bool{}, 2, 1},
		{"Foundry facts of a campaign without the add-on are not counted", map[string]bool{"sync-api": false}, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDeskService()
			s.Register(stubProvider{DeskAI, func(context.Context) (DeskFacts, error) {
				return DeskFacts{AI: &DeskAIFacts{Waiting: tt.ai}}, nil
			}})
			// A provider outside Needs-you must not be asked.
			s.Register(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
				t.Error("the trash provider was read for a count")
				return DeskFacts{}, nil
			}})
			in := DeskInputs{Campaign: &Campaign{ID: "c1"}, Addons: tt.addons}
			if got := s.NeedsCount(context.Background(), DeskRequest{Now: time.Now()}, in); got != tt.want {
				t.Fatalf("count = %d, want %d", got, tt.want)
			}
		})
	}
}

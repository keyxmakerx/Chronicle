// desk_manage_test.go pins the lines a Manage tool page opens with: the
// people sentence, the service's single-line wording, the frame's sentence
// resolver and the Needs-you count.

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


func TestPeopleLine_WaitingInvites(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	tests := []struct {
		name    string
		members []CampaignMember
		invited int
		want    string
	}{
		{"none waiting", members(2, RolePlayer), 0, "3 people: you and 2 players."},
		{"one waiting", members(2, RolePlayer), 1, "3 people: you and 2 players, with 1 invited."},
		{"alone with some waiting", members(0, RolePlayer), 2, "Just you, so far, with 2 invited."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deskText(peopleLine(c, tt.members, tt.invited)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}


func TestDeskService_Line(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	s := NewDeskService()
	s.Register(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, nil
	}})
	in := DeskInputs{Campaign: c, Members: members(1, RoleScribe), Invited: 1, Addons: map[string]bool{}}
	tests := []struct {
		id     string
		want   string
		wantOK bool
	}{
		{"people", "2 people: you and 1 scribe, with 1 invited.", true},
		{"trash", "Trash holds 2 items for 30 days.", true},
		{"media", "", false}, // its add-on is off
		{"foundry", "", false},
		{"nonsense", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			l, ok := s.Line(context.Background(), DeskRequest{}, in, tt.id)
			if ok != tt.wantOK || (ok && deskText(l) != tt.want) {
				t.Errorf("got %q ok=%v, want %q ok=%v", deskText(l), ok, tt.want, tt.wantOK)
			}
		})
	}
}


func ptrTime(t time.Time) *time.Time { return &t }

// deskSentenceService answers what ManageSentence reads.
type deskSentenceService struct{ CampaignService }

func (deskSentenceService) GetByID(_ context.Context, id string) (*Campaign, error) {
	return &Campaign{ID: id, Name: "Therin", Settings: "{}"}, nil
}
func (deskSentenceService) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return members(3, RolePlayer), nil
}

func sentenceCtx(role int, in bool) context.Context {
	ctx := layouts.SetUserID(context.Background(), "owner")
	if in {
		ctx = layouts.SetCampaignID(ctx, "c1")
		ctx = layouts.SetCampaignRole(ctx, role)
	}
	return ctx
}

// A Manage page's own sentence is the owner's alone and is the same words the
// desk shows, from the same provider.
func TestManageSentence(t *testing.T) {
	h := NewHandler(deskSentenceService{})
	h.RegisterDeskProvider(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, nil
	}})
	tests := []struct {
		name string
		ctx  context.Context
		line string
		want string // "" means no sentence at all
	}{
		{"owner, people", sentenceCtx(int(RoleOwner), true), "people", "4 people: you and 3 players."},
		{"owner, trash", sentenceCtx(int(RoleOwner), true), "trash", "Trash holds 2 items for 30 days."},
		{"owner, a line with no page", sentenceCtx(int(RoleOwner), true), "keys", ""},
		{"scribe", sentenceCtx(int(RoleScribe), true), "people", ""},
		{"player", sentenceCtx(int(RolePlayer), true), "people", ""},
		{"owner viewing as a player reads as player", sentenceCtx(int(RolePlayer), true), "trash", ""},
		{"outside a campaign", sentenceCtx(0, false), "people", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := h.ManageSentence(tt.ctx, tt.line)
			if tt.want == "" {
				if comp != nil {
					t.Fatal("this viewer or line must get no sentence")
				}
				return
			}
			if comp == nil {
				t.Fatal("want a sentence")
			}
			var sb strings.Builder
			if err := comp.Render(tt.ctx, &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if !strings.Contains(out, tt.want) || !strings.Contains(out, `data-manage-sentence`) || !strings.Contains(out, `data-line="`+tt.line+`"`) {
				t.Errorf("sentence = %s, want it to say %q", out, tt.want)
			}
		})
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

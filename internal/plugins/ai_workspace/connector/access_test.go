package connector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// An owner's Safe key reads as a player, so it is offered none of the
// owner's tools, built-in or plugged in, and can't call one by name. The
// same owner set to Permitted gets every one of them.
func TestOwnerSafeKey_NoOwnerTools(t *testing.T) {
	l := &records.Lookups{Plugged: []aiplug.Lookup{&plugLookup{owner: true}}}
	var owned []string
	for _, tl := range toolsFor(&Connection{Privacy: aiexport.PrivacyModePermitted}, l) {
		if tl.Owner {
			owned = append(owned, tl.Name)
		}
	}
	if all := "," + strings.Join(owned, ",") + ","; !strings.Contains(all, ",rolling_tables,") || !strings.Contains(all, ",widgets,") {
		t.Fatalf("a Permitted owner key lists the owner's tools %v, want built-in and plugged ones", owned)
	}
	cases := []struct {
		name string
		conn *Connection
	}{
		{"owner, Safe, only looks", &Connection{ID: 1, CampaignID: "camp-1", UserID: "owner-1", Privacy: aiexport.PrivacyModeSafe}},
		{"owner, Safe, may ask", &Connection{ID: 1, CampaignID: "camp-1", UserID: "owner-1", Privacy: aiexport.PrivacyModeSafe, CanAsk: true}},
		{"player", &Connection{ID: 2, CampaignID: "camp-1", UserID: "player-1", ForPlayer: true, Privacy: aiexport.PrivacyModeSafe}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, tl := range toolsFor(tc.conn, l) {
				if tl.Owner {
					t.Errorf("lists %s", tl.Name)
				}
			}
			s := &Server{Lookups: l}
			sess := Session{Conn: tc.conn, CampaignName: "Ashfall", Actor: actorFor(tc.conn)}
			for _, name := range owned {
				out := s.Handle(context.Background(), sess, callBody(name, nil))
				var r rpcReply
				if err := json.Unmarshal(out.Body, &r); err != nil || r.Error == nil || out.Called != nil {
					t.Errorf("%s answered: %s", name, out.Body)
				}
			}
		})
	}
}

// outlinePages is a campaign with one hidden page, as the role filter reads it.
type outlinePages struct{}

func (outlinePages) List(_ context.Context, _ string, _ int, role int, _ string, o entities.ListOptions) ([]entities.Entity, int, error) {
	if o.Page > 1 {
		return nil, 0, nil
	}
	out := []entities.Entity{{ID: "inn", CampaignID: "camp-1", Name: "The Rusty Anchor", TypeName: "Location", TypeNamePlural: "Locations"}}
	if role >= 3 {
		out = append(out, entities.Entity{ID: "ghost", CampaignID: "camp-1", Name: "The Ghost", TypeName: "Character", TypeNamePlural: "Characters", IsPrivate: true})
	}
	return out, len(out), nil
}

func (outlinePages) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return nil, nil
}

// campaign_outline opens with who the key reads as and whether it may ask,
// in words, before the pages, so an AI that never saw the initialize
// instructions still knows what it may repeat to players.
func TestOutline_SaysWhatTheConnectionReads(t *testing.T) {
	cases := []struct {
		name    string
		conn    *Connection
		want    []string
		notWant []string
	}{
		{"owner, Permitted, only looks",
			&Connection{UserID: "owner-1", Privacy: aiexport.PrivacyModePermitted},
			[]string{"Reads as the campaign's owner", "GM-only pages and secret text included", "{gm{", "it only reads", "Your AI page", "The Ghost (GM only)"},
			[]string{"may ask"}},
		{"owner, Permitted, may ask",
			&Connection{UserID: "owner-1", Privacy: aiexport.PrivacyModePermitted, CanAsk: true},
			[]string{"Reads as the campaign's owner", "may ask for changes, which wait for the owner's yes"}, nil},
		{"owner, Safe",
			&Connection{UserID: "owner-1", Privacy: aiexport.PrivacyModeSafe},
			[]string{"Reads as the campaign's players", "GM-only pages, secret text, notes", "it only reads", "Your AI page"},
			[]string{"Ghost", "(GM only)", "owner: everything"}},
		{"player, may ask",
			&Connection{UserID: "player-1", ForPlayer: true, CanAsk: true},
			[]string{"Reads as you, as a player", "pages you claimed and those shared with you", "Nothing GM-only", "your own character's sheet", "your AI panel"},
			[]string{"Ghost", "(GM only)", "Your AI page"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.conn.CampaignID = "camp-1"
			s := &Server{Lookups: &records.Lookups{Pages: outlinePages{}}}
			sess := Session{Conn: tc.conn, CampaignName: "Ashfall", Actor: actorFor(tc.conn)}
			got := toolText(t, s.Handle(context.Background(), sess, callBody("campaign_outline", nil))).Content[0].Text
			if !strings.HasPrefix(got, "# Ashfall\n\n## This connection\n\n- Reads as ") {
				t.Errorf("the outline doesn't open with the connection:\n%s", got)
			}
			if i, j := strings.Index(got, "## This connection"), strings.Index(got, "## Pages by kind"); j < 0 || i > j {
				t.Errorf("the connection should come before the pages:\n%s", got)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
		})
	}
}

// The outline's description tells the AI the access line is there.
func TestOutline_DescriptionPointsAtTheAccessLine(t *testing.T) {
	tl := findTool(&Connection{}, nil, "campaign_outline")
	for _, forPlayer := range []bool{false, true} {
		if d := tl.describe(forPlayer); !strings.Contains(d, "what this connection reads as") {
			t.Errorf("forPlayer=%v: %q", forPlayer, d)
		}
	}
}

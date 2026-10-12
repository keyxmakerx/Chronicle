package entities

// players_box_test.go pins the Players box and the pick box as the operator
// signed them: a row per player with chips that unclaim, the Free to pick strip
// with chips that take a character off it, one finder placed by two buttons,
// and the pick box shown only to a player who has characters to pick.

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func rosterFixture() *ClaimRoster {
	ana, bo := "u1", "u2"
	return &ClaimRoster{
		Characters: []Entity{
			{ID: "c1", Name: "Tyne", OwnerUserID: &ana, Playable: true},
			{ID: "c2", Name: "Old Hob", Playable: true},
			{ID: "c3", Name: "Vex of the Hollow", Playable: true},
			{ID: "c4", Name: "Bren", OwnerUserID: &bo, Playable: true},
		},
		Members: []campaigns.CampaignMember{
			{UserID: "u1", DisplayName: "Ana", Role: campaigns.RolePlayer},
			{UserID: "u2", DisplayName: "Bo", Role: campaigns.RoleScribe}, // owns Bren, so still listed
			{UserID: "u3", DisplayName: "Cy", Role: campaigns.RolePlayer},
			{UserID: "u4", DisplayName: "Dee", Role: campaigns.RoleScribe}, // plays nothing, so not listed
		},
	}
}

func TestRosterPlayerRows(t *testing.T) {
	rows := rosterPlayerRows(rosterFixture())
	var got []string
	for _, r := range rows {
		var chars []string
		for _, c := range r.Chars {
			chars = append(chars, c.Name)
		}
		got = append(got, r.Name+":"+strings.Join(chars, ","))
	}
	want := "Ana:Tyne|Bo:Bren|Cy:"
	if strings.Join(got, "|") != want {
		t.Errorf("rows = %q, want %q", strings.Join(got, "|"), want)
	}
	var free []string
	for _, c := range rosterFreeCharacters(rosterFixture()) {
		free = append(free, c.Name)
	}
	if strings.Join(free, ",") != "Old Hob,Vex of the Hollow" {
		t.Errorf("free = %v", free)
	}
}

func TestMemberInitial(t *testing.T) {
	for in, want := range map[string]string{"Ana": "A", " bo": "B", "éla": "É", "": "?"} {
		if got := memberInitial(in); got != want {
			t.Errorf("memberInitial(%q) = %q, want %q", in, got, want)
		}
	}
}

func renderPlayersBox(t *testing.T, roster *ClaimRoster) string {
	t.Helper()
	var sb strings.Builder
	if err := castPlayersBox("camp1", "tok", roster, true).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func TestCastPlayersBox(t *testing.T) {
	out := renderPlayersBox(t, rosterFixture())
	tests := []struct {
		name string
		want string
		n    int // exact count; 0 means "at least once"
	}{
		{"heading", "<h4>Players</h4>", 1},
		{"a row per listed player", `class="pw-row"`, 3},
		{"avatar initial", `<span class="pw-av" aria-hidden="true">A</span>`, 1},
		{"a player with none says so", `<span class="pw-none">No character yet</span>`, 1},
		{"a chip per owned character", `class="pw-chip"`, 2},
		{"a chip per free character", `class="pw-chip pw-free"`, 2},
		{"each owned chip unclaims through the owner route", `hx-post="/campaigns/camp1/characters/owner"`, 2},
		{"unclaim posts an empty owner", `&#34;owner_user_id&#34;:&#34;&#34;`, 2},
		{"free chips leave Free to pick through the playable route", `hx-post="/campaigns/camp1/characters/playable"`, 2},
		{"free chips post playable 0", `&#34;playable&#34;:&#34;0&#34;`, 2},
		{"a Give button per player", "Give a character", 3},
		{"one Add button", "Add a character", 1},
		{"free heading", "<h5>Free to pick</h5>", 1},
		{"one finder", `class="pw-find"`, 1},
		{"finder searches through the new route", `hx-get="/campaigns/camp1/characters/players/search"`, 1},
		{"csrf is inherited by every button", `{&#34;X-CSRF-Token&#34;:&#34;tok&#34;}`, 1},
		{"the hint", "A player with no character chooses from these.", 1},
	}
	for _, tc := range tests {
		got := strings.Count(out, tc.want)
		if tc.n == 0 && got == 0 || tc.n > 0 && got != tc.n {
			t.Errorf("%s: %q appears %d times, want %d", tc.name, tc.want, got, tc.n)
		}
	}
	if strings.Contains(out, "<script") {
		t.Error("the box must not emit a script tag")
	}
	if strings.Contains(out, "Dee") {
		t.Error("a non-player who plays nothing must not get a row")
	}
}

func TestCastPlayersBox_EmptyStates(t *testing.T) {
	out := renderPlayersBox(t, &ClaimRoster{Members: []campaigns.CampaignMember{{UserID: "u1", DisplayName: "Dee", Role: campaigns.RoleScribe}}})
	for _, want := range []string{"No players have joined yet.", "Nobody is free to pick yet", "Add a character"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty box lacks %q", want)
		}
	}
	if strings.Contains(out, `class="pw-list"`) {
		t.Error("no player list without players")
	}
}

func TestCharactersContent_PickBox(t *testing.T) {
	pick := []Entity{{ID: "c2", Name: "Old Hob"}, {ID: "c3", Name: "Vex of the Hollow"}}
	party := []CastMember{{Entity: Entity{ID: "a", Name: "Aldric"}}}

	shown := renderCastContent(t, CastView{ShowPlayers: true, PartyListed: true, Party: party, Pickable: pick, CSRFToken: "tok"})
	for _, want := range []string{
		"Pick your character",
		"These characters are free to play. Pick one to make it yours. Your GM can change it later.",
		`aria-label="Pick Old Hob"`, `aria-label="Pick Vex of the Hollow"`,
		"Make Old Hob yours? Only your GM can undo this.",
		"Yes, it’s mine",
		`hx-post="/campaigns/camp1/entities/c2/claim"`,
		`hx-post="/campaigns/camp1/entities/c3/claim"`,
		`&#34;from&#34;:&#34;characters&#34;`,
	} {
		if !strings.Contains(shown, want) {
			t.Errorf("pick box lacks %q", want)
		}
	}
	if strings.Index(shown, "Pick your character") > strings.Index(shown, "The Party") {
		t.Error("the pick box sits above The Party")
	}
	if strings.Count(shown, `class="ag-confirm-row"`) != 2 || strings.Count(shown, " hidden>") < 2 {
		t.Error("each character has its own hidden confirm row")
	}
	if strings.Contains(shown, "<script") {
		t.Error("the pick box must not emit a script tag")
	}

	for name, view := range map[string]CastView{
		"nothing to pick":      {ShowPlayers: true, PartyListed: true, Party: party},
		"staff roster instead": {ShowPlayers: true, PartyListed: true, Party: party, Roster: rosterFixture()},
	} {
		if out := renderCastContent(t, view); strings.Contains(out, "Pick your character") {
			t.Errorf("%s: the pick box must not show", name)
		}
	}
}

func TestCharactersContent_PlayersButton(t *testing.T) {
	out := renderCastContent(t, CastView{ShowPlayers: true, PartyListed: true, Roster: rosterFixture(), CSRFToken: "tok"})
	for _, want := range []string{`data-opens="players"`, "</i> Players</button>", `data-box="players"`} {
		if !strings.Contains(out, want) {
			t.Errorf("staff page lacks %q", want)
		}
	}
	if strings.Contains(out, "Who plays whom") {
		t.Error("the old name is gone")
	}
}

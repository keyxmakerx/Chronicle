package entities

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func renderStanding(t *testing.T, v *StandingView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := BlockStanding(stCamp, stPage, "The Lantern Guild", v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func viewFor(t *testing.T, repo *fakeStandingRepo, role int) *StandingView {
	t.Helper()
	v, err := NewStandingService(repo, fakeGate{}).View(context.Background(), stCamp, stPage, role, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Hidden {
		t.Fatalf("view for role %d is hidden", role)
	}
	return v
}

// The rendered block, whole, for a player and for the Director. A player's page
// source (markup AND the JSON the script reads) holds none of the Director's
// words and none of the Director's controls; the Director's holds them marked.
func TestBlockStanding_PlayerMarkupHoldsNothingOfTheDirectors(t *testing.T) {
	repo := seededStanding()
	player := renderStanding(t, viewFor(t, repo, permissions.RolePlayer))
	dm := renderStanding(t, viewFor(t, repo, permissions.RoleOwner))

	for _, bad := range []string{secret, "other secret", "data-dm-only", "data-sg-add", "data-sg-form", "data-sg-see", "sg-dmtag", `"dmOnly"`, `"playersSee"`, "Tobin", "Add a reason", "Players can see"} {
		if strings.Contains(player, bad) {
			t.Errorf("player's block contains %q", bad)
		}
	}
	for _, want := range []string{"Saved the granary", "Why: Saved the granary", `data-dm="0"`, "Wary", "aria-label=\"Standing: Wary, step 2 of 5\""} {
		if !strings.Contains(player, want) {
			t.Errorf("player's block is missing %q", want)
		}
	}
	for _, want := range []string{secret, "data-dm-only", "data-sg-add", "data-sg-form", "data-sg-see", `data-dm="1"`, "Add a reason", "Players can see this standing", `"dmOnly":true`} {
		if !strings.Contains(dm, want) {
			t.Errorf("director's block is missing %q", want)
		}
	}
}

func TestBlockStanding_AmberEdgeAndDMTagOnlyWhenPlayersCannotSee(t *testing.T) {
	tests := []struct {
		name      string
		switchOff bool
		wantEdge  bool
	}{
		{"players can see: no amber edge on the word", false, false},
		{"players cannot see: amber edge and the DM tag on the word", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			if tt.switchOff {
				repo.playersSee[stPage] = false
			}
			html := renderStanding(t, viewFor(t, repo, permissions.RoleOwner))
			row := regexp.MustCompile(`<div class="([^"]*)" data-sg-srow`).FindStringSubmatch(html)
			if row == nil {
				t.Fatal("no standing row in the block")
			}
			if has := strings.Contains(row[1], "sg-dm"); has != tt.wantEdge {
				t.Errorf("row class %q has sg-dm = %v, want %v", row[1], has, tt.wantEdge)
			}
			tag := regexp.MustCompile(`data-sg-dmtag([^>]*)>GM`).FindStringSubmatch(html)
			if tag == nil {
				t.Fatal("no DM tag element")
			}
			if hidden := strings.Contains(tag[1], "hidden"); hidden == tt.wantEdge {
				t.Errorf("DM tag hidden = %v when the edge is %v", hidden, tt.wantEdge)
			}
		})
	}
}

// Swap safety: the controls are inline IIFEs written Go-side. A templ script
// helper emits a <script> sibling that an HTMX swap does not run.
func TestBlockStanding_HandlersAreInlineIIFEs(t *testing.T) {
	html := renderStanding(t, viewFor(t, seededStanding(), permissions.RoleOwner))
	if !strings.Contains(html, "window.ChronicleStanding.toggle(el,e)") {
		t.Error("the toggle handler is not an inline IIFE")
	}
	for _, m := range regexp.MustCompile(`<script[^>]*>`).FindAllString(html, -1) {
		if !strings.Contains(m, `type="application/json"`) {
			t.Errorf("block emits an executable script tag: %s", m)
		}
	}
	if strings.Contains(html, "addEventListener") {
		t.Error("the block's markup wires a listener itself")
	}
	if n := strings.Count(html, "<script"); n != 1 {
		t.Errorf("script tags = %d, want only the data script", n)
	}
}

func TestBlockStanding_ASubjectPickerOnlyWhenThereIsAChoice(t *testing.T) {
	repo := newFakeStandingRepo()
	repo.rows = []StandingRow{{ID: "st-party", CampaignID: stCamp, EntityID: stPage, SubjectKey: StandingPartyKey, Level: 3}}
	one := renderStanding(t, viewFor(t, repo, permissions.RolePlayer))
	if strings.Contains(one, "data-sg-subjbtn") {
		t.Error("a player with one subject got a picker")
	}
	many := renderStanding(t, viewFor(t, seededStanding(), permissions.RoleOwner))
	if !strings.Contains(many, "data-sg-subjbtn") || !strings.Contains(many, ">Mira<") {
		t.Error("the Director with three subjects got no picker")
	}
}

func TestBlockStanding_UnsetSubjectSaysSo(t *testing.T) {
	html := renderStanding(t, viewFor(t, newFakeStandingRepo(), permissions.RoleOwner))
	if !strings.Contains(html, "Not set yet") || !strings.Contains(html, "Why: no reasons shared yet") {
		t.Errorf("an unset standing does not say so: %s", html)
	}
}

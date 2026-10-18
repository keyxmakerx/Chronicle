package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

const (
	stCamp  = "camp-1"
	stPage  = "guild-page"
	stMira  = "5f0a7c52-6b64-4d6f-8d0c-0b6f3a1f1111"
	stTobin = "5f0a7c52-6b64-4d6f-8d0c-0b6f3a1f2222"
	// secret is text that must never reach a player.
	secret = "Secretly paid off by the Duke"
)

func stReason(id, standing, dir, text string, dm bool, age int) StandingReason {
	return StandingReason{ID: id, StandingID: standing, Direction: dir, Text: text, When: "session 1", DMOnly: dm, CreatedAt: time.Unix(int64(5000-age), 0)}
}

// seededStanding is a guild page with a party standing (one public reason,
// one DM-only), a Mira standing with only a DM-only reason, and Tobin unset.
func seededStanding() *fakeStandingRepo {
	f := newFakeStandingRepo()
	f.names[stMira], f.names[stTobin] = "Mira", "Tobin"
	f.chars = []StandingSubjectRef{{Key: stMira, Name: "Mira"}, {Key: stTobin, Name: "Tobin"}}
	f.rows = []StandingRow{
		{ID: "st-party", CampaignID: stCamp, EntityID: stPage, SubjectKey: StandingPartyKey, Level: 1},
		{ID: "st-mira", CampaignID: stCamp, EntityID: stPage, SubjectKey: stMira, Level: 3},
	}
	f.reasons = []StandingReason{
		stReason("r1", "st-party", StandingUp, "Saved the granary", false, 1),
		stReason("r2", "st-party", StandingDown, secret, true, 0),
		stReason("r3", "st-mira", StandingUp, "Kept the Guildmaster's other secret", true, 2),
	}
	return f
}

func textsOf(s StandingSubjectView) []string {
	var out []string
	for _, r := range s.Reasons {
		out = append(out, r.Text)
	}
	return out
}

func TestStandingView_WhoSeesWhat(t *testing.T) {
	tests := []struct {
		name         string
		role         int
		leaky        bool
		switchOff    bool
		hideMira     bool
		wantHidden   bool
		wantSubjects []string // keys in order
		wantParty    []string // reason texts of the party subject
		wantTotal    int      // party Total
		wantSwitch   bool     // PlayersSee present in the view
	}{
		{name: "player: word plus public reasons only, only set subjects", role: permissions.RolePlayer,
			wantSubjects: []string{StandingPartyWire, stMira}, wantParty: []string{"Saved the granary"}, wantTotal: 1},
		{name: "player with a leaky repository still gets no DM-only reason", role: permissions.RolePlayer, leaky: true,
			wantSubjects: []string{StandingPartyWire, stMira}, wantParty: []string{"Saved the granary"}, wantTotal: 1},
		{name: "player: switch off means nothing at all", role: permissions.RolePlayer, switchOff: true, wantHidden: true},
		{name: "scribe is a player here", role: permissions.RoleScribe,
			wantSubjects: []string{StandingPartyWire, stMira}, wantParty: []string{"Saved the granary"}, wantTotal: 1},
		{name: "director: everything, unset characters too, with the switch", role: permissions.RoleOwner,
			wantSubjects: []string{StandingPartyWire, stMira, stTobin}, wantParty: []string{secret, "Saved the granary"}, wantTotal: 2, wantSwitch: true},
		{name: "director still sees the page when the switch is off", role: permissions.RoleOwner, switchOff: true,
			wantSubjects: []string{StandingPartyWire, stMira, stTobin}, wantParty: []string{secret, "Saved the granary"}, wantTotal: 2, wantSwitch: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			repo.leaky = tt.leaky
			if tt.switchOff {
				repo.playersSee[stPage] = false
			}
			svc := NewStandingService(repo, fakeGate{})
			v, err := svc.View(context.Background(), stCamp, stPage, tt.role, "u1")
			if err != nil {
				t.Fatal(err)
			}
			if v.Hidden != tt.wantHidden {
				t.Fatalf("Hidden = %v, want %v", v.Hidden, tt.wantHidden)
			}
			if tt.wantHidden {
				if repo.reads != 0 {
					t.Errorf("a hidden block read %d standings tables, want 0", repo.reads)
				}
				return
			}
			var keys []string
			for _, s := range v.Subjects {
				keys = append(keys, s.Key)
			}
			if strings.Join(keys, ",") != strings.Join(tt.wantSubjects, ",") {
				t.Fatalf("subjects = %v, want %v", keys, tt.wantSubjects)
			}
			if got := textsOf(v.Subjects[0]); strings.Join(got, "|") != strings.Join(tt.wantParty, "|") {
				t.Errorf("party reasons = %v, want %v", got, tt.wantParty)
			}
			// Mira has a word but only a DM-only reason: a player gets the word and no reasons.
			if tt.role < permissions.RoleOwner {
				if m := v.Subjects[1]; m.Key != stMira || !m.Set || len(m.Reasons) != 0 || m.Total != 0 {
					t.Errorf("player's Mira = %+v, want her word and no reasons", m)
				}
			}
			if v.Subjects[0].Total != tt.wantTotal {
				t.Errorf("party total = %d, want %d", v.Subjects[0].Total, tt.wantTotal)
			}
			if (v.PlayersSee != nil) != tt.wantSwitch {
				t.Errorf("PlayersSee present = %v, want %v", v.PlayersSee != nil, tt.wantSwitch)
			}
			if v.CanEdit != (tt.role >= permissions.RoleOwner) {
				t.Errorf("CanEdit = %v for role %d", v.CanEdit, tt.role)
			}
			// Whatever the view says, a player's JSON carries none of the Director's words.
			if tt.role < permissions.RoleOwner {
				raw, _ := json.Marshal(v)
				for _, bad := range []string{secret, "other secret", "dmOnly", "Tobin"} {
					if strings.Contains(string(raw), bad) {
						t.Errorf("a player's view contains %q: %s", bad, raw)
					}
				}
			}
		})
	}
}

func TestStandingView_PlayerCharacterSubjects(t *testing.T) {
	tests := []struct {
		name string
		hide map[string]bool
		want []string
	}{
		{"a character the player can open is listed", nil, []string{StandingPartyWire, stMira}},
		{"a character page hidden from the player is not named", map[string]bool{stMira: true}, []string{StandingPartyWire}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			// Give Mira a public reason so her standing would otherwise show.
			repo.reasons = append(repo.reasons, stReason("r4", "st-mira", StandingUp, "Vouched for an apprentice", false, 3))
			svc := NewStandingService(repo, fakeGate{hidden: tt.hide})
			v, err := svc.View(context.Background(), stCamp, stPage, permissions.RolePlayer, "u1")
			if err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, s := range v.Subjects {
				keys = append(keys, s.Key)
			}
			if strings.Join(keys, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("subjects = %v, want %v", keys, tt.want)
			}
			raw, _ := json.Marshal(v)
			if tt.hide != nil && strings.Contains(string(raw), "Mira") {
				t.Errorf("hidden character's name leaked: %s", raw)
			}
		})
	}
}

func TestStandingView_PlayerWithNothingToSeeGetsNoBlock(t *testing.T) {
	repo := newFakeStandingRepo() // no standing set anywhere
	svc := NewStandingService(repo, fakeGate{})
	v, err := svc.View(context.Background(), stCamp, stPage, permissions.RolePlayer, "u1")
	if err != nil || !v.Hidden {
		t.Fatalf("View = %+v, %v; want Hidden", v, err)
	}
	dm, err := svc.View(context.Background(), stCamp, stPage, permissions.RoleOwner, "u1")
	if err != nil || dm.Hidden || len(dm.Subjects) != 1 || dm.Subjects[0].Set {
		t.Fatalf("director on an empty page = %+v, %v; want the party, unset", dm, err)
	}
}

func TestStandingView_OrphanedAndOverCapSubjects(t *testing.T) {
	repo := seededStanding()
	delete(repo.names, stMira) // Mira's page was deleted
	repo.chars = []StandingSubjectRef{{Key: stTobin, Name: "Tobin"}}
	svc := NewStandingService(repo, fakeGate{})
	v, _ := svc.View(context.Background(), stCamp, stPage, permissions.RoleOwner, "u1")
	for _, s := range v.Subjects {
		if s.Key == stMira {
			t.Error("a deleted character's standing was still listed")
		}
	}
}

func TestStandingSubject(t *testing.T) {
	tests := []struct {
		name      string
		role      int
		subject   string
		switchOff bool
		hide      map[string]bool
		wantCode  int
		wantTexts []string
	}{
		{"player reads the party", permissions.RolePlayer, "party", false, nil, 0, []string{"Saved the granary"}},
		{"player cannot read an unset subject", permissions.RolePlayer, stTobin, false, nil, http.StatusNotFound, nil},
		{"player: switch off reads as not found", permissions.RolePlayer, "party", true, nil, http.StatusNotFound, nil},
		{"player cannot read a hidden character", permissions.RolePlayer, stMira, false, map[string]bool{stMira: true}, http.StatusNotFound, nil},
		{"player: a subject that is not a key", permissions.RolePlayer, "x' OR '1'='1", false, nil, http.StatusBadRequest, nil},
		{"director reads the party with DM-only", permissions.RoleOwner, "party", true, nil, 0, []string{secret, "Saved the granary"}},
		{"director reads an unset character", permissions.RoleOwner, stTobin, false, nil, 0, nil},
		{"director: a page that is not in the campaign", permissions.RoleOwner, uuid.NewString(), false, nil, http.StatusNotFound, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			if tt.switchOff {
				repo.playersSee[stPage] = false
			}
			svc := NewStandingService(repo, fakeGate{hidden: tt.hide})
			v, err := svc.Subject(context.Background(), stCamp, stPage, tt.subject, tt.role, "u1")
			if tt.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := textsOf(*v); strings.Join(got, "|") != strings.Join(tt.wantTexts, "|") {
				t.Errorf("reasons = %v, want %v", got, tt.wantTexts)
			}
		})
	}
}

func TestStandingAddReason(t *testing.T) {
	ten := 4
	bad := 9
	tests := []struct {
		name     string
		in       AddStandingReasonInput
		full     bool
		wantCode int
		wantMsg  string
		want     int // level afterwards
	}{
		{name: "a player is refused", in: AddStandingReasonInput{Role: permissions.RolePlayer, Subject: "party", Direction: "up", Text: "x"}, wantCode: http.StatusForbidden},
		{name: "a scribe is refused", in: AddStandingReasonInput{Role: permissions.RoleScribe, Subject: "party", Direction: "up", Text: "x"}, wantCode: http.StatusForbidden},
		{name: "raises the word the Director picks", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: "Paid the debt", Level: &ten}, want: 4},
		{name: "no word given keeps the current one", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "down", Text: "Brawled"}, want: 1},
		{name: "a new subject starts Neutral", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: stTobin, Direction: "none", Text: "Met at the Gate"}, want: 2},
		{name: "empty text", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: "   "}, wantCode: http.StatusUnprocessableEntity, wantMsg: "Say what happened first."},
		{name: "text over 90", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: strings.Repeat("a", 91)}, wantCode: http.StatusUnprocessableEntity, wantMsg: "90 characters"},
		{name: "text of exactly 90", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: strings.Repeat("é", 90)}, want: 1},
		{name: "control character", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: "a\x00b"}, wantCode: http.StatusUnprocessableEntity},
		{name: "when over 30", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: "ok", When: strings.Repeat("w", 31)}, wantCode: http.StatusUnprocessableEntity, wantMsg: "30 characters"},
		{name: "unknown direction", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "sideways", Text: "ok"}, wantCode: http.StatusUnprocessableEntity},
		{name: "word out of range", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: "ok", Level: &bad}, wantCode: http.StatusUnprocessableEntity},
		{name: "subject that is not a key", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "nope", Direction: "up", Text: "ok"}, wantCode: http.StatusBadRequest},
		{name: "subject page not in this campaign", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: uuid.NewString(), Direction: "up", Text: "ok"}, wantCode: http.StatusNotFound},
		{name: "a full standing asks to remove some", in: AddStandingReasonInput{Role: permissions.RoleOwner, Subject: "party", Direction: "up", Text: "ok"}, full: true, wantCode: http.StatusUnprocessableEntity, wantMsg: "300"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			repo.full = tt.full
			svc := NewStandingService(repo, fakeGate{})
			tt.in.CampaignID, tt.in.EntityID, tt.in.UserID = stCamp, stPage, "u1"
			v, err := svc.AddReason(context.Background(), tt.in)
			if tt.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
				if tt.wantMsg != "" && !strings.Contains(ae.Message, tt.wantMsg) {
					t.Errorf("message = %q, want it to contain %q", ae.Message, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v.Level != tt.want {
				t.Errorf("level = %d, want %d", v.Level, tt.want)
			}
			if !v.Set || len(v.Reasons) == 0 || v.Reasons[0].Text == "" {
				t.Errorf("the new reason is not first in %+v", v)
			}
		})
	}
}

func TestStandingAddReason_DMOnlyFlagIsKeptAndTextIsCleaned(t *testing.T) {
	repo := seededStanding()
	svc := NewStandingService(repo, fakeGate{})
	v, err := svc.AddReason(context.Background(), AddStandingReasonInput{
		CampaignID: stCamp, EntityID: stPage, Subject: "party", Direction: "down", Text: "  Sold   a map  ", When: " session 13 ", DMOnly: true,
		Role: permissions.RoleOwner, UserID: "u1",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := v.Reasons[0]
	if r.Text != "Sold a map" || r.When != "session 13" || !r.DMOnly {
		t.Errorf("reason = %+v", r)
	}
	// The same reason is invisible to a player.
	pv, _ := svc.Subject(context.Background(), stCamp, stPage, "party", permissions.RolePlayer, "u2")
	for _, x := range pv.Reasons {
		if x.Text == "Sold a map" {
			t.Error("a DM-only reason reached a player")
		}
	}
}

func TestStandingRemoveReason(t *testing.T) {
	tests := []struct {
		name     string
		role     int
		subject  string
		id       string
		wantCode int
	}{
		{"player is refused", permissions.RolePlayer, "party", "r1", http.StatusForbidden},
		{"director removes one", permissions.RoleOwner, "party", uuidFor("r1"), 0},
		{"a reason of another subject is not found", permissions.RoleOwner, stMira, uuidFor("r1"), http.StatusNotFound},
		{"an id that is not an id", permissions.RoleOwner, "party", "x", http.StatusNotFound},
		{"an unknown subject", permissions.RoleOwner, "zzz", uuidFor("r1"), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			repo.reasons[0].ID = uuidFor("r1")
			svc := NewStandingService(repo, fakeGate{})
			v, err := svc.RemoveReason(context.Background(), stCamp, stPage, tt.subject, tt.id, tt.role, "u1")
			if tt.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range v.Reasons {
				if r.ID == uuidFor("r1") {
					t.Error("the reason is still there")
				}
			}
		})
	}
}

// uuidFor makes a stable valid id from a label, since reason ids are checked.
func uuidFor(label string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(label)).String()
}

func TestStandingSetPlayersSee(t *testing.T) {
	tests := []struct {
		name     string
		role     int
		wantCode int
	}{
		{"player is refused", permissions.RolePlayer, http.StatusForbidden},
		{"scribe is refused", permissions.RoleScribe, http.StatusForbidden},
		{"director may", permissions.RoleOwner, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seededStanding()
			svc := NewStandingService(repo, fakeGate{})
			err := svc.SetPlayersSee(context.Background(), stCamp, stPage, tt.role, "u1", false)
			if tt.wantCode != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
				if v, _ := repo.PlayersSee(context.Background(), stPage); !v {
					t.Error("a refused call still changed the switch")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := repo.PlayersSee(context.Background(), stPage); v {
				t.Error("the switch did not turn off")
			}
		})
	}
}

func TestStandingWhyText(t *testing.T) {
	tests := []struct {
		total  int
		newest string
		want   string
	}{
		{0, "", "Why: no reasons shared yet"},
		{1, "Saved the granary", "Why: Saved the granary"},
		{1, "Saved the granary from the fire", "Why: Saved the granary from…"},
		{5, "Saved the granary from the fire", "Why: Saved the granary from… and 4 more"},
		{2, "Brawled", "Why: Brawled and 1 more"},
		{8, "Brawled", "Why: Brawled and 7 more"},
		{9, "Brawled", "Why: 9 reasons"},
		{120, "x", "Why: 120 reasons"},
	}
	for _, tt := range tests {
		if got := StandingWhyText(tt.total, tt.newest); got != tt.want {
			t.Errorf("StandingWhyText(%d, %q) = %q, want %q", tt.total, tt.newest, got, tt.want)
		}
	}
}

func TestCleanStandingText(t *testing.T) {
	tests := []struct {
		in     string
		max    int
		want   string
		wantOK bool
	}{
		{"  hello   world ", 90, "hello world", true},
		{"", 90, "", true},
		{"a\tb\nc", 90, "a b c", true},
		{"a\x00b", 90, "", false},
		{"a\x1bb", 90, "", false},
		{strings.Repeat("é", 5), 5, strings.Repeat("é", 5), true},
		{strings.Repeat("é", 6), 5, strings.Repeat("é", 6), false},
	}
	for _, tt := range tests {
		got, ok := cleanStandingText(tt.in, tt.max)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("cleanStandingText(%q, %d) = %q, %v; want %q, %v", tt.in, tt.max, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestStandingWord(t *testing.T) {
	for i, w := range []string{"Hostile", "Wary", "Neutral", "Warm", "Trusted"} {
		if StandingWord(i) != w {
			t.Errorf("StandingWord(%d) = %q, want %q", i, StandingWord(i), w)
		}
	}
	if StandingWord(-1) != "Neutral" || StandingWord(9) != "Neutral" {
		t.Error("an out-of-range level should read as Neutral")
	}
}

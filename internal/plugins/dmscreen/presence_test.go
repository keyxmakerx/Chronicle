package dmscreen

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakePresence struct {
	players []PlayerPresence
	err     error
}

func (f fakePresence) Players(context.Context, string) ([]PlayerPresence, error) {
	return f.players, f.err
}

func TestBuildPresence(t *testing.T) {
	tests := []struct {
		name    string
		players []PlayerPresence
		want    *PresenceView
		label   string
		title   string
	}{
		{"no players omits the section", nil, nil, "", ""},
		{
			"three of four",
			[]PlayerPresence{{"u1", "Bren", true}, {"u2", "Aria", true}, {"u3", "Cass", false}, {"u4", "Dov", true}},
			&PresenceView{Here: 3, Total: 4, HereNames: []string{"Aria", "Bren", "Dov"}, AwayNames: []string{"Cass"}},
			"3 of 4 players here", "Here: Aria, Bren, Dov. Not here: Cass.",
		},
		{
			"nobody here",
			[]PlayerPresence{{"u1", "Bren", false}},
			&PresenceView{Here: 0, Total: 1, AwayNames: []string{"Bren"}},
			"0 of 1 player here", "Here: nobody. Not here: Bren.",
		},
		{
			"unnamed player still counted",
			[]PlayerPresence{{"u1", "", true}},
			&PresenceView{Here: 1, Total: 1, HereNames: []string{"A player"}},
			"1 of 1 player here", "Here: A player. Not here: nobody.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPresence(tt.players)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			if got != nil && (got.Label() != tt.label || got.Title() != tt.title) {
				t.Errorf("label %q title %q", got.Label(), got.Title())
			}
		})
	}
}

func TestBuild_PresenceAndHeroDot(t *testing.T) {
	src := Sources{
		Presence: fakePresence{players: []PlayerPresence{{"u1", "Bren", true}, {"u2", "Aria", false}}},
		Party: fakeParty{heroes: []Hero{
			{ID: "h1", Name: "Thorin", PlayerUserID: "u1"},
			{ID: "h2", Name: "Mira", PlayerUserID: "u2"},
			{ID: "h3", Name: "Orphan"},
		}},
	}
	v, err := NewService(src).Build(context.Background(), "c1", Viewer{Role: 3})
	if err != nil {
		t.Fatal(err)
	}
	if v.Presence == nil || v.Presence.Here != 1 || v.Presence.Total != 2 {
		t.Fatalf("presence = %+v", v.Presence)
	}
	got := map[string]bool{}
	for _, h := range v.Party {
		got[h.ID] = h.PlayerHere
	}
	if !got["h1"] || got["h2"] || got["h3"] {
		t.Errorf("hero dots = %v", got)
	}

	// A failing source leaves the section out and never fails the screen.
	v, err = NewService(Sources{Presence: fakePresence{err: errors.New("db down")}}).Build(context.Background(), "c1", Viewer{Role: 3})
	if err != nil || v.Presence != nil {
		t.Fatalf("err %v presence %+v", err, v.Presence)
	}
}

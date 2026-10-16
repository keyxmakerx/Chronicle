package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// A listed page is Director-only when the same list read as a player leaves
// it out; pages a player also gets stay unmarked.
func TestPlayerHiddenIDs(t *testing.T) {
	cases := []struct {
		name            string
		shown, asPlayer []string
		want            []string
	}{
		{"all shared", []string{"a", "b"}, []string{"a", "b"}, nil},
		{"one hidden", []string{"a", "b", "c"}, []string{"a", "c"}, []string{"b"}},
		{"all hidden", []string{"a", "b"}, nil, []string{"a", "b"}},
		{"player list has extras", []string{"a"}, []string{"a", "z"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := playerHiddenIDs(tc.shown, tc.asPlayer)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for _, id := range tc.want {
				if !got[id] {
					t.Errorf("%s not marked hidden (%v)", id, got)
				}
			}
		})
	}
}

// A count carries the players' number beside the full one only when they
// differ, so player view can show the right one.
func TestBlockCount(t *testing.T) {
	cases := []struct {
		name              string
		total, forPlayers int
		split             bool
	}{
		{"same", 3, 3, false},
		{"fewer for players", 5, 2, true},
		{"none for players", 2, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := blockCount(tc.total, tc.forPlayers).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if got := strings.Contains(out, "data-player-only"); got != tc.split {
				t.Errorf("split = %v, want %v: %s", got, tc.split, out)
			}
		})
	}
}

// Sub-page rows a player can't see carry the Director-only mark.
func TestHiddenFromPlayersContext(t *testing.T) {
	ctx := withPlayerHidden(context.Background(), map[string]bool{"b": true})
	if hiddenFromPlayers(ctx, "a") || !hiddenFromPlayers(ctx, "b") {
		t.Error("hiddenFromPlayers does not read the ids the handler set")
	}
	if hiddenFromPlayers(context.Background(), "b") {
		t.Error("no ids set must mark nothing")
	}
	n := childrenHiddenFromPlayers(ctx, []Entity{{ID: "a"}, {ID: "b"}})
	if n != 1 {
		t.Errorf("childrenHiddenFromPlayers = %d, want 1", n)
	}
}

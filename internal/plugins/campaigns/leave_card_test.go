package campaigns

import (
	"context"
	"strings"
	"testing"
	"time"
)

// My campaigns draws Hand over on a campaign the person owns and Leave on the
// rest, and the Leave box says by name what stays and what is put away.
func TestCampaignListMenus(t *testing.T) {
	list := []Campaign{
		{ID: "c-own", Name: "The Ember Coast", UpdatedAt: time.Now()},
		{ID: "c-play", Name: "Therin", UpdatedAt: time.Now()},
	}
	view := LeaveView{
		Owned:      map[string]bool{"c-own": true},
		Characters: map[string][]string{"c-play": {"Brannoc"}},
	}
	var sb strings.Builder
	if err := CampaignListContent(list, 2, DefaultListOptions(), "tok", view).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	own := between(t, html, `data-campaign-card="c-own"`, `data-campaign-card="c-play"`)
	if !strings.Contains(own, "Hand over…") || !strings.Contains(own, "/campaigns/c-own/transfer") {
		t.Errorf("the owner's card has no Hand over to the transfer page:\n%s", own)
	}
	if strings.Contains(own, "Leave campaign") {
		t.Error("the owner's card offers Leave")
	}

	play := html[strings.Index(html, `data-campaign-card="c-play"`):]
	if !strings.Contains(play, "Leave campaign…") || !strings.Contains(play, `data-box="leave-box-c-play"`) {
		t.Errorf("the player's card has no Leave entry:\n%s", play)
	}
	if strings.Contains(play[:strings.Index(play, "leave-box-c-play\"")+1], "Hand over") {
		t.Error("the player's card offers Hand over")
	}
	if !strings.Contains(play, "Open") {
		t.Error("the menu has no Open")
	}

	for _, want := range []string{
		"Leave Therin?",
		"Brannoc stays with the campaign. The DM can give them to someone else.",
		"Your posts and shared notes stay, with your name on them.",
		"Your private notes and journal are put away. They come back if you rejoin.",
		"You can come back with an invite or the campaign&#39;s join link.",
		">Stay<",
		"Leave Therin</button>",
		`data-leave-url="/campaigns/c-play/leave"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the Leave box lacks %q", want)
		}
	}
	// An owner has no box at all, and a player without a character gets no
	// character line.
	if strings.Contains(html, "leave-box-c-own") {
		t.Error("an owned campaign has a Leave box")
	}
	view.Characters = nil
	sb.Reset()
	if err := CampaignListContent(list, 2, DefaultListOptions(), "tok", view).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sb.String(), "stays with the campaign") {
		t.Error("the box names a character the person does not have")
	}
}

func between(t *testing.T, s, from, to string) string {
	t.Helper()
	i := strings.Index(s, from)
	j := strings.Index(s, to)
	if i < 0 || j < i {
		t.Fatalf("markers %q .. %q not found in order", from, to)
	}
	return s[i:j]
}

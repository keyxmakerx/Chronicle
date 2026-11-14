package sessions

import (
	"context"
	"testing"
	"time"
)

// The recap's posted time is stamped when a recap first becomes non-empty and
// kept after that, so renaming a night or editing its recap never re-announces
// it; the since reader returns only recaps a player can read, nights set and
// polls opened after the cut-off, and nothing from another campaign.
func TestSinceReader_Integration(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	ctx := context.Background()
	camp, user := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)

	posted := func(id string) (t0 *time.Time) {
		var at *time.Time
		if err := db.QueryRow(`SELECT recap_posted_at FROM sessions WHERE id = ?`, id).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	str := func(s string) *string { return &s }

	night := seedSession(t, db, camp, user, "The Drowned Archive")
	if posted(night) != nil {
		t.Fatal("a night with no recap has no posted time")
	}
	if err := repo.UpdateRecap(ctx, night, nil, str("<p>We went down.</p>")); err != nil {
		t.Fatal(err)
	}
	first := posted(night)
	if first == nil {
		t.Fatal("writing a recap stamps its posted time")
	}
	time.Sleep(1100 * time.Millisecond)
	if err := repo.UpdateRecap(ctx, night, nil, str("<p>We went down, and up.</p>")); err != nil {
		t.Fatal(err)
	}
	if again := posted(night); again == nil || !again.Equal(*first) {
		t.Errorf("editing a written recap moved its posted time: %v -> %v", first, again)
	}
	s, err := repo.FindByID(ctx, night)
	if err != nil {
		t.Fatal(err)
	}
	s.Name = "Renamed"
	if err := repo.Update(ctx, s); err != nil {
		t.Fatal(err)
	}
	if again := posted(night); again == nil || !again.Equal(*first) {
		t.Errorf("renaming a night moved its posted time: %v -> %v", first, again)
	}

	// A night first written through the full update is stamped too.
	viaUpdate := seedSession(t, db, camp, user, "Via Update")
	s2, _ := repo.FindByID(ctx, viaUpdate)
	s2.RecapHTML = str("<p>Hello</p>")
	if err := repo.Update(ctx, s2); err != nil {
		t.Fatal(err)
	}
	if posted(viaUpdate) == nil {
		t.Error("a recap written through Update is stamped")
	}

	// A recap that is only GM-only text posts a time but a player cannot read it.
	secretOnly := seedSession(t, db, camp, user, "Secret Night")
	if err := repo.UpdateRecap(ctx, secretOnly, nil, str(`<p><span data-secret="true">The duke did it.</span></p>`)); err != nil {
		t.Fatal(err)
	}
	foreign := seedSession(t, db, otherCamp, otherUser, "Foreign Night")
	if err := repo.UpdateRecap(ctx, foreign, nil, str("<p>Elsewhere.</p>")); err != nil {
		t.Fatal(err)
	}

	r := NewSinceReader(db)
	cutoff := time.Now().UTC().Add(-time.Hour)
	names := func(items []SinceItem) map[string]bool {
		m := map[string]bool{}
		for _, it := range items {
			m[it.Name] = true
		}
		return m
	}
	player, err := r.RecapsSince(ctx, camp, cutoff, SinceViewer{})
	if err != nil {
		t.Fatal(err)
	}
	if n := names(player); !n["Renamed"] || !n["Via Update"] || n["Secret Night"] || n["Foreign Night"] || len(n) != 2 {
		t.Errorf("player recaps = %v", n)
	}
	dm, _ := r.RecapsSince(ctx, camp, cutoff, SinceViewer{DMTeam: true})
	if n := names(dm); !n["Secret Night"] || len(n) != 3 {
		t.Errorf("DM recaps = %v", n)
	}
	if later, _ := r.RecapsSince(ctx, camp, time.Now().UTC().Add(time.Hour), SinceViewer{}); len(later) != 0 {
		t.Errorf("nothing was posted after the future: %v", later)
	}

	// Nights set: planned, dated today or later, created after the cut-off.
	future := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	past := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
	setNight := seedSession(t, db, camp, user, "Next Thursday")
	pastNight := seedSession(t, db, camp, user, "Long Ago")
	for id, d := range map[string]string{setNight: future, pastNight: past} {
		if _, err := db.Exec(`UPDATE sessions SET scheduled_date = ? WHERE id = ?`, d, id); err != nil {
			t.Fatal(err)
		}
	}
	today := time.Now().UTC().Format("2006-01-02")
	got, err := r.NightsSince(ctx, camp, cutoff, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Next Thursday" || got[0].Date != future {
		t.Errorf("nights = %+v", got)
	}

	// Polls: open ones created after the cut-off, this campaign only.
	mkPoll := func(c, u, title, status string) {
		if _, err := db.Exec(`INSERT INTO slot_proposals (id, campaign_id, created_by, title, status, created_at, updated_at) VALUES (?,?,?,?,?,NOW(),NOW())`,
			newDBID(t), c, u, title, status); err != nil {
			t.Fatal(err)
		}
	}
	mkPoll(camp, user, "Pick a Thursday", ProposalOpen)
	mkPoll(camp, user, "Settled poll", ProposalClosed)
	mkPoll(otherCamp, otherUser, "Foreign poll", ProposalOpen)
	polls, err := r.PollsSince(ctx, camp, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(polls) != 1 || polls[0].Name != "Pick a Thursday" {
		t.Errorf("polls = %+v", polls)
	}
}

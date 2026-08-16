package sessions

import (
	"context"
	"testing"
	"time"
)

// C-RSVP-P9 against a REAL MariaDB.
//
// Everything here is a claim a mock cannot check: that migration 005's DDL is
// syntax MariaDB actually accepts (ADD UNIQUE KEY IF NOT EXISTS / DROP INDEX IF
// EXISTS are MariaDB-only spellings), that the re-keyed unique constraint really
// does let one slot exist on both alternating tracks, and that an EMPTY save
// still writes the answered stamp — which is the entire point of the status
// table and is invisible to any test that only inspects returned structs.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/sessions/ -run TestDB

// The migration must actually apply, and must leave week_parity defaulting to
// "every week" so pre-existing rows keep their meaning.
func TestDB_CadenceColumnDefaultsToEveryWeek(t *testing.T) {
	db := newScratchDB(t)
	campID, userID := seedCampaign(t, db)

	// Insert WITHOUT naming week_parity — the shape a pre-C-RSVP-P9 writer had.
	if _, err := db.Exec(`INSERT INTO member_availability
		(id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, updated_at)
		VALUES (?,?,?,?,?,?,?,?,NOW())`,
		newDBID(t), campID, userID, 1, 1080, 1320, AvailAvailable, "UTC"); err != nil {
		t.Fatalf("legacy-shaped insert failed — migration 005 broke the old write path: %v", err)
	}

	var parity int
	if err := db.QueryRow(`SELECT week_parity FROM member_availability WHERE campaign_id = ?`, campID).
		Scan(&parity); err != nil {
		t.Fatalf("reading week_parity: %v", err)
	}
	if parity != CadenceEveryWeek {
		t.Fatalf("week_parity defaulted to %d, want %d (every week) — every block written before this "+
			"migration would silently become fortnightly", parity, CadenceEveryWeek)
	}
}

// The re-keyed unique constraint is the reason the migration drops and re-adds
// an index rather than only adding a column. Without it these two rows collide.
func TestDB_SameSlotOnBothTracksIsStorable(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	campID, userID := seedCampaign(t, db)

	blocks := []AvailabilityBlock{
		{DayOfWeek: 1, StartMinute: 1080, EndMinute: 1320, State: AvailAvailable, TZ: "UTC", WeekCadence: CadenceWeekA},
		{DayOfWeek: 1, StartMinute: 1080, EndMinute: 1320, State: AvailPreferred, TZ: "UTC", WeekCadence: CadenceWeekB},
	}
	if err := repo.ReplaceUserAvailability(context.Background(), campID, userID, "UTC", blocks); err != nil {
		t.Fatalf("storing the same slot on both tracks failed — the old unique key is still in force: %v", err)
	}

	got, err := repo.ListUserAvailability(context.Background(), campID, userID)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read back %d blocks, want 2", len(got))
	}
	seen := map[int]string{}
	for _, b := range got {
		seen[b.WeekCadence] = b.State
	}
	if seen[CadenceWeekA] != AvailAvailable || seen[CadenceWeekB] != AvailPreferred {
		t.Fatalf("cadence did not round-trip: %+v", seen)
	}
}

// Saving an EMPTY grid must still stamp the answer. This is the case the
// derived-from-row-count shortcut gets wrong, and the reason the status table
// exists at all.
func TestDB_EmptySaveStillRecordsTheAnswer(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	campID, userID := seedCampaign(t, db)
	ctx := context.Background()

	before, err := repo.ListAnsweredUserIDs(ctx, campID)
	if err != nil {
		t.Fatalf("listing answers: %v", err)
	}
	if _, ok := before[userID]; ok {
		t.Fatal("a member who has never saved is already marked as answered")
	}

	if err := repo.ReplaceUserAvailability(ctx, campID, userID, "Europe/London", nil); err != nil {
		t.Fatalf("empty save: %v", err)
	}

	after, err := repo.ListAnsweredUserIDs(ctx, campID)
	if err != nil {
		t.Fatalf("listing answers: %v", err)
	}
	at, ok := after[userID]
	if !ok {
		t.Fatal("saving an empty grid did not record an answer — 'I am never free' stays " +
			"indistinguishable from never having opened the page, which is the whole defect")
	}
	if at.IsZero() {
		t.Fatal("answered_at is zero")
	}

	// And no blocks were written, so the answer really is "never free".
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM member_availability WHERE campaign_id = ?`, campID).Scan(&n); err != nil {
		t.Fatalf("counting blocks: %v", err)
	}
	if n != 0 {
		t.Fatalf("an empty save wrote %d blocks", n)
	}
}

// Re-saving must UPDATE the stamp, not duplicate it — the table is keyed
// (campaign, user) and an INSERT without the ON DUPLICATE clause would error on
// the second save, taking the whole availability write down with it.
func TestDB_ResavingUpdatesTheAnswerStampInPlace(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	campID, userID := seedCampaign(t, db)
	ctx := context.Background()

	if err := repo.ReplaceUserAvailability(ctx, campID, userID, "UTC", nil); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := repo.ReplaceUserAvailability(ctx, campID, userID, "Asia/Tokyo", []AvailabilityBlock{
		{DayOfWeek: 3, StartMinute: 600, EndMinute: 660, State: AvailAvailable, TZ: "Asia/Tokyo"},
	}); err != nil {
		t.Fatalf("second save failed — the stamp is INSERT-only: %v", err)
	}

	var rows int
	var tz string
	if err := db.QueryRow(`SELECT COUNT(*) FROM member_availability_status WHERE campaign_id = ?`, campID).
		Scan(&rows); err != nil {
		t.Fatalf("counting stamps: %v", err)
	}
	if rows != 1 {
		t.Fatalf("got %d stamp rows for one member, want 1", rows)
	}
	if err := db.QueryRow(`SELECT tz FROM member_availability_status WHERE campaign_id = ? AND user_id = ?`,
		campID, userID).Scan(&tz); err != nil {
		t.Fatalf("reading stamp zone: %v", err)
	}
	if tz != "Asia/Tokyo" {
		t.Fatalf("stamp zone is %q, want Asia/Tokyo — the update did not overwrite", tz)
	}
}

// Answers must not bleed between campaigns: answering in one table's campaign
// must not mark you answered in another.
func TestDB_AnswersAreScopedToTheirCampaign(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	campA, userID := seedCampaign(t, db)
	campB, _ := seedCampaign(t, db)
	ctx := context.Background()

	if err := repo.ReplaceUserAvailability(ctx, campA, userID, "UTC", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	other, err := repo.ListAnsweredUserIDs(ctx, campB)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if _, ok := other[userID]; ok {
		t.Fatal("answering in one campaign marked the member answered in another")
	}
}

// The FK must cascade: deleting a campaign has to take its answer stamps with
// it, or a deleted campaign leaves rows that outlive their parent.
func TestDB_AnswerStampsCascadeWithTheCampaign(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	campID, userID := seedCampaign(t, db)
	ctx := context.Background()

	if err := repo.ReplaceUserAvailability(ctx, campID, userID, "UTC", nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, campID); err != nil {
		t.Fatalf("deleting campaign: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM member_availability_status WHERE campaign_id = ?`, campID).
		Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 0 {
		t.Fatalf("%d answer stamps survived their campaign", n)
	}
}

// End to end through the service: a fortnightly block stored via the real store
// must project onto alternate weeks only when read back through BuildOverlay.
func TestDB_FortnightlyBlockProjectsOnAlternateWeeksOnly(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	campID, userID := seedCampaign(t, db)
	ctx := context.Background()

	// Monday 2026-08-17 and the track that week falls in.
	onWeek := cd(2026, time.August, 17)
	track := WeekCadenceFor(onWeek)

	if err := svc.SaveMyAvailability(ctx, campID, userID, SaveAvailabilityRequest{
		TZ: "UTC",
		Blocks: []AvailabilityBlockDTO{{
			DayOfWeek: int(time.Monday), StartMinute: 18 * 60, EndMinute: 22 * 60,
			State: AvailAvailable, WeekCadence: track,
		}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	roster := []overlayMemberInput{{UserID: userID, Name: "Ana"}}

	freeHours := func(weekStart string) int {
		ov, err := svc.BuildOverlay(ctx, campID, roster, weekStart, "UTC", true)
		if err != nil {
			t.Fatalf("overlay for %s: %v", weekStart, err)
		}
		n := 0
		for _, d := range ov.Days {
			for _, h := range d.Hours {
				n += h.Free
			}
		}
		return n
	}

	on := freeHours(onWeek.String())
	off := freeHours(onWeek.AddDays(7).String())
	back := freeHours(onWeek.AddDays(14).String())

	if on == 0 {
		t.Fatal("the fortnightly block did not project on its own week")
	}
	if off != 0 {
		t.Fatalf("the block projected %d free hours on the OFF week — it is behaving as weekly", off)
	}
	if back != on {
		t.Fatalf("two weeks later shows %d free hours, want %d", back, on)
	}
}

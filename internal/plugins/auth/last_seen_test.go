package auth

import (
	"context"
	"testing"
)

// noteSeen writes the last-seen time once a day per person, however many
// requests they make, and again once the day's marker expires.
func TestNoteSeenWritesOncePerDay(t *testing.T) {
	repo := &mockUserRepo{}
	svc, mr := newTestAuthServiceWithRedis(t, repo)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		svc.noteSeen(ctx, "u1")
	}
	svc.noteSeen(ctx, "u2")
	if len(repo.seen) != 2 || repo.seen[0] != "u1" || repo.seen[1] != "u2" {
		t.Fatalf("seen = %v, want [u1 u2]", repo.seen)
	}

	mr.FastForward(lastSeenEvery)
	svc.noteSeen(ctx, "u1")
	if len(repo.seen) != 3 {
		t.Fatalf("after a day seen = %v, want a second write for u1", repo.seen)
	}
}

package systems

import (
	"context"
	"testing"
	"time"
)

// TestChoiceRuleRepository_Integration runs the hand-written SQL against a
// real MariaDB: one row per (entry, target) with ” for everyone, the viewer
// filter, atomic batches, deleting an entry's rules while withdrawing its
// waiting requests, one waiting request per hero and field, and the cascade on
// campaign delete. Skipped under -short or when no server answers.
func TestChoiceRuleRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openBookTestDB(t)
	ctx := context.Background()
	repo := NewChoiceRuleRepository(db)

	userID, campA, campB := bookTestUUID(t), bookTestUUID(t), bookTestUUID(t)
	mustBookExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "rule-"+userID+"@example.test", "Rule Test", "x")
	for _, c := range []string{campA, campB} {
		mustBookExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			c, "Rule "+c, "rule-"+c, userID)
	}
	rule := func(camp, key, target, state string) ChoiceRule {
		return ChoiceRule{CampaignID: camp, SystemID: "sys", FieldKey: "ancestry", Source: "package", Key: key,
			Name: key, Target: target, State: state, Reason: "r", CreatedBy: userID}
	}

	if err := repo.Apply(ctx, []ChoiceRule{
		rule(campA, "fury", "", RuleNotOffered), rule(campA, "fury", "pat", RuleOpen),
		rule(campA, "*", "lee", RuleNotOffered), rule(campB, "fury", "", RuleRequired),
	}, nil); err != nil {
		t.Fatal(err)
	}
	// The same (entry, target) is replaced, not duplicated: the unique key
	// holds for everyone because that target is '', not NULL.
	if err := repo.Apply(ctx, []ChoiceRule{rule(campA, "fury", "", RuleAskFirst)}, nil); err != nil {
		t.Fatal(err)
	}
	all, err := repo.List(ctx, campA, "sys", "ancestry")
	if err != nil || len(all) != 3 {
		t.Fatalf("list = %d, %v; want 3 and no cross-campaign rows", len(all), err)
	}
	pat, err := repo.ListForViewer(ctx, campA, "sys", "ancestry", "pat")
	if err != nil || len(pat) != 2 {
		t.Fatalf("Pat's rules = %d, %v; want the everyone row and Pat's own, not Lee's", len(pat), err)
	}
	for _, r := range pat {
		if r.Key == "fury" && r.Target == "" && r.State != RuleAskFirst {
			t.Fatalf("the replaced row kept state %s", r.State)
		}
	}
	if anon, _ := repo.ListForViewer(ctx, campA, "sys", "", ""); len(anon) != 1 {
		t.Fatalf("a viewer with no account gets %d rules, want only the everyone row", len(anon))
	}
	if n, _ := repo.CountForEntry(ctx, campA, "sys", "ancestry", "package", "fury"); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}

	// A batch is atomic: a failing statement rolls the whole batch back.
	bad := rule(campA, "devil", "", RuleNotOffered)
	bad.State = "nonsense"
	if err := repo.Apply(ctx, []ChoiceRule{rule(campA, "human", "", RuleNotOffered), bad}, nil); err == nil {
		t.Fatal("an invalid state was accepted")
	}
	if all, _ = repo.List(ctx, campA, "sys", "ancestry"); len(all) != 3 {
		t.Fatalf("a failed batch left %d rules, want the original 3", len(all))
	}
	if err := repo.Apply(ctx, nil, []ChoiceRule{rule(campA, "*", "lee", "")}); err != nil {
		t.Fatal(err)
	}
	if all, _ = repo.List(ctx, campA, "sys", "ancestry"); len(all) != 2 {
		t.Fatalf("after a delete %d rules, want 2", len(all))
	}

	// Requests: one waiting row per player, hero and field.
	req := &ChoiceRequest{CampaignID: campA, SystemID: "sys", FieldKey: "ancestry", Source: "package", Key: "fury",
		Name: "Fury", UserID: userID, EntityID: "hero1", EntityName: "Tess"}
	if err := repo.UpsertWaiting(ctx, req); err != nil || req.ID == 0 {
		t.Fatalf("request = %+v, %v", req, err)
	}
	again := *req
	again.ID, again.Key, again.Name = 0, "devil", "Devil"
	if err := repo.UpsertWaiting(ctx, &again); err != nil || again.ID != req.ID {
		t.Fatalf("a second pick for the same hero and field = id %d (%v), want to replace %d", again.ID, err, req.ID)
	}
	if w, _ := repo.ListRequests(ctx, campA, "sys", RequestWaiting); len(w) != 1 || w[0].Name != "Devil" || w[0].EntityName != "Tess" {
		t.Fatalf("waiting = %+v", w)
	}
	if w, _ := repo.HeroWaiting(ctx, campA, userID, "hero1"); len(w) != 1 {
		t.Fatalf("hero waiting = %+v", w)
	}
	if w, _ := repo.ListRequests(ctx, campB, "sys", RequestWaiting); len(w) != 0 {
		t.Fatal("requests crossed campaigns")
	}
	if ok, err := repo.Close(ctx, campA, req.ID, RequestSuggested, "try another", userID); err != nil || !ok {
		t.Fatalf("close = %v, %v", ok, err)
	}
	if ok, _ := repo.Close(ctx, campA, req.ID, RequestAllowed, "", userID); ok {
		t.Fatal("a closed request was closed again")
	}
	mine, err := repo.UserRequests(ctx, campA, "sys", userID, "ancestry", time.Now().Add(-time.Hour))
	if err != nil || len(mine) != 1 || mine[0].Status != RequestSuggested || mine[0].Message != "try another" {
		t.Fatalf("user requests = %+v, %v", mine, err)
	}
	if old, _ := repo.UserRequests(ctx, campA, "sys", userID, "ancestry", time.Now().Add(time.Hour)); len(old) != 0 {
		t.Fatal("a suggestion older than the cutoff was returned")
	}

	// Deleting an entry's rules withdraws what waits on it.
	waiting := &ChoiceRequest{CampaignID: campA, SystemID: "sys", FieldKey: "ancestry", Source: "package", Key: "fury",
		Name: "Fury", UserID: userID, EntityID: "hero2"}
	if err := repo.UpsertWaiting(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	n, err := repo.DeleteForEntry(ctx, campA, "sys", "ancestry", "package", "fury")
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, %v; want 2", n, err)
	}
	if got, _ := repo.GetRequest(ctx, campA, waiting.ID); got.Status != RequestWithdrawn {
		t.Fatalf("request is %s, want withdrawn", got.Status)
	}
	if _, err := repo.GetRequest(ctx, campB, waiting.ID); err == nil {
		t.Fatal("get crossed campaigns")
	}
	if left, _ := repo.List(ctx, campB, "sys", "ancestry"); len(left) != 1 {
		t.Fatalf("another campaign's rules = %d, want 1 untouched", len(left))
	}

	// Deleting the campaign takes its rules and requests with it.
	mustBookExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campB)
	if left, _ := repo.List(ctx, campB, "sys", "ancestry"); len(left) != 0 {
		t.Fatal("rules outlived their campaign")
	}
}

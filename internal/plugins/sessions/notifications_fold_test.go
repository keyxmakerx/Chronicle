package sessions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// foldRepo keeps notification rows in memory, with the repository's rules for
// the two fold calls: only an unread row is found, and a rewrite lands only
// while the row is unread and holds the payload the caller built on.
type foldRepo struct {
	*mockSessionRepo
	rows []*Notification
}

func (r *foldRepo) CreateNotification(_ context.Context, n *Notification) error {
	c := *n
	r.rows = append(r.rows, &c)
	return nil
}

func (r *foldRepo) FindUnreadNotification(_ context.Context, userID, ntype, link string) (*Notification, error) {
	for i := len(r.rows) - 1; i >= 0; i-- {
		n := r.rows[i]
		if n.UserID == userID && n.Type == ntype && n.Link != nil && *n.Link == link && n.ReadAt == nil {
			c := *n
			return &c, nil
		}
	}
	return nil, nil
}

func (r *foldRepo) RewriteUnreadNotification(_ context.Context, id string, was, payload *string, at time.Time) (bool, error) {
	for _, n := range r.rows {
		if n.ID == id && n.ReadAt == nil && (n.Payload == nil) == (was == nil) && (was == nil || *n.Payload == *was) {
			n.Payload, n.CreatedAt = payload, at
			return true, nil
		}
	}
	return false, nil
}

func (r *foldRepo) read(userID string) {
	now := time.Now()
	for _, n := range r.rows {
		if n.UserID == userID {
			n.ReadAt = &now
		}
	}
}

func (r *foldRepo) of(userID string) []*Notification {
	var out []*Notification
	for _, n := range r.rows {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	return out
}

// A fold keeps one unread row per recipient and link, hands each recipient
// the state their own row kept, and starts a fresh row once it is read.
func TestFoldUnreadNotification(t *testing.T) {
	repo := &foldRepo{mockSessionRepo: &mockSessionRepo{}}
	svc := &sessionService{repo: repo}
	ctx := context.Background()
	counts := func(kept string) (string, string, string) {
		n := len(kept)
		return "msg", "detail " + kept, kept + "x" + string(rune('0'+n%10))
	}
	fold := func(users []string, link string) {
		t.Helper()
		if err := svc.FoldUnreadNotification(ctx, users, "c1", "hero_changed", link, counts); err != nil {
			t.Fatal(err)
		}
	}
	state := func(n *Notification) string {
		var p notificationPayload
		if err := json.Unmarshal([]byte(*n.Payload), &p); err != nil {
			t.Fatal(err)
		}
		return p.State
	}

	fold([]string{"a", "b", ""}, "/h1")
	fold([]string{"a"}, "/h1")
	fold([]string{"a"}, "/h1")
	if got := len(repo.of("a")); got != 1 {
		t.Fatalf("a has %d rows, want one", got)
	}
	if got := len(repo.of("b")); got != 1 {
		t.Fatalf("b has %d rows, want one", got)
	}
	// a's row has folded three times; b's only once: each keeps their own state.
	if a, b := state(repo.of("a")[0]), state(repo.of("b")[0]); a == b || !strings.HasPrefix(a, b) {
		t.Fatalf("a's state %q should extend b's %q", a, b)
	}
	// Another hero is another row.
	fold([]string{"a"}, "/h2")
	if got := len(repo.of("a")); got != 2 {
		t.Fatalf("one row per link: %d", got)
	}
	// Once read, the next write starts a fresh row from an empty state.
	repo.read("a")
	fold([]string{"a"}, "/h1")
	rows := repo.of("a")
	if len(rows) != 3 || rows[2].ReadAt != nil || state(rows[2]) != "x0" {
		t.Fatalf("a read row must start a fresh one: %d rows, last state %q", len(rows), state(rows[len(rows)-1]))
	}
	if err := svc.FoldUnreadNotification(ctx, []string{"a"}, "c1", "", "/h1", counts); err == nil {
		t.Fatal("a blank type is refused")
	}
	if err := svc.FoldUnreadNotification(ctx, []string{"a"}, "c1", "hero_changed", "", counts); err == nil {
		t.Fatal("a blank link is refused")
	}
}

// The kept state is the writer's own: the bell list gives out the message,
// detail and action and nothing of it.
func TestNotificationState_NeverServed(t *testing.T) {
	p := marshalPayloadState("Ana changed Tess", "hero_changed", "Class: Fury → Shadow", `{"fields":[{"key":"class","from":"Fury"}]}`)
	n := Notification{Type: "hero_changed", Payload: p}
	dto := notifDTO{Message: notificationMessage(n), Detail: notificationDetail(n), Action: notificationAction(n)}
	out, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "fields") || strings.Contains(string(out), "state") {
		t.Fatalf("the bell list served the kept state: %s", out)
	}
	if dto.Message != "Ana changed Tess" || dto.Detail != "Class: Fury → Shadow" {
		t.Fatalf("the row must still read as before: %+v", dto)
	}
}

// The real queries: find only an unread row of the user's, rewrite it only
// while it holds the payload the caller saw, and treat a rewrite that says
// what the row already says as done, so a fold never adds a second row.
func TestDB_FoldUnreadNotification(t *testing.T) {
	db := newScratchDB(t)
	ctx := context.Background()
	repo := NewSessionRepository(db)
	svc := &sessionService{repo: repo}
	campID, me := seedCampaign(t, db)
	other := seedUser(t, db, "Other")
	link := "/campaigns/" + campID + "/entities/e1"

	fold := func(user string, state string) {
		t.Helper()
		err := svc.FoldUnreadNotification(ctx, []string{user}, campID, "hero_changed", link, func(string) (string, string, string) {
			return "Ana changed Tess", "detail", state
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows := func(user string) []Notification {
		t.Helper()
		ns, err := repo.ListNotifications(ctx, user, 50)
		if err != nil {
			t.Fatal(err)
		}
		return ns
	}

	fold(me, "s1")
	fold(me, "s2")
	fold(me, "s2") // the same words again, in the same second
	fold(other, "o1")
	if got := rows(me); len(got) != 1 {
		t.Fatalf("one unread row per hero, got %d", len(got))
	}
	if got := rows(other); len(got) != 1 {
		t.Fatalf("each Director has their own, got %d", len(got))
	}
	var p notificationPayload
	if err := json.Unmarshal([]byte(*rows(me)[0].Payload), &p); err != nil || p.State != "s2" {
		t.Fatalf("payload %v, err %v", p, err)
	}

	cur, err := repo.FindUnreadNotification(ctx, me, "hero_changed", link)
	if err != nil || cur == nil {
		t.Fatalf("find: %v %v", cur, err)
	}
	// A rewrite built on a payload the row no longer holds is refused.
	stale := "{}"
	if ok, err := repo.RewriteUnreadNotification(ctx, cur.ID, &stale, &stale, time.Now().UTC()); err != nil || ok {
		t.Fatalf("a stale rewrite must be refused: ok=%v err=%v", ok, err)
	}
	// Read: it is no longer found, and the next fold starts a fresh row.
	if err := repo.MarkNotificationRead(ctx, me, cur.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.FindUnreadNotification(ctx, me, "hero_changed", link); err != nil || got != nil {
		t.Fatalf("a read row must not be found: %v %v", got, err)
	}
	if ok, err := repo.RewriteUnreadNotification(ctx, cur.ID, cur.Payload, cur.Payload, time.Now().UTC()); err != nil || ok {
		t.Fatalf("a read row must not be rewritten: ok=%v err=%v", ok, err)
	}
	fold(me, "s3")
	if got := rows(me); len(got) != 2 {
		t.Fatalf("a read row starts a fresh one, got %d rows", len(got))
	}
	// Someone else's row is never found for me.
	if got, err := repo.FindUnreadNotification(ctx, seedUser(t, db, "Third"), "hero_changed", link); err != nil || got != nil {
		t.Fatalf("found another user's row: %v %v", got, err)
	}
}

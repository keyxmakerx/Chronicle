package connector

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// memWatch is an in-memory WatchRepository; apps is each key's seen apps.
type memWatch struct {
	mu     sync.Mutex
	server ServerAccess
	camps  map[string]CampaignAccess
	apps   map[int64][]string
	alerts []*Alert
	repo   *memRepo
}

func newMemWatch(repo *memRepo) *memWatch {
	return &memWatch{camps: map[string]CampaignAccess{}, apps: map[int64][]string{}, repo: repo}
}

func (w *memWatch) ServerAccess(context.Context) (ServerAccess, error) { return w.server, nil }
func (w *memWatch) SetServerAccess(_ context.Context, a ServerAccess, _ string, _ time.Time) error {
	w.server = a
	return nil
}
func (w *memWatch) CampaignAccess(_ context.Context, id string) (CampaignAccess, error) {
	return w.camps[id], nil
}
func (w *memWatch) SetCutOff(_ context.Context, id string, at *time.Time, _ time.Time) error {
	a := w.camps[id]
	a.CutOffAt = at
	w.camps[id] = a
	return nil
}
func (w *memWatch) SetPlayersAllowed(_ context.Context, id string, on bool, _ time.Time) error {
	a := w.camps[id]
	a.PlayersAllowed = on
	w.camps[id] = a
	return nil
}
func (w *memWatch) NoteApp(_ context.Context, id int64, app string) (bool, error) {
	for _, a := range w.apps[id] {
		if strings.EqualFold(a, app) {
			return false, nil
		}
	}
	had := len(w.apps[id]) > 0
	w.apps[id] = append(w.apps[id], app)
	return had, nil
}
func (w *memWatch) RaiseAlert(_ context.Context, a *Alert) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, x := range w.alerts {
		if x.Kind == a.Kind && x.SettledAt == nil && x.ConnectionID == a.ConnectionID && (a.ConnectionID != 0 || x.Address == a.Address) {
			x.Hits += a.Hits
			x.UpdatedAt, x.Detail = a.UpdatedAt, a.Detail
			return nil
		}
	}
	cp := *a
	cp.ID = int64(len(w.alerts) + 1)
	w.alerts = append(w.alerts, &cp)
	return nil
}
func (w *memWatch) OpenAlerts(_ context.Context, campaignID string, limit int) ([]Alert, error) {
	var out []Alert
	for i := len(w.alerts) - 1; i >= 0 && len(out) < limit; i-- {
		if a := w.alerts[i]; a.SettledAt == nil && (campaignID == "" || a.CampaignID == campaignID) {
			out = append(out, *a)
		}
	}
	return out, nil
}
func (w *memWatch) RecentAlerts(_ context.Context, since time.Time, limit int) ([]Alert, error) {
	var out []Alert
	for i := len(w.alerts) - 1; i >= 0 && len(out) < limit; i-- {
		if !w.alerts[i].UpdatedAt.Before(since) {
			out = append(out, *w.alerts[i])
		}
	}
	return out, nil
}
func (w *memWatch) SettleAlert(_ context.Context, campaignID string, id int64, how string, at time.Time) (*Alert, error) {
	for _, a := range w.alerts {
		if a.ID == id && a.SettledAt == nil && (campaignID == "" || a.CampaignID == campaignID) {
			t := at
			a.SettledAt, a.SettledHow = &t, how
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}
func (w *memWatch) ConnectionByID(_ context.Context, id int64) (*Connection, error) {
	for _, c := range w.repo.conns {
		if c.ID == id {
			cp := *c
			return &cp, nil
		}
	}
	return nil, nil
}
func (w *memWatch) LiveKeys(_ context.Context, now, _ time.Time) ([]KeyRow, error) {
	var out []KeyRow
	for _, c := range w.repo.conns {
		if c.Live(now) {
			out = append(out, KeyRow{Connection: *c, CampaignName: "Ashfall"})
		}
	}
	return out, nil
}
func (w *memWatch) CallsSince(context.Context, time.Time) (int, error) { return len(w.repo.calls), nil }

func watchedService(t *testing.T) (*Service, *memRepo, *memWatch, *time.Time, string, *Connection) {
	t.Helper()
	svc, repo, _, now := newTestService()
	w := newMemWatch(repo)
	svc.SetWatch(w)
	raw, c, err := svc.Connect(context.Background(), "camp-1", "owner-1", aiexport.PrivacyModeSafe, true)
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, w, now, raw, c
}

// The server's switch and the campaign's brake refuse a good key from its
// next call, and letting AI back on lets the same key in again.
func TestSwitches_RefuseAGoodKeyUntilTurnedBackOn(t *testing.T) {
	ctx := context.Background()
	svc, _, w, _, raw, _ := watchedService(t)
	w.server.AIOff = true
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrAIOff) {
		t.Fatalf("server off: err = %v, want ErrAIOff", err)
	}
	w.server.AIOff = false
	if err := svc.SetCutOff(ctx, "camp-1", "owner-1", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrCutOff) {
		t.Fatalf("campaign stopped: err = %v, want ErrCutOff", err)
	}
	if err := svc.SetCutOff(ctx, "camp-2", "owner-1", true); err == nil {
		t.Error("someone who doesn't own the campaign stopped its AI")
	}
	if err := svc.SetCutOff(ctx, "camp-1", "owner-1", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); err != nil {
		t.Fatalf("back on: %v", err)
	}
}

// A key's first app is remembered quietly; a different one warns the owner,
// once; the same app again warns nothing.
func TestNoteConnect_WarnsOnANewApp(t *testing.T) {
	ctx := context.Background()
	svc, _, w, _, _, c := watchedService(t)
	svc.NoteConnect(ctx, c, "Claude", "203.0.113.9")
	svc.NoteConnect(ctx, c, "claude", "203.0.113.9")
	if len(w.alerts) != 0 {
		t.Fatalf("warned about the key's own app: %+v", w.alerts[0])
	}
	svc.NoteConnect(ctx, c, "python-httpx", "198.51.100.4")
	svc.NoteConnect(ctx, c, "python-httpx", "198.51.100.4")
	open, _ := svc.Alerts(ctx, "camp-1")
	if len(open) != 1 || open[0].Kind != AlertNewApp || !strings.Contains(open[0].Detail, "python-httpx") || open[0].Address != "198.51.100.4" {
		t.Fatalf("alerts = %+v, want one new-app warning", open)
	}
	if other, _ := svc.Alerts(ctx, "camp-2"); len(other) != 0 {
		t.Errorf("another campaign sees the warning: %+v", other)
	}
}

// Limit refusals warn only once they keep happening, as one warning.
func TestNoteLimited_WarnsAfterRepeats(t *testing.T) {
	ctx := context.Background()
	svc, _, w, _, _, c := watchedService(t)
	for i := 0; i < limitAlertAt-1; i++ {
		svc.NoteLimited(ctx, c)
	}
	if len(w.alerts) != 0 {
		t.Fatal("warned before the limit was hit repeatedly")
	}
	for i := 0; i < 30; i++ {
		svc.NoteLimited(ctx, c)
	}
	if len(w.alerts) != 1 || w.alerts[0].Kind != AlertLimit || w.alerts[0].Hits < limitAlertAt {
		t.Fatalf("alerts = %+v, want one limit warning with its hits", w.alerts)
	}
	if today, _ := svc.TodayCounts(ctx); today.Limited != limitAlertAt-1+30 {
		t.Errorf("limited today = %d", today.Limited)
	}
}

// Settling as the owner: "mine" closes the warning, "paused" also pauses
// the key and closes its other warnings; another campaign's owner can't
// settle it.
func TestSettleOwnAlert(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, raw, c := watchedService(t)
	svc.NoteConnect(ctx, c, "Claude", "")
	svc.NoteConnect(ctx, c, "curl", "")
	for i := 0; i < limitAlertAt; i++ {
		svc.NoteLimited(ctx, c)
	}
	open, _ := svc.Alerts(ctx, "camp-1")
	if len(open) != 2 {
		t.Fatalf("want a new-app and a limit warning, got %+v", open)
	}
	if err := svc.SettleOwnAlert(ctx, "camp-1", "owner-2", open[0].ID, SettledPaused); err == nil {
		t.Error("someone else settled the warning")
	}
	if err := svc.SettleOwnAlert(ctx, "camp-1", "owner-1", open[0].ID, "delete"); err == nil {
		t.Error("an unknown answer was accepted")
	}
	if err := svc.SettleOwnAlert(ctx, "camp-1", "owner-1", open[0].ID, SettledPaused); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrKeyPaused) {
		t.Errorf("after Pause the key: err = %v, want ErrKeyPaused", err)
	}
	if left, _ := svc.Alerts(ctx, "camp-1"); len(left) != 0 {
		t.Errorf("warning still open: %+v", left)
	}
	if err := svc.SettleOwnAlert(ctx, "camp-1", "owner-1", open[0].ID, SettledMine); err == nil {
		t.Error("a settled warning was settled again")
	}
}

// The site admin pauses, resumes and ends any key; ending it settles its
// warnings.
func TestAdminKey(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, raw, c := watchedService(t)
	svc.NoteConnect(ctx, c, "Claude", "")
	svc.NoteConnect(ctx, c, "curl", "")
	if _, err := svc.AdminKey(ctx, c.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrKeyPaused) {
		t.Fatalf("paused by admin: %v", err)
	}
	if _, err := svc.AdminKey(ctx, c.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminKey(ctx, c.ID, "explode"); err == nil {
		t.Error("an unknown act was accepted")
	}
	if _, err := svc.AdminKey(ctx, c.ID, "end"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrKeyEnded) {
		t.Fatalf("ended by admin: %v", err)
	}
	if open, _ := svc.Alerts(ctx, "camp-1"); len(open) != 0 {
		t.Errorf("warnings about an ended key stay open: %+v", open)
	}
	if _, err := svc.AdminKey(ctx, c.ID, "pause"); err == nil {
		t.Error("an ended key was paused")
	}
}

// A flood of wrong keys turns the address away and warns the site admin
// once; a working key from that address still gets in; lifting it, or a day
// passing, ends the turn-away.
func TestEndpoint_WrongKeyFloodIsTurnedAway(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModeSafe)
	w := newMemWatch(r.repo)
	r.svc.SetWatch(w)
	from := func(ip, key string) func(*http.Request, string) {
		return func(req *http.Request, _ string) {
			req.RemoteAddr = ip + ":5000"
			if key != "" {
				req.Header.Set("Authorization", "Bearer "+key)
			}
		}
	}
	bad := "chai_" + strings.Repeat("A", len(r.key)-len("chai_"))
	for i := 0; i < failuresPerMinute+3; i++ {
		r.post("camp-1", pingBody, from("192.0.2.50", bad))
	}
	if !r.svc.TurnedAway("192.0.2.50") {
		t.Fatal("a flooding address is not turned away")
	}
	floods := 0
	for _, a := range w.alerts {
		if a.Kind == AlertWrongKeys && a.Address == "192.0.2.50" && a.CampaignID == "" {
			floods++
		}
	}
	if floods != 1 {
		t.Fatalf("flood warnings = %d, want 1: %+v", floods, w.alerts)
	}
	if rec := r.post("camp-1", pingBody, from("192.0.2.50", bad)); rec.Code != http.StatusTooManyRequests {
		t.Errorf("wrong key from a turned-away address: %d", rec.Code)
	}
	if rec := r.post("camp-1", pingBody, from("192.0.2.50", "")); rec.Code != http.StatusOK {
		t.Errorf("working key from a turned-away address: %d %s", rec.Code, rec.Body)
	}
	if r.svc.TurnedAway("192.0.2.51") {
		t.Error("another address is turned away")
	}
	open, _ := w.OpenAlerts(context.Background(), "", 10)
	if _, err := r.svc.AdminSettle(context.Background(), open[0].ID); err != nil {
		t.Fatal(err)
	}
	if r.svc.TurnedAway("192.0.2.50") {
		t.Error("still turned away after the admin lifted it")
	}
}

// The turn-away ends by itself after blockFor, and survives a restart.
func TestTurnAway_ExpiresAndReloads(t *testing.T) {
	ctx := context.Background()
	svc, repo, w, now, _, _ := watchedService(t)
	svc.NoteRefused(ctx, "192.0.2.7", "camp-1", true)
	again := NewService(repo, owners{"camp-1": "owner-1"})
	again.now = func() time.Time { return *now }
	again.SetWatch(w)
	if !again.TurnedAway("192.0.2.7") {
		t.Fatal("a restart forgot the turn-away")
	}
	*now = now.Add(blockFor + time.Minute)
	if svc.TurnedAway("192.0.2.7") || again.TurnedAway("192.0.2.7") {
		t.Error("still turned away after a day")
	}
}

// Switches over HTTP: a stopped campaign and a server with AI off answer
// plainly, and neither counts as a wrong key.
func TestEndpoint_SwitchesAnswerPlainly(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModeSafe)
	w := newMemWatch(r.repo)
	r.svc.SetWatch(w)
	now := time.Now()
	w.camps["camp-1"] = CampaignAccess{CutOffAt: &now}
	if rec := r.post("camp-1", pingBody, nil); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "stopped AI") {
		t.Errorf("stopped campaign: %d %s", rec.Code, rec.Body)
	}
	w.camps["camp-1"] = CampaignAccess{}
	w.server.AIOff = true
	if rec := r.post("camp-1", pingBody, nil); rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "turned off") {
		t.Errorf("AI off: %d %s", rec.Code, rec.Body)
	}
	if today, _ := r.svc.TodayCounts(context.Background()); today.Refused != 0 {
		t.Errorf("a good key counted as refused: %d", today.Refused)
	}
}

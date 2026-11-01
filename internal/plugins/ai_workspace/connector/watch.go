package connector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The watch: switches that stop AI on the server or in one campaign, and
// warnings when a key is used in a way that looks wrong. A warning never
// stops a key by itself; a person reads it and decides. The one automatic
// step is turning away wrong keys from an address that floods them.

// Why a good key was refused anyway. Neither counts as a wrong key.
var (
	// ErrAIOff: the site admin turned AI off on this server.
	ErrAIOff = errors.New("AI is turned off on this server")
	// ErrCutOff: the campaign's owner stopped every AI in the campaign.
	ErrCutOff = errors.New("the campaign's owner has stopped AI in this campaign")
	// ErrPlayersBlocked: the site admin doesn't let players use AI.
	ErrPlayersBlocked = errors.New("the server's admin hasn't let players use AI")
	// ErrPlayersOff: the campaign's owner hasn't let players use AI.
	ErrPlayersOff = errors.New("the campaign's owner hasn't let players use AI here")
)

// ServerAccess is the site admin's switches. The zero value is AI on with
// players allowed, read as no row.
type ServerAccess struct {
	AIOff bool
	// PlayersBlocked hides the players switch in every campaign.
	PlayersBlocked bool
}

// CampaignAccess is one campaign's switches.
type CampaignAccess struct {
	PlayersAllowed bool
	// CutOffAt is set while the owner has stopped every AI in the campaign.
	CutOffAt *time.Time
}

// What an alert is about.
const (
	// AlertNewApp: a key connected as an AI app it had never used before.
	AlertNewApp = "new_app"
	// AlertLimit: a key kept hitting its call limit.
	AlertLimit = "limit"
	// AlertWrongKeys: one address sent a flood of wrong keys. It has no
	// campaign, and only the site admin sees it.
	AlertWrongKeys = "wrong_keys"
)

// How an alert was settled.
const (
	SettledMine   = "mine"
	SettledPaused = "paused"
	SettledEnded  = "ended"
	SettledLifted = "lifted"
	SettledSeen   = "seen"
)

// Alert is one warning, open until someone settles it.
type Alert struct {
	ID           int64
	CampaignID   string
	ConnectionID int64
	Kind         string
	Detail       string
	Address      string
	Hits         int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	SettledAt    *time.Time
	SettledHow   string
}

// KeyRow is one live key as Site admin lists it.
type KeyRow struct {
	Connection
	CampaignName string
	PersonName   string
	CallsToday   int
	CallsTotal   int
}

// WatchRepository is the SQL behind the switches and alerts.
type WatchRepository interface {
	ServerAccess(ctx context.Context) (ServerAccess, error)
	SetServerAccess(ctx context.Context, a ServerAccess, by string, at time.Time) error
	CampaignAccess(ctx context.Context, campaignID string) (CampaignAccess, error)
	SetCutOff(ctx context.Context, campaignID string, at *time.Time, now time.Time) error
	SetPlayersAllowed(ctx context.Context, campaignID string, allowed bool, now time.Time) error
	// NoteApp adds app to the key's seen apps and reports whether it is
	// new for a key that had seen another before.
	NoteApp(ctx context.Context, connID int64, app string) (bool, error)
	// RaiseAlert opens an alert, or adds a's hits to the open one about the
	// same key (or address) and kind.
	RaiseAlert(ctx context.Context, a *Alert) error
	// OpenAlerts lists open alerts newest first: a campaign's, or every one
	// when campaignID is "".
	OpenAlerts(ctx context.Context, campaignID string, limit int) ([]Alert, error)
	// RecentAlerts lists alerts opened or updated since, settled or not, for
	// the site admin's feed.
	RecentAlerts(ctx context.Context, since time.Time, limit int) ([]Alert, error)
	// SettleAlert closes an open alert of the campaign ("" for any) and
	// returns it, or nil when there was none.
	SettleAlert(ctx context.Context, campaignID string, id int64, how string, at time.Time) (*Alert, error)
	// ConnectionByID is any connection, for the site admin.
	ConnectionByID(ctx context.Context, id int64) (*Connection, error)
	// LiveKeys lists every key not revoked or run out, with its campaign,
	// its maker and its calls since since.
	LiveKeys(ctx context.Context, now, since time.Time) ([]KeyRow, error)
	// CallsSince counts every key's calls since.
	CallsSince(ctx context.Context, since time.Time) (int, error)
}

// blockFor is how long an address that flooded wrong keys is turned away.
const blockFor = 24 * time.Hour

// limitAlertAt is how many refusals for its limit in a day make a key worth
// a look; later ones are added to the same alert in steps, not one write each.
const limitAlertAt = 5

// watcher is the in-memory side of the watch: limit hits per key today, the
// addresses turned away, and today's refusals, for Site admin's counts. A
// restart forgets the counts (the page says "since"), and reloads blocks.
type watcher struct {
	mu        sync.Mutex
	day       time.Time
	limitHits map[int64]int
	refused   int
	limited   int
	blocked   map[string]time.Time
	since     time.Time
}

func newWatcher(now time.Time) *watcher {
	return &watcher{day: dayOf(now), since: now, limitHits: map[int64]int{}, blocked: map[string]time.Time{}}
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// roll starts a new day's counts. Called under mu.
func (w *watcher) roll(now time.Time) {
	if d := dayOf(now); !d.Equal(w.day) {
		w.day, w.limitHits, w.refused, w.limited = d, map[int64]int{}, 0, 0
		if w.since.Before(d) {
			w.since = d
		}
	}
}

// SetWatch wires the switches and alerts. Optional: unwired, there are no
// switches to check and nothing is watched.
func (s *Service) SetWatch(w WatchRepository) {
	s.watchRepo = w
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	open, err := w.OpenAlerts(ctx, "", 500)
	if err != nil {
		slog.Warn("ai connector: reading open alerts", slog.Any("error", err))
		return
	}
	now := s.now()
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	for _, a := range open {
		if a.Kind == AlertWrongKeys && a.Address != "" && now.Sub(a.UpdatedAt) < blockFor {
			s.watch.blocked[a.Address] = a.UpdatedAt.Add(blockFor)
		}
	}
}

// Watching reports whether the switches and alerts are wired.
func (s *Service) Watching() bool { return s.watchRepo != nil }

// checkSwitches refuses a good key while the server or its campaign has AI
// stopped. A failure to read them is logged and lets the key through: the
// switches are a brake, and the owner check and the key itself still hold.
func (s *Service) checkSwitches(ctx context.Context, campaignID string) error {
	if s.watchRepo == nil {
		return nil
	}
	sa, err := s.watchRepo.ServerAccess(ctx)
	if err != nil {
		slog.Warn("ai connector: reading server switches", slog.Any("error", err))
	} else if sa.AIOff {
		return ErrAIOff
	}
	ca, err := s.watchRepo.CampaignAccess(ctx, campaignID)
	if err != nil {
		slog.Warn("ai connector: reading campaign switches", slog.String("campaign_id", campaignID), slog.Any("error", err))
	} else if ca.CutOffAt != nil {
		return ErrCutOff
	}
	return nil
}

// checkPlayerSwitches refuses a player's key unless both the site admin and
// the campaign's owner let players use AI. It fails closed: without the
// switches, or when they can't be read, players are refused.
func (s *Service) checkPlayerSwitches(ctx context.Context, campaignID string) error {
	if s.watchRepo == nil {
		return ErrPlayersOff
	}
	sa, err := s.watchRepo.ServerAccess(ctx)
	if err != nil {
		slog.Warn("ai connector: reading server switches", slog.Any("error", err))
		return ErrPlayersBlocked
	}
	if sa.PlayersBlocked {
		return ErrPlayersBlocked
	}
	ca, err := s.watchRepo.CampaignAccess(ctx, campaignID)
	if err != nil {
		slog.Warn("ai connector: reading campaign switches", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return ErrPlayersOff
	}
	if !ca.PlayersAllowed {
		return ErrPlayersOff
	}
	return nil
}

// PlayersMayUseAI reports whether players can use AI in the campaign right
// now, and why not: ErrPlayersBlocked, ErrPlayersOff, or the campaign's
// stop (ErrCutOff, ErrAIOff), so the player's page can say which.
func (s *Service) PlayersMayUseAI(ctx context.Context, campaignID string) error {
	if err := s.checkSwitches(ctx, campaignID); err != nil {
		return err
	}
	return s.checkPlayerSwitches(ctx, campaignID)
}

// ServerSwitches is the site admin's switches, so the owner's page can hide
// the players switch the admin has blocked.
func (s *Service) ServerSwitches(ctx context.Context) (ServerAccess, error) {
	if s.watchRepo == nil {
		return ServerAccess{PlayersBlocked: true}, nil
	}
	return s.watchRepo.ServerAccess(ctx)
}

// NoteConnect records the app a key connected as, and warns the owner when
// it is one the key has never used. ip is where the call came from.
func (s *Service) NoteConnect(ctx context.Context, c *Connection, clientName, ip string) {
	if s.watchRepo == nil {
		return
	}
	app := cleanClientName(clientName)
	if app == "" {
		app = "an app with no name"
	}
	isNew, err := s.watchRepo.NoteApp(ctx, c.ID, app)
	if err != nil {
		slog.Warn("ai connector: noting app", slog.Int64("connection", c.ID), slog.Any("error", err))
		return
	}
	if !isNew {
		return
	}
	s.raise(ctx, &Alert{
		CampaignID: c.CampaignID, ConnectionID: c.ID, Kind: AlertNewApp, Address: addressOf(ip),
		Detail: oneLine(fmt.Sprintf("Connected as %q, an app this key hadn't used before", app), noteMax),
	})
}

// NoteLimited counts a key refused for its call limit, and warns once it
// keeps happening.
func (s *Service) NoteLimited(ctx context.Context, c *Connection) {
	now := s.now()
	s.watch.mu.Lock()
	s.watch.roll(now)
	s.watch.limited++
	s.watch.limitHits[c.ID]++
	n := s.watch.limitHits[c.ID]
	s.watch.mu.Unlock()
	if s.watchRepo == nil || n < limitAlertAt || (n != limitAlertAt && n%25 != 0) {
		return
	}
	hits := 25
	if n == limitAlertAt {
		hits = limitAlertAt
	}
	s.raise(ctx, &Alert{
		CampaignID: c.CampaignID, ConnectionID: c.ID, Kind: AlertLimit, Hits: hits,
		Detail: "Kept hitting its call limit today",
	})
}

// NoteRefused counts one refused key for today's figures. When the address
// passes the flood line for the first time it is turned away for blockFor
// and the site admin is warned.
func (s *Service) NoteRefused(ctx context.Context, ip, campaignID string, flooded bool) {
	now := s.now()
	addr := addressOf(ip)
	s.watch.mu.Lock()
	s.watch.roll(now)
	s.watch.refused++
	_, already := s.watch.blocked[addr]
	if flooded && !already && addr != "" {
		s.watch.blocked[addr] = now.Add(blockFor)
	}
	s.watch.mu.Unlock()
	if !flooded || already || addr == "" || s.watchRepo == nil {
		return
	}
	slog.Warn("ai connector: turning away wrong keys from an address", slog.String("ip", addr), slog.Duration("for", blockFor))
	s.raise(ctx, &Alert{
		Kind: AlertWrongKeys, Address: addr, Hits: failuresPerMinute + 1,
		Detail: oneLine(fmt.Sprintf("Wrong keys flooded in from %s (last tried campaign %s). Wrong keys from it are turned away for a day; working keys still get in.", addr, campaignID), summaryMax),
	})
}

// TurnedAway reports whether wrong keys from ip are being turned away.
func (s *Service) TurnedAway(ip string) bool {
	addr := addressOf(ip)
	now := s.now()
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	until, ok := s.watch.blocked[addr]
	if ok && !now.Before(until) {
		delete(s.watch.blocked, addr)
		return false
	}
	return ok
}

func (s *Service) raise(ctx context.Context, a *Alert) {
	now := s.now().UTC().Truncate(time.Second)
	a.CreatedAt, a.UpdatedAt = now, now
	if a.Hits < 1 {
		a.Hits = 1
	}
	if err := s.watchRepo.RaiseAlert(ctx, a); err != nil {
		slog.Warn("ai connector: raising alert", slog.String("kind", a.Kind), slog.Any("error", err))
	}
}

// addressOf is ip in its canonical form, or "" when it isn't one address.
func addressOf(ip string) string {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return ""
	}
	return p.String()
}

// Today is the watch's counts for Site admin: refusals and limit hits since
// Since (midnight, or the server's start if later).
type Today struct {
	Since   time.Time
	Refused int
	Limited int
	Calls   int
}

// TodayCounts reads today's counts.
func (s *Service) TodayCounts(ctx context.Context) (Today, error) {
	now := s.now()
	s.watch.mu.Lock()
	s.watch.roll(now)
	t := Today{Since: s.watch.since, Refused: s.watch.refused, Limited: s.watch.limited}
	s.watch.mu.Unlock()
	if s.watchRepo != nil {
		n, err := s.watchRepo.CallsSince(ctx, dayOf(now).UTC())
		if err != nil {
			return t, err
		}
		t.Calls = n
	}
	return t, nil
}

// --- The owner's side ---

// Alerts is the campaign's open warnings, for its owner.
func (s *Service) Alerts(ctx context.Context, campaignID string) ([]Alert, error) {
	if s.watchRepo == nil {
		return nil, nil
	}
	return s.watchRepo.OpenAlerts(ctx, campaignID, 20)
}

// CampaignSwitches is the campaign's switches, for its owner.
func (s *Service) CampaignSwitches(ctx context.Context, campaignID string) (CampaignAccess, error) {
	if s.watchRepo == nil {
		return CampaignAccess{}, nil
	}
	return s.watchRepo.CampaignAccess(ctx, campaignID)
}

// SettleOwnAlert closes one of the campaign's warnings for its owner: "mine"
// marks it as them, "paused" also pauses the key it is about.
func (s *Service) SettleOwnAlert(ctx context.Context, campaignID, userID string, id int64, how string) error {
	if s.watchRepo == nil {
		return apperror.NewNotFound("warnings aren't available")
	}
	if how != SettledMine && how != SettledPaused {
		return apperror.NewBadRequest("pick what to do about it")
	}
	if _, err := s.owners.Check(ctx, campaignID, userID); err != nil {
		return apperror.NewForbidden("only the campaign's owner can answer its AI warnings")
	}
	a, err := s.watchRepo.SettleAlert(ctx, campaignID, id, how, s.now().UTC())
	if err != nil {
		return err
	}
	if a == nil {
		return apperror.NewNotFound("that warning was already answered")
	}
	if how == SettledPaused && a.ConnectionID != 0 {
		at := s.now().UTC().Truncate(time.Second)
		if err := s.repo.SetPaused(ctx, campaignID, a.ConnectionID, &at); err != nil {
			return err
		}
		// Pausing answers every warning about that key, not just this one.
		s.settleKeyAlerts(ctx, a.ConnectionID, SettledPaused)
	}
	return nil
}

// SetCutOff stops (on) or lets back (off) every AI in the campaign.
func (s *Service) SetCutOff(ctx context.Context, campaignID, userID string, on bool) error {
	if s.watchRepo == nil {
		return apperror.NewNotFound("this switch isn't available")
	}
	if _, err := s.owners.Check(ctx, campaignID, userID); err != nil {
		return apperror.NewForbidden("only the campaign's owner can stop AI in it")
	}
	now := s.now().UTC().Truncate(time.Second)
	var at *time.Time
	if on {
		at = &now
	}
	return s.watchRepo.SetCutOff(ctx, campaignID, at, now)
}

// SetPlayersAllowed lets the campaign's players make and use their own AI
// key (on), or stops every player's key there (off). Turning it off keeps
// the keys, so turning it back on needs nothing pasted again.
func (s *Service) SetPlayersAllowed(ctx context.Context, campaignID, userID string, on bool) error {
	if s.watchRepo == nil {
		return apperror.NewNotFound("this switch isn't available")
	}
	if _, err := s.owners.Check(ctx, campaignID, userID); err != nil {
		return apperror.NewForbidden("only the campaign's owner can let players use AI")
	}
	if on {
		sa, err := s.watchRepo.ServerAccess(ctx)
		if err != nil {
			return err
		}
		if sa.PlayersBlocked {
			return apperror.NewForbidden("the server's admin hasn't let players use AI")
		}
	}
	return s.watchRepo.SetPlayersAllowed(ctx, campaignID, on, s.now().UTC().Truncate(time.Second))
}

// PlayerKeys lists the campaign's player keys for its owner.
func (s *Service) PlayerKeys(ctx context.Context, campaignID, userID string) ([]KeyRow, error) {
	if _, err := s.owners.Check(ctx, campaignID, userID); err != nil {
		return nil, apperror.NewForbidden("only the campaign's owner can see players' AI keys")
	}
	return s.repo.PlayerKeys(ctx, campaignID, s.now().UTC(), dayOf(s.now()).UTC())
}

// StopPlayerKey pauses (stop) or resumes one player's key for the
// campaign's owner. It returns the key, for the audit line.
func (s *Service) StopPlayerKey(ctx context.Context, campaignID, userID string, id int64, stop bool) (*Connection, error) {
	c, err := s.ownersPlayerKey(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	var at *time.Time
	if stop {
		now := s.now().UTC().Truncate(time.Second)
		at = &now
	}
	if err := s.repo.SetPaused(ctx, campaignID, id, at); err != nil {
		return nil, err
	}
	if stop {
		s.settleKeyAlerts(ctx, id, SettledPaused)
	}
	return c, nil
}

// EndPlayerKey disconnects one player's key for the campaign's owner. The
// player can make a new one while players may use AI.
func (s *Service) EndPlayerKey(ctx context.Context, campaignID, userID string, id int64) (*Connection, error) {
	c, err := s.ownersPlayerKey(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RevokeOne(ctx, campaignID, id, s.now().UTC().Truncate(time.Second)); err != nil {
		return nil, err
	}
	s.settleKeyAlerts(ctx, id, SettledEnded)
	return c, nil
}

// ownersPlayerKey is a live player's key of the campaign, for its owner.
func (s *Service) ownersPlayerKey(ctx context.Context, campaignID, userID string, id int64) (*Connection, error) {
	if s.watchRepo == nil {
		return nil, apperror.NewNotFound("this isn't available")
	}
	if _, err := s.owners.Check(ctx, campaignID, userID); err != nil {
		return nil, apperror.NewForbidden("only the campaign's owner can stop a player's AI")
	}
	c, err := s.watchRepo.ConnectionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || c.CampaignID != campaignID || !c.ForPlayer || !c.Live(s.now()) {
		return nil, apperror.NewNotFound("that key was ended or has run out; reload the page")
	}
	return c, nil
}

// --- The site admin's side ---

// AdminView is everything Site admin ▸ AI shows.
type AdminView struct {
	Access ServerAccess
	Today  Today
	Keys   []KeyRow
	// Alerts is every open warning, keyed by the key it is about; Floods
	// are the open warnings about addresses.
	Alerts map[int64][]Alert
	Floods []Alert
	Feed   []Alert
}

// Admin reads Site admin ▸ AI.
func (s *Service) Admin(ctx context.Context) (*AdminView, error) {
	if s.watchRepo == nil {
		return nil, apperror.NewNotFound("the AI watch isn't available")
	}
	now := s.now()
	v := &AdminView{Alerts: map[int64][]Alert{}}
	var err error
	if v.Access, err = s.watchRepo.ServerAccess(ctx); err != nil {
		return nil, err
	}
	if v.Today, err = s.TodayCounts(ctx); err != nil {
		return nil, err
	}
	if v.Keys, err = s.watchRepo.LiveKeys(ctx, now.UTC(), dayOf(now).UTC()); err != nil {
		return nil, err
	}
	open, err := s.watchRepo.OpenAlerts(ctx, "", 200)
	if err != nil {
		return nil, err
	}
	for _, a := range open {
		if a.ConnectionID != 0 {
			v.Alerts[a.ConnectionID] = append(v.Alerts[a.ConnectionID], a)
		} else {
			v.Floods = append(v.Floods, a)
		}
	}
	if v.Feed, err = s.watchRepo.RecentAlerts(ctx, now.UTC().Add(-24*time.Hour), 30); err != nil {
		return nil, err
	}
	return v, nil
}

// SetServerAccess changes the site admin's switches.
func (s *Service) SetServerAccess(ctx context.Context, a ServerAccess, adminID string) error {
	if s.watchRepo == nil {
		return apperror.NewNotFound("the AI watch isn't available")
	}
	return s.watchRepo.SetServerAccess(ctx, a, adminID, s.now().UTC().Truncate(time.Second))
}

// AdminKey changes any live key for the site admin: "pause", "resume" or
// "end". It returns the key as it was, for the audit line.
func (s *Service) AdminKey(ctx context.Context, id int64, act string) (*Connection, error) {
	if s.watchRepo == nil {
		return nil, apperror.NewNotFound("the AI watch isn't available")
	}
	c, err := s.watchRepo.ConnectionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil || !c.Live(s.now()) {
		return nil, apperror.NewNotFound("that key was already ended or has run out")
	}
	now := s.now().UTC().Truncate(time.Second)
	switch act {
	case "pause":
		err = s.repo.SetPaused(ctx, c.CampaignID, c.ID, &now)
	case "resume":
		err = s.repo.SetPaused(ctx, c.CampaignID, c.ID, nil)
	case "end":
		err = s.repo.RevokeOne(ctx, c.CampaignID, c.ID, now)
	default:
		return nil, apperror.NewBadRequest("pick pause, resume or end")
	}
	if err != nil {
		return nil, err
	}
	if act != "resume" {
		s.settleKeyAlerts(ctx, c.ID, map[string]string{"pause": SettledPaused, "end": SettledEnded}[act])
	}
	return c, nil
}

// settleKeyAlerts closes the open warnings about one key.
func (s *Service) settleKeyAlerts(ctx context.Context, connID int64, how string) {
	open, err := s.watchRepo.OpenAlerts(ctx, "", 200)
	if err != nil {
		slog.Warn("ai connector: settling alerts", slog.Any("error", err))
		return
	}
	for _, a := range open {
		if a.ConnectionID == connID {
			if _, err := s.watchRepo.SettleAlert(ctx, "", a.ID, how, s.now().UTC()); err != nil {
				slog.Warn("ai connector: settling alert", slog.Int64("alert", a.ID), slog.Any("error", err))
			}
		}
	}
}

// AdminSettle closes any warning for the site admin. Settling a flood
// warning lifts its address's turn-away at once.
func (s *Service) AdminSettle(ctx context.Context, id int64) (*Alert, error) {
	if s.watchRepo == nil {
		return nil, apperror.NewNotFound("the AI watch isn't available")
	}
	a, err := s.watchRepo.SettleAlert(ctx, "", id, SettledSeen, s.now().UTC())
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, apperror.NewNotFound("that warning was already answered")
	}
	if a.Kind == AlertWrongKeys && a.Address != "" {
		s.watch.mu.Lock()
		delete(s.watch.blocked, a.Address)
		s.watch.mu.Unlock()
	}
	return a, nil
}

// foundry_players.go — who is in a campaign's Foundry world and which member
// each Foundry user is linked to, as the GM's Foundry client reports it.
//
// Chronicle cannot see this on its own: players' changes arrive on the GM's
// connection, so only Foundry knows which player made one. The GM's client
// sends the whole list (POST /sync/players); each report replaces the
// campaign's rows. The owner's Foundry page and the People page read it.
//
// Everything in a report is the client's word: names are capped and escaped
// on display, member ids that are not this campaign's members are dropped,
// and times are clamped, so a bad or hostile client can at worst mislabel
// its own campaign's diagnostics.

package syncapi

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// FoundryPlayer is one Foundry user of a campaign's world.
type FoundryPlayer struct {
	FoundryUserID string
	Name          string
	// MemberUserID is the Chronicle member the GM linked this Foundry user
	// to; empty when not linked.
	MemberUserID string
	Online       bool
	// LastChangeAt is when a change this user made last synced.
	LastChangeAt *time.Time
	// LastFailedAt, LastFailure and FailedCount describe this user's changes
	// that Chronicle refused or that never arrived, as Foundry counted them.
	LastFailedAt *time.Time
	LastFailure  string
	FailedCount  int
	ReportedAt   time.Time
}

// Limits on one report and on what is kept.
const (
	foundryPlayersMax       = 100
	foundryPlayerIDMax      = 64
	foundryPlayerNameMax    = 100
	foundryPlayerFailureMax = 200
	// foundryPlayersRetention is how long a campaign's list is kept after its
	// last report; a world nobody opens stops telling us anything.
	foundryPlayersRetention = 30 * 24 * time.Hour
)

// FoundryPlayerRepository owns the foundry_players table.
type FoundryPlayerRepository interface {
	// Replace makes players the campaign's whole list.
	Replace(ctx context.Context, campaignID string, players []FoundryPlayer) error
	// List returns the campaign's list, linked members first, then by name.
	List(ctx context.Context, campaignID string) ([]FoundryPlayer, error)
	// PruneOlderThan deletes rows last reported before cutoff.
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type foundryPlayerRepo struct {
	db *sql.DB
}

// NewFoundryPlayerRepository creates the MariaDB-backed repository.
func NewFoundryPlayerRepository(db *sql.DB) FoundryPlayerRepository {
	return &foundryPlayerRepo{db: db}
}

func (r *foundryPlayerRepo) Replace(ctx context.Context, campaignID string, players []FoundryPlayer) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("begin foundry players: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	prev, err := listPlayers(ctx, tx, campaignID)
	if err != nil {
		return err
	}
	byID := make(map[string]FoundryPlayer, len(prev))
	for _, old := range prev {
		byID[old.FoundryUserID] = old
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM foundry_players WHERE campaign_id = ?`, campaignID); err != nil {
		return apperror.NewInternal(fmt.Errorf("clear foundry players: %w", err))
	}
	for _, p := range players {
		if old, ok := byID[p.FoundryUserID]; ok {
			p = carryForward(p, old)
		}
		var member any
		if p.MemberUserID != "" {
			member = p.MemberUserID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO foundry_players
			(campaign_id, foundry_user_id, foundry_name, member_user_id, is_online,
			 last_change_at, last_failed_at, last_failure, failed_count, reported_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			campaignID, p.FoundryUserID, p.Name, member, p.Online,
			utcOrNil(p.LastChangeAt), utcOrNil(p.LastFailedAt), p.LastFailure, p.FailedCount, p.ReportedAt.UTC()); err != nil {
			return apperror.NewInternal(fmt.Errorf("insert foundry player: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return apperror.NewInternal(fmt.Errorf("commit foundry players: %w", err))
	}
	return nil
}

func utcOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// carryForward keeps what an earlier report knew and this one doesn't. The
// module counts a user's changes only while the world is open, so each new
// Foundry session reports none; without this a player who synced yesterday
// would read "none yet" today. The newer of each time wins.
func carryForward(p, old FoundryPlayer) FoundryPlayer {
	if old.LastChangeAt != nil && (p.LastChangeAt == nil || old.LastChangeAt.After(*p.LastChangeAt)) {
		p.LastChangeAt = old.LastChangeAt
	}
	if old.LastFailedAt != nil && (p.LastFailedAt == nil || old.LastFailedAt.After(*p.LastFailedAt)) {
		p.LastFailedAt, p.LastFailure, p.FailedCount = old.LastFailedAt, old.LastFailure, old.FailedCount
	}
	return p
}

func (r *foundryPlayerRepo) List(ctx context.Context, campaignID string) ([]FoundryPlayer, error) {
	return listPlayers(ctx, r.db, campaignID)
}

// queryer is what listPlayers needs from a *sql.DB or *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func listPlayers(ctx context.Context, db queryer, campaignID string) ([]FoundryPlayer, error) {
	rows, err := db.QueryContext(ctx, `SELECT foundry_user_id, foundry_name, member_user_id, is_online,
		last_change_at, last_failed_at, last_failure, failed_count, reported_at
		FROM foundry_players WHERE campaign_id = ?
		ORDER BY member_user_id IS NULL, foundry_name, foundry_user_id`, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list foundry players: %w", err))
	}
	defer rows.Close()
	out := []FoundryPlayer{}
	for rows.Next() {
		var p FoundryPlayer
		var member sql.NullString
		var change, failed sql.NullTime
		if err := rows.Scan(&p.FoundryUserID, &p.Name, &member, &p.Online,
			&change, &failed, &p.LastFailure, &p.FailedCount, &p.ReportedAt); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan foundry player: %w", err))
		}
		p.MemberUserID = member.String
		if change.Valid {
			t := change.Time
			p.LastChangeAt = &t
		}
		if failed.Valid {
			t := failed.Time
			p.LastFailedAt = &t
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read foundry players: %w", err))
	}
	return out, nil
}

func (r *foundryPlayerRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM foundry_players WHERE reported_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("prune foundry players: %w", err))
	}
	return res.RowsAffected()
}

// reportedPlayer is one entry of POST /sync/players, as the module sends it.
type reportedPlayer struct {
	FoundryUserID string     `json:"foundryUserId"`
	Name          string     `json:"name"`
	MemberID      string     `json:"memberId"`
	Online        bool       `json:"online"`
	LastChangeAt  *time.Time `json:"lastChangeAt"`
	LastFailedAt  *time.Time `json:"lastFailedAt"`
	LastFailure   string     `json:"lastFailure"`
	FailedCount   int        `json:"failedCount"`
}

type reportPlayersRequest struct {
	Players []reportedPlayer `json:"players"`
}

// cleanPlayers validates and clamps a report. members is the campaign's
// member ids; a link to anyone else is dropped rather than stored, so the
// list can never name a stranger as a player. A duplicate Foundry id keeps
// its first entry.
func cleanPlayers(in []reportedPlayer, members map[string]bool, now time.Time) ([]FoundryPlayer, error) {
	if len(in) > foundryPlayersMax {
		return nil, apperror.NewBadRequest(fmt.Sprintf("too many players in one report (at most %d)", foundryPlayersMax))
	}
	seen := make(map[string]bool, len(in))
	out := make([]FoundryPlayer, 0, len(in))
	for _, rp := range in {
		id := strings.TrimSpace(rp.FoundryUserID)
		if id == "" || len(id) > foundryPlayerIDMax || !isPrintableASCII(id) {
			return nil, apperror.NewBadRequest("each player needs a foundryUserId of 1 to 64 plain characters")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		p := FoundryPlayer{
			FoundryUserID: id,
			Name:          truncate(strings.TrimSpace(stripControl(rp.Name)), foundryPlayerNameMax),
			Online:        rp.Online,
			LastChangeAt:  clampReported(rp.LastChangeAt, now),
			LastFailedAt:  clampReported(rp.LastFailedAt, now),
			LastFailure:   truncate(strings.TrimSpace(stripControl(rp.LastFailure)), foundryPlayerFailureMax),
			ReportedAt:    now,
		}
		if members[rp.MemberID] {
			p.MemberUserID = rp.MemberID
		}
		if rp.FailedCount > 0 {
			p.FailedCount = min(rp.FailedCount, 100000)
		}
		out = append(out, p)
	}
	return out, nil
}

// clampReported drops a time in the future or older than the retention, so a
// wrong clock cannot make a player look active.
func clampReported(t *time.Time, now time.Time) *time.Time {
	if t == nil || t.IsZero() || t.After(now.Add(5*time.Minute)) || t.Before(now.Add(-foundryPlayersRetention)) {
		return nil
	}
	v := t.UTC()
	return &v
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// stripControl removes control characters so a name cannot carry line
// breaks or terminal escapes into a page or a log.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// PlayerStatus is how one person's Foundry link reads on the owner's pages.
type PlayerStatus string

const (
	PlayerWorking   PlayerStatus = "working"
	PlayerIdle      PlayerStatus = "idle"
	PlayerRefused   PlayerStatus = "refused"
	PlayerUnlinked  PlayerStatus = "unlinked"
	PlayerNoFoundry PlayerStatus = "none"
)

// recentWindow is how long a synced change keeps a player Working and a
// failed one keeps them marked.
const recentWindow = 7 * 24 * time.Hour

// PlayerRow is one row of the Players in Foundry table: a Foundry user, a
// member with no Foundry user, or both.
type PlayerRow struct {
	MemberUserID string
	MemberName   string
	MemberRole   string
	FoundryName  string
	Online       bool
	LastChangeAt *time.Time
	LastFailedAt *time.Time
	LastFailure  string
	FailedCount  int
	Status       PlayerStatus
	// Why and Fix explain a status that needs the owner; empty when working.
	Why string
	Fix []string
}

// FoundryPlayersView is the table plus when Foundry last reported it.
type FoundryPlayersView struct {
	CampaignID string
	Rows       []PlayerRow
	ReportedAt *time.Time
	Now        time.Time
}

// MemberRef is the part of a campaign member the table shows.
type MemberRef struct {
	UserID string
	Name   string
	Role   string
	Owner  bool
}

// buildPlayerRows joins Foundry's list with the campaign's members. Linked
// Foundry users come first, then Foundry users nobody linked, then members
// with no Foundry user (the owner is left out of that last group: the GM
// account is theirs whether or not it is linked).
func buildPlayerRows(players []FoundryPlayer, members []MemberRef, now time.Time) []PlayerRow {
	byID := make(map[string]MemberRef, len(members))
	for _, m := range members {
		byID[m.UserID] = m
	}
	linked := map[string]bool{}
	var rows, unlinked []PlayerRow
	for _, p := range players {
		r := PlayerRow{FoundryName: p.Name, Online: p.Online, LastChangeAt: p.LastChangeAt,
			LastFailedAt: p.LastFailedAt, LastFailure: p.LastFailure, FailedCount: p.FailedCount}
		m, ok := byID[p.MemberUserID]
		if !ok {
			r.Status = PlayerUnlinked
			r.Why = "This Foundry user isn't linked to anyone in this campaign, so Chronicle can't tell who they are. Shops and stashes refuse them until they are linked."
			r.Fix = []string{
				"In Foundry, open the Chronicle Sync window and go to Members.",
				"Next to this Foundry user, pick the member they are.",
			}
			unlinked = append(unlinked, r)
			continue
		}
		linked[m.UserID] = true
		r.MemberUserID, r.MemberName, r.MemberRole = m.UserID, m.Name, m.Role
		// Working is a claim that something synced lately, never just a link.
		switch {
		case p.LastFailedAt != nil && now.Sub(*p.LastFailedAt) < recentWindow &&
			(p.LastChangeAt == nil || !p.LastChangeAt.After(*p.LastFailedAt)):
			r.Status = PlayerRefused
			r.Why = "The latest sync through Foundry as " + m.Name + " (a shop buy, a stash move, or a change going either way) failed."
			if p.LastFailure != "" {
				r.Why += " Foundry was told: " + p.LastFailure
			}
			r.Fix = []string{
				"If Chronicle refused something " + m.Name + " should be allowed to do, give them a role that allows it on the People page.",
				"If it failed inside Foundry, the Chronicle Sync window's Debug tab has the details, and Foundry tries again on the next sync.",
			}
		case p.LastChangeAt != nil && now.Sub(*p.LastChangeAt) < recentWindow:
			r.Status = PlayerWorking
		default:
			r.Status = PlayerIdle
			r.Why = "Linked, but nothing done in Foundry as " + m.Name + " has synced in the last week, so there's nothing to show yet. Changes only count while your world is open."
		}
		rows = append(rows, r)
	}
	rows = append(rows, unlinked...)
	for _, m := range members {
		if linked[m.UserID] || m.Owner {
			continue
		}
		rows = append(rows, PlayerRow{MemberUserID: m.UserID, MemberName: m.Name, MemberRole: m.Role,
			Status: PlayerNoFoundry,
			Why:    "No Foundry user is linked to " + m.Name + ". If they play in Foundry, their shop and stash windows won't work until they are.",
			Fix: []string{
				"In Foundry, open the Chronicle Sync window and go to Members.",
				"Next to " + m.Name + "'s Foundry user, pick " + m.Name + ".",
			}})
	}
	return rows
}

// latestReport is when Foundry last sent the list, nil when it never has.
func latestReport(players []FoundryPlayer) *time.Time {
	var at *time.Time
	for i := range players {
		if at == nil || players[i].ReportedAt.After(*at) {
			t := players[i].ReportedAt
			at = &t
		}
	}
	return at
}

// problemEvent is the failed row to describe: the run itself, or its first
// failed step when the run's own row reads fine.
func problemEvent(ev SyncEvent) SyncEvent {
	if !ev.OK {
		return ev
	}
	for _, ch := range ev.Children {
		if !ch.OK {
			return ch
		}
	}
	return ev
}

// problemLine is the short reason under a problem: Chronicle's message when
// there is one, else what the answer's code means.
func problemLine(ev SyncEvent) string {
	if ev.Message != "" {
		return shortProblem(ev) + ". " + ev.Message
	}
	return shortProblem(ev)
}

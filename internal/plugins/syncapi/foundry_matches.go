// foundry_matches.go — which of Foundry's monsters each creature page is.
//
// A creature page and a monster in Foundry (a world actor or a compendium
// entry) can be the same creature. Rather than keep two copies, each page is
// matched to Foundry's monster of the same name, and the DM chooses per page
// whose numbers play: Foundry's (the page keeps the lore; Foundry places its
// own actor) or the page's (creature sync makes the actor, as for a homebrew
// monster). A name that only looks alike is a suggestion until the DM
// confirms it, and the DM can say a same-named monster is a different one.
//
// The match itself is worked out on every read from the latest monster
// report; only the DM's choices are stored, so a renamed page or a changed
// compendium never leaves a stale match behind.

package syncapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Whose numbers a matched page plays with.
const (
	MatchUseFoundry = "foundry"
	MatchUsePage    = "page"
)

// Match states, as the page panel and the module read them.
const (
	// MatchMatched: a monster in Foundry is this page.
	MatchMatched = "matched"
	// MatchSuggested: a monster's name looks like this page's; the DM confirms.
	MatchSuggested = "suggested"
	// MatchMissing: the DM matched a monster Foundry no longer reports.
	MatchMissing = "missing"
	// MatchNone: nothing in Foundry is this page; creature sync makes its actor.
	MatchNone = "none"
	// MatchNotReported: Foundry has not sent its monster list yet.
	MatchNotReported = "not_reported"
)

// foundryMatchNotSameMax bounds the "not the same monster" list per page.
const foundryMatchNotSameMax = 20

// FoundryMatchChoice is what the DM decided for one page. A zero choice
// follows the page's name.
type FoundryMatchChoice struct {
	EntityID string
	// Name and Pack name the monster the DM matched; empty follows the name.
	Name, Pack string
	// Use is MatchUseFoundry, MatchUsePage, or empty for the default.
	Use string
	// NotSame lists monster names the DM said are a different creature.
	NotSame []string
}

// CreatureMatch is one page's match as the API answers it.
type CreatureMatch struct {
	EntityID   string          `json:"entityId"`
	EntityName string          `json:"entityName"`
	State      string          `json:"state"`
	Monster    *FoundryMonster `json:"monster,omitempty"`
	// Use is whose numbers play; set for matched and missing pages.
	Use string `json:"use,omitempty"`
	// UseChosen says the DM picked Use; otherwise it is the default.
	UseChosen bool `json:"useChosen"`
	// Confirmed says the DM matched this monster by hand.
	Confirmed bool `json:"confirmed"`
	// InPlay says creature sync already made an actor from this page.
	InPlay bool `json:"inPlay"`
}

// FoundryMatchRepository owns the foundry_monster_matches table.
type FoundryMatchRepository interface {
	// Get is one page's choice; nil when the DM never made one.
	Get(ctx context.Context, campaignID, entityID string) (*FoundryMatchChoice, error)
	// List is every choice in the campaign, by page.
	List(ctx context.Context, campaignID string) (map[string]FoundryMatchChoice, error)
	// Put stores a page's choice; a zero choice deletes the row.
	Put(ctx context.Context, campaignID string, ch FoundryMatchChoice) error
}

type foundryMatchRepo struct{ db *sql.DB }

// NewFoundryMatchRepository creates the MariaDB-backed repository.
func NewFoundryMatchRepository(db *sql.DB) FoundryMatchRepository {
	return &foundryMatchRepo{db: db}
}

func (r *foundryMatchRepo) Get(ctx context.Context, campaignID, entityID string) (*FoundryMatchChoice, error) {
	ch := FoundryMatchChoice{EntityID: entityID}
	var notSame string
	err := r.db.QueryRowContext(ctx, `SELECT monster_name, monster_pack, use_numbers, not_same
		FROM foundry_monster_matches WHERE campaign_id = ? AND entity_id = ?`, campaignID, entityID).
		Scan(&ch.Name, &ch.Pack, &ch.Use, &notSame)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("read foundry match: %w", err))
	}
	if err := json.Unmarshal([]byte(notSame), &ch.NotSame); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("decode foundry match: %w", err))
	}
	return &ch, nil
}

func (r *foundryMatchRepo) List(ctx context.Context, campaignID string) (map[string]FoundryMatchChoice, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT entity_id, monster_name, monster_pack, use_numbers, not_same
		FROM foundry_monster_matches WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list foundry matches: %w", err))
	}
	defer func() { _ = rows.Close() }()
	out := map[string]FoundryMatchChoice{}
	for rows.Next() {
		var ch FoundryMatchChoice
		var notSame string
		if err := rows.Scan(&ch.EntityID, &ch.Name, &ch.Pack, &ch.Use, &notSame); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan foundry match: %w", err))
		}
		if err := json.Unmarshal([]byte(notSame), &ch.NotSame); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("decode foundry match: %w", err))
		}
		out[ch.EntityID] = ch
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list foundry matches: %w", err))
	}
	return out, nil
}

func (r *foundryMatchRepo) Put(ctx context.Context, campaignID string, ch FoundryMatchChoice) error {
	if ch.Name == "" && ch.Pack == "" && ch.Use == "" && len(ch.NotSame) == 0 {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM foundry_monster_matches WHERE campaign_id = ? AND entity_id = ?`,
			campaignID, ch.EntityID); err != nil {
			return apperror.NewInternal(fmt.Errorf("clear foundry match: %w", err))
		}
		return nil
	}
	notSame := ch.NotSame
	if notSame == nil {
		notSame = []string{}
	}
	data, err := json.Marshal(notSame)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("encode foundry match: %w", err))
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO foundry_monster_matches
		(campaign_id, entity_id, monster_name, monster_pack, use_numbers, not_same, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE monster_name = VALUES(monster_name), monster_pack = VALUES(monster_pack),
			use_numbers = VALUES(use_numbers), not_same = VALUES(not_same), updated_at = VALUES(updated_at)`,
		campaignID, ch.EntityID, ch.Name, ch.Pack, ch.Use, string(data), time.Now().UTC()); err != nil {
		return apperror.NewInternal(fmt.Errorf("store foundry match: %w", err))
	}
	return nil
}

// matchKey is a name as an exact match compares it: case and spacing aside.
func matchKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// looseKey is a name as a suggestion compares it: letters and digits only,
// a bracketed note dropped ("Stalker (level 3)") and a plural s ignored.
func looseKey(s string) string {
	if i := strings.IndexAny(s, "(["); i > 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSuffix(b.String(), "s")
}

// matchCreature works out one page's match from the latest report and the
// DM's choice (nil for none). Actors creature sync made are never a match:
// they are the page's own copy, and mark it as in play.
func matchCreature(entityID, name string, list *FoundryMonsterList, ch *FoundryMatchChoice) CreatureMatch {
	out := CreatureMatch{EntityID: entityID, EntityName: name}
	if list == nil {
		out.State = MatchNotReported
		return out
	}
	if ch == nil {
		ch = &FoundryMatchChoice{}
	}
	notSame := map[string]bool{}
	for _, n := range ch.NotSame {
		notSame[matchKey(n)] = true
	}
	var candidates []FoundryMonster
	for _, m := range list.Monsters {
		if m.Linked {
			if m.Page == entityID {
				out.InPlay = true
			}
			continue
		}
		candidates = append(candidates, m)
	}
	// A world actor wins over a compendium entry of the same name: it is the
	// one Foundry places first, and may carry the DM's own edits.
	pick := func(keep func(FoundryMonster) bool) *FoundryMonster {
		var found *FoundryMonster
		for i := range candidates {
			m := candidates[i]
			if !keep(m) {
				continue
			}
			if found == nil || (found.Pack != "" && m.Pack == "") {
				found = &m
			}
		}
		return found
	}

	switch {
	case ch.Name != "":
		out.Confirmed = true
		want := matchKey(ch.Name)
		m := pick(func(m FoundryMonster) bool { return matchKey(m.Name) == want && m.Pack == ch.Pack })
		if m == nil {
			m = pick(func(m FoundryMonster) bool { return matchKey(m.Name) == want })
		}
		if m != nil {
			out.State, out.Monster = MatchMatched, m
		} else {
			out.State, out.Monster = MatchMissing, &FoundryMonster{Name: ch.Name, Pack: ch.Pack}
		}
	default:
		want := matchKey(name)
		if m := pick(func(m FoundryMonster) bool { return matchKey(m.Name) == want && !notSame[matchKey(m.Name)] }); m != nil {
			out.State, out.Monster = MatchMatched, m
			break
		}
		loose := looseKey(name)
		if m := pick(func(m FoundryMonster) bool {
			return loose != "" && looseKey(m.Name) == loose && !notSame[matchKey(m.Name)]
		}); m != nil {
			out.State, out.Monster = MatchSuggested, m
			break
		}
		out.State = MatchNone
	}

	if out.State == MatchMatched || out.State == MatchMissing {
		out.Use, out.UseChosen = ch.Use, ch.Use != ""
		if out.Use == "" {
			// A page whose actor is already in play keeps playing as it is
			// until the DM switches it; otherwise Foundry's numbers play.
			out.Use = MatchUseFoundry
			if out.InPlay {
				out.Use = MatchUsePage
			}
		}
	}
	return out
}

// FoundryMatchAction is one change the DM makes to a page's match.
type FoundryMatchAction struct {
	// Action is "use", "match", "not_same" or "reset".
	Action string `json:"action"`
	// Use is MatchUseFoundry or MatchUsePage, for "use".
	Use string `json:"use,omitempty"`
	// Name and Pack name the monster, for "match" and "not_same".
	Name string `json:"name,omitempty"`
	Pack string `json:"pack,omitempty"`
}

// applyMatchAction returns the page's choice after the DM's action. A match
// must name a monster Foundry reports, so a typo cannot pin a page to
// nothing; "use" needs the page's current match to be one.
func applyMatchAction(ch FoundryMatchChoice, cur CreatureMatch, list *FoundryMonsterList, a FoundryMatchAction) (FoundryMatchChoice, error) {
	field := func(s string, n int) string { return truncate(strings.TrimSpace(stripControl(s)), n) }
	switch a.Action {
	case "use":
		if a.Use != MatchUseFoundry && a.Use != MatchUsePage {
			return ch, apperror.NewBadRequest(`use must be "foundry" or "page"`)
		}
		if cur.State != MatchMatched && cur.State != MatchMissing {
			return ch, apperror.NewBadRequest("this page is not matched to a monster in Foundry")
		}
		ch.Use = a.Use
	case "match":
		name, pack := field(a.Name, foundryMonsterNameMax), field(a.Pack, foundryMonsterPackMax)
		if name == "" {
			return ch, apperror.NewBadRequest("name the monster to match")
		}
		found := false
		if list != nil {
			for _, m := range list.Monsters {
				if !m.Linked && matchKey(m.Name) == matchKey(name) && m.Pack == pack {
					found, name = true, m.Name
					break
				}
			}
		}
		if !found {
			return ch, apperror.NewBadRequest("Foundry did not report a monster by that name there")
		}
		ch.Name, ch.Pack = name, pack
		kept := ch.NotSame[:0]
		for _, n := range ch.NotSame {
			if matchKey(n) != matchKey(name) {
				kept = append(kept, n)
			}
		}
		ch.NotSame = kept
	case "not_same":
		name := field(a.Name, foundryMonsterNameMax)
		if name == "" {
			return ch, apperror.NewBadRequest("name the monster that is different")
		}
		if matchKey(ch.Name) == matchKey(name) {
			ch.Name, ch.Pack, ch.Use = "", "", ""
		}
		for _, n := range ch.NotSame {
			if matchKey(n) == matchKey(name) {
				return ch, nil
			}
		}
		if len(ch.NotSame) >= foundryMatchNotSameMax {
			return ch, apperror.NewBadRequest(fmt.Sprintf("at most %d monsters can be marked as different", foundryMatchNotSameMax))
		}
		ch.NotSame = append(ch.NotSame, name)
	case "reset":
		ch = FoundryMatchChoice{EntityID: ch.EntityID}
	default:
		return ch, apperror.NewBadRequest(`action must be "use", "match", "not_same" or "reset"`)
	}
	return ch, nil
}

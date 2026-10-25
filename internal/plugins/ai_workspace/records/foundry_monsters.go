package records

import (
	"context"
	"strings"
	"time"
)

// FoundryMonstersAPI is the list of monsters the campaign's Foundry world
// last reported holding (syncapi's foundry_monsters), adapted in app wiring.
// It is Foundry's word about Foundry: Chronicle can't check it, so answers
// say when it was reported.
type FoundryMonstersAPI interface {
	Monsters(ctx context.Context, campaignID string, a Actor) (FoundryMonsterReport, error)
}

// FoundryMonsterReport is the whole list. ReportedAt is nil when the world
// never reported one.
type FoundryMonsterReport struct {
	Monsters   []FoundryMonster
	ReportedAt *time.Time
	// Truncated is true when Foundry had more than it could send.
	Truncated bool
}

// FoundryMonster is one actor Foundry can place: a world actor, or an entry
// of an Actor compendium. Level, Role, Organization and EV are the game
// system's own words, empty when the system has no such field.
type FoundryMonster struct {
	Name string
	// Pack is the compendium's label; empty for a world actor.
	Pack                          string
	Level, Role, Organization, EV string
	// Linked is true for an actor creature sync made from a Chronicle page.
	Linked bool
}

// Where says where Foundry keeps it, for the AI and the review.
func (m FoundryMonster) Where() string {
	switch {
	case m.Linked:
		return "an actor made from its Chronicle page"
	case m.Pack != "":
		return "in the " + m.Pack + " compendium"
	}
	return "an actor in the world"
}

// Find is the monster Foundry would place for a name, preferring a world
// actor (Foundry looks there first) over a compendium entry.
func (r FoundryMonsterReport) Find(name string) *FoundryMonster {
	var hit *FoundryMonster
	for i := range r.Monsters {
		m := &r.Monsters[i]
		if !sameName(m.Name, name) {
			continue
		}
		if m.Pack == "" {
			return m
		}
		if hit == nil {
			hit = m
		}
	}
	return hit
}

// line is the monster as one answer line.
func (m FoundryMonster) line() string {
	parts := []string{m.Name}
	if m.Level != "" {
		parts = append(parts, "level "+m.Level)
	}
	for _, s := range []string{m.Organization, m.Role} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if m.EV != "" {
		parts = append(parts, "EV "+m.EV)
	}
	return strings.Join(parts, " · ") + " (" + m.Where() + ")"
}

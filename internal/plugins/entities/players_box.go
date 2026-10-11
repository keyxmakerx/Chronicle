package entities

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// players_box.go shapes the Characters page's Players box: one row per player
// with the characters they play, the Free to pick strip, and the finder's
// result rows. Kept apart from the templates so the grouping rules are plain,
// testable Go.

// rosterPlayerRow is one player of the box with the roster characters they own.
type rosterPlayerRow struct {
	Name  string
	User  string
	Chars []Entity
}

// rosterPlayerRows lists every Player-role member, plus any higher-role member
// who owns a roster character (a co-DM playing a PC must be able to give it
// back). Members keep the order the campaign lists them in.
func rosterPlayerRows(r *ClaimRoster) []rosterPlayerRow {
	rows := make([]rosterPlayerRow, 0, len(r.Members))
	for _, m := range r.Members {
		var chars []Entity
		for _, c := range r.Characters {
			if c.OwnerUserID != nil && *c.OwnerUserID == m.UserID {
				chars = append(chars, c)
			}
		}
		if m.Role != campaigns.RolePlayer && len(chars) == 0 {
			continue
		}
		rows = append(rows, rosterPlayerRow{Name: rosterMemberLabel(m), User: m.UserID, Chars: chars})
	}
	return rows
}

// rosterFreeCharacters are the roster characters a character-less player may
// pick: marked playable and not owned.
func rosterFreeCharacters(r *ClaimRoster) []Entity {
	var free []Entity
	for _, c := range r.Characters {
		if c.Playable && c.OwnerUserID == nil {
			free = append(free, c)
		}
	}
	return free
}

// memberInitial is the avatar letter for a name.
func memberInitial(name string) string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(name))
	if r == utf8.RuneError {
		return "?"
	}
	return strings.ToUpper(string(r))
}

// rosterHit is one finder result with the tag that says where the character
// stands now.
type rosterHit struct {
	Entity Entity
	Tag    string
}

// rosterHitTag says where a found character stands: free to pick, played by
// someone, or (neither) just its category.
func rosterHitTag(e Entity, ownerNames map[string]string) string {
	if e.OwnerUserID != nil {
		if name := ownerNames[*e.OwnerUserID]; name != "" {
			return "Played by " + name
		}
		return "Played by someone who left"
	}
	if e.Playable {
		return "Free to pick"
	}
	return e.TypeName
}

// rosterHits tags a search result list.
func rosterHits(found []Entity, ownerNames map[string]string) []rosterHit {
	hits := make([]rosterHit, 0, len(found))
	for _, e := range found {
		hits = append(hits, rosterHit{Entity: e, Tag: rosterHitTag(e, ownerNames)})
	}
	return hits
}

// htmxVals encodes the form values an htmx button posts (hx-vals). The
// values are IDs and fixed words; the encoder keeps any quote out of the
// attribute regardless.
func htmxVals(kv map[string]string) string {
	b, err := json.Marshal(kv)
	if err != nil {
		return "{}"
	}
	return string(b)
}

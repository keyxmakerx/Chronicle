package app

import (
	"encoding/json"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// hero_change_fold.go keeps a Director's bell for one hero to one unread row.
// Each save adds to what the row already says instead of adding a row, and the
// row remembers, per field, the value it started from and the newest value, so
// folding never parses the words on the bell. A field that ends where it began
// drops out. The state rides with the row and is never shown or served.

// heroFoldField is one changed field as the row remembers it. A story keeps no
// text at all: only that it was rewritten.
type heroFoldField struct {
	Key   string                  `json:"key"`
	Kind  entities.HeroChangeKind `json:"kind"`
	Label string                  `json:"label"`
	// From and To are the first old value and the newest one, for a pick or a
	// short text.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Was and Now are the names a list held at the start and holds now.
	Was []string `json:"was,omitempty"`
	Now []string `json:"now,omitempty"`
}

type heroFoldState struct {
	Fields []heroFoldField `json:"fields"`
}

// encodeHeroFold is the state a row keeps.
func encodeHeroFold(fields []heroFoldField) string {
	b, err := json.Marshal(heroFoldState{Fields: fields})
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeHeroFold reads a row's kept state; a row with none, or one that can't
// be read, starts over.
func decodeHeroFold(kept string) []heroFoldField {
	if kept == "" {
		return nil
	}
	var st heroFoldState
	if err := json.Unmarshal([]byte(kept), &st); err != nil {
		return nil
	}
	return st.Fields
}

func foldFieldOf(c entities.HeroPickChange) heroFoldField {
	return heroFoldField{Key: c.FieldKey, Kind: c.Kind, Label: c.Label, From: c.From, To: c.To, Was: c.WasNames, Now: c.NowNames}
}

// foldHeroChanges adds a save's changes to what the row kept: a field already
// there keeps its first old value and takes the newest; one that is back where
// it began drops out.
func foldHeroChanges(kept string, changes []entities.HeroPickChange) []heroFoldField {
	fields := decodeHeroFold(kept)
	for _, c := range changes {
		i := -1
		for j := range fields {
			if fields[j].Key == c.FieldKey {
				i = j
				break
			}
		}
		if i < 0 {
			fields = append(fields, foldFieldOf(c))
			continue
		}
		f := &fields[i]
		f.Label, f.Kind = c.Label, c.Kind
		f.To, f.Now = c.To, c.NowNames
		if heroFoldBack(*f) {
			fields = append(fields[:i], fields[i+1:]...)
		}
	}
	return fields
}

// heroFoldBack reports whether a field ends where it started. A story keeps
// no text, so it never does.
func heroFoldBack(f heroFoldField) bool {
	switch f.Kind {
	case entities.HeroChangeStory:
		return false
	case entities.HeroChangeList:
		return sameFoldNames(f.Was, f.Now)
	}
	return strings.EqualFold(strings.TrimSpace(f.From), strings.TrimSpace(f.To))
}

func sameFoldNames(a, b []string) bool {
	set := map[string]bool{}
	for _, n := range a {
		set[strings.ToLower(n)] = true
	}
	for _, n := range b {
		if !set[strings.ToLower(n)] {
			return false
		}
	}
	return len(set) == len(b)
}

// heroChangeDetail words one save's changes, as a row's first line of detail.
func heroChangeDetail(changes []entities.HeroPickChange) string {
	fields := make([]heroFoldField, len(changes))
	for i, c := range changes {
		fields[i] = foldFieldOf(c)
	}
	return heroFoldDetail(fields)
}

// heroFoldDetail names each field, the first few and how many more. A row
// whose every field is back as it was says so.
func heroFoldDetail(fields []heroFoldField) string {
	if len(fields) == 0 {
		return "Back as it was"
	}
	parts := make([]string, 0, heroChangeShown+1)
	for i, f := range fields {
		if i == heroChangeShown {
			parts = append(parts, "and "+pluralMore(len(fields)-heroChangeShown))
			break
		}
		parts = append(parts, heroFoldPart(f))
	}
	return strings.Join(parts, "; ")
}

// heroFoldPart words one field. A story is never quoted.
func heroFoldPart(f heroFoldField) string {
	switch f.Kind {
	case entities.HeroChangeStory:
		return f.Label + " rewritten"
	case entities.HeroChangeList:
		added, removed := nameDiff(f.Was, f.Now)
		var sides []string
		if len(added) > 0 {
			sides = append(sides, "added "+nameSide(added))
		}
		if len(removed) > 0 {
			sides = append(sides, "removed "+nameSide(removed))
		}
		return f.Label + ": " + strings.Join(sides, " and ")
	}
	to := f.To
	if to == "" {
		to = "nothing"
	}
	return f.Label + ": " + f.From + " → " + to
}

// nameDiff is the names in now that were not in was (in now's order) and the
// names in was that are not in now (in was's order), ignoring case.
func nameDiff(was, now []string) (added, removed []string) {
	had, has := map[string]bool{}, map[string]bool{}
	for _, n := range was {
		had[strings.ToLower(n)] = true
	}
	for _, n := range now {
		has[strings.ToLower(n)] = true
		if !had[strings.ToLower(n)] {
			added = append(added, n)
		}
	}
	for _, n := range was {
		if !has[strings.ToLower(n)] {
			removed = append(removed, n)
		}
	}
	return added, removed
}

// nameSide joins the names of one side, the first few and how many more.
func nameSide(names []string) string {
	if len(names) <= heroChangeNames {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:heroChangeNames], ", ") + " and " + pluralMore(len(names)-heroChangeNames)
}

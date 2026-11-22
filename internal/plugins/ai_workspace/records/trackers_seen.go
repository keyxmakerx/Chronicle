package records

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// trackerSeen is the `shown_to` and `may_change` keys of a tracker block,
// resolved to the people they name. They act per placed tracker, never on the
// kind, which only holds the default for new placements.
type trackerSeen struct {
	TrackerSeen
}

// readSeen reads `shown_to` and `may_change`. Names are resolved here, at plan
// time and again inside Apply, so a member who left or was renamed since the
// review is caught before anything is written.
func (k TrackerKind) readSeen(ctx context.Context, campaignID string, a Actor, r Record, d *trackerDraft) error {
	if !r.Has("shown_to") {
		return apperror.NewBadRequest("`may_change` goes with `shown_to`, which lists the players who see the tracker")
	}
	if r.Str("page") == "" && len(r.List("pages")) == 0 {
		return apperror.NewBadRequest("`shown_to` is set per page: name the page with `page` (or `pages`). `players_see` sets the default for trackers put on pages later")
	}
	var shown []string
	if r.Fields["shown_to"] != nil {
		for _, it := range r.List("shown_to") {
			shown = append(shown, strings.TrimSpace(fmt.Sprint(it)))
		}
	}
	var change []string
	for _, it := range r.List("may_change") {
		change = append(change, strings.TrimSpace(fmt.Sprint(it)))
	}
	// A bare `directors` or `everyone` is the setting; a list is always names,
	// so a player who happens to be called Everyone is chosen with a list.
	_, isList := r.Fields["shown_to"].([]any)
	if len(shown) == 1 && !isList {
		switch strings.ToLower(shown[0]) {
		case trackerHidden:
			return seenWithoutPlayers(r, trackerHidden, d)
		case trackerShown:
			return seenWithoutPlayers(r, trackerShown, d)
		}
	}
	if len(shown) == 0 {
		return apperror.NewBadRequest("`shown_to` needs `directors`, `everyone` or at least one player's name")
	}
	if k.Svc == nil {
		return apperror.NewBadRequest("trackers aren't available on this server")
	}
	members, err := k.Svc.Members(ctx, campaignID, a)
	if err != nil {
		return apperror.NewBadRequest("could not read the campaign's players")
	}
	seen := TrackerSeen{Visibility: trackerChosen}
	byID := map[string]int{}
	for _, name := range shown {
		m, err := pickPlayer(members, name, "shown_to")
		if err != nil {
			return err
		}
		if _, dup := byID[m.UserID]; dup {
			continue
		}
		byID[m.UserID] = len(seen.Players)
		seen.Players = append(seen.Players, TrackerPlayer{UserID: m.UserID, Name: m.Name})
	}
	for _, name := range change {
		m, err := pickPlayer(members, name, "may_change")
		if err != nil {
			return err
		}
		i, ok := byID[m.UserID]
		if !ok {
			return badRequestf("%s may change it but is not in `shown_to`; add them there too", quote(m.Name))
		}
		seen.Players[i].CanChange = true
	}
	d.seen = &trackerSeen{seen}
	return nil
}

func seenWithoutPlayers(r Record, vis string, d *trackerDraft) error {
	if r.Has("may_change") {
		return apperror.NewBadRequest("`may_change` names players who may change a tracker shown to chosen players; with `" + vis + "` it has no meaning")
	}
	d.seen = &trackerSeen{TrackerSeen{Visibility: vis}}
	return nil
}

// pickPlayer finds the one member a name means: a display name or the name of
// a claimed hero page, ignoring case. The refusal lists the players who can be
// chosen by display name only, never anything else about them.
func pickPlayer(members []TrackerMember, name, key string) (TrackerMember, error) {
	var hits []TrackerMember
	for _, m := range members {
		if sameName(m.Name, name) || (m.Character != "" && sameName(m.Character, name)) {
			hits = append(hits, m)
		}
	}
	var names []string
	for _, m := range members {
		if !m.Director {
			names = append(names, m.Name)
		}
	}
	sort.Strings(names)
	choose := "no players can be chosen yet"
	if len(names) > 0 {
		choose = "players who can be chosen: " + strings.Join(names, ", ")
	}
	switch len(hits) {
	case 0:
		return TrackerMember{}, badRequestf("`%s` names %s, who is nobody in this campaign (%s)", key, quote(name), choose)
	case 1:
		if hits[0].Director {
			return TrackerMember{}, badRequestf("%s is a Director and already sees every tracker", hits[0].Name)
		}
		return hits[0], nil
	}
	return TrackerMember{}, badRequestf("`%s` names %s, which more than one member answers to (%s)", key, quote(name), choose)
}

// seenTargets are the pages the keys act on: `page` and `pages`, each looked
// up now. A page this paste makes is resolved at Apply.
func (k TrackerKind) seenTargets(ctx context.Context, campaignID string, r Record, d *trackerDraft) error {
	names := []string{}
	for _, it := range r.List("pages") {
		names = append(names, strings.TrimSpace(fmt.Sprint(it)))
	}
	if p := r.Str("page"); p != "" {
		names = append(names, p)
	}
	done := map[string]bool{}
	for _, n := range names {
		slug := entities.Slugify(n)
		if n == "" || done[slug] {
			continue
		}
		done[slug] = true
		if p := k.page(ctx, campaignID, n); p != nil {
			d.seenPages = append(d.seenPages, p)
		} else {
			d.seenMissing = append(d.seenMissing, n)
		}
	}
	return nil
}

// seenDiffers says whether a placed tracker is not already seen as asked.
func seenDiffers(t TrackerOnPage, s TrackerSeen) bool {
	if t.Visibility != s.Visibility {
		return true
	}
	if s.Visibility != trackerChosen {
		return false
	}
	have := map[string]bool{}
	for _, p := range t.Players {
		have[p.UserID] = p.CanChange
	}
	if len(have) != len(s.Players) {
		return true
	}
	for _, p := range s.Players {
		if c, ok := have[p.UserID]; !ok || c != p.CanChange {
			return true
		}
	}
	return false
}

// seenWords says what the keys do, in the plan's plain words.
func seenWords(s TrackerSeen) string {
	switch s.Visibility {
	case trackerHidden:
		return "hidden from players"
	case trackerShown:
		return "shown to everyone"
	}
	var all, may []string
	for _, p := range s.Players {
		all = append(all, p.Name)
		if p.CanChange {
			may = append(may, p.Name)
		}
	}
	out := "shown to " + andList(all)
	if len(may) > 0 {
		out += "; " + andList(may) + " may change it"
	}
	return out
}

func andList(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// seenPart is the plan's words for the keys: what changes on which pages, or
// that nothing does because each placed tracker already matches.
func (k TrackerKind) seenPart(ctx context.Context, campaignID string, a Actor, d trackerDraft) string {
	var changed []string
	for _, pg := range d.seenPages {
		if d.existing != nil {
			if pv, err := k.Svc.Page(ctx, campaignID, pg.ID, a); err == nil {
				if t, ok := findOnPage(pv, d.existing.ID); ok && !seenDiffers(t, d.seen.TrackerSeen) {
					continue
				}
			}
		}
		changed = append(changed, pg.Name)
	}
	changed = append(changed, d.seenMissing...)
	if len(changed) == 0 {
		return "who sees it changes nothing"
	}
	return seenWords(d.seen.TrackerSeen) + " on " + andList(changed)
}

func findOnPage(pv *TrackerPage, kindID string) (TrackerOnPage, bool) {
	for _, t := range pv.Trackers {
		if t.KindID == kindID {
			return t, true
		}
	}
	return TrackerOnPage{}, false
}

// applySeen puts the kind on each target page, hidden first so it is never
// open to more people than asked for, then sets who sees it.
func (k TrackerKind) applySeen(ctx context.Context, campaignID string, a Actor, d trackerDraft, kind *TrackerSetup) error {
	for _, pg := range d.seenPages {
		t, err := k.placed(ctx, campaignID, pg.ID, a, kind, trackerHidden, true)
		if err != nil {
			return err
		}
		if !seenDiffers(t, d.seen.TrackerSeen) {
			continue
		}
		if err := k.Svc.SetSeen(ctx, campaignID, t.ID, a, d.seen.TrackerSeen); err != nil {
			return err
		}
	}
	return nil
}

// seenLine is how the lookup says who sees a placed tracker. Chosen players
// are named, so the lookup stays the Director's.
func seenLine(t TrackerOnPage) string {
	switch t.Visibility {
	case trackerShown:
		return "everyone"
	case trackerChosen:
		if len(t.Players) == 0 {
			return "Director only (no players chosen)"
		}
		parts := make([]string, len(t.Players))
		for i, p := range t.Players {
			parts[i] = p.Name
			if p.CanChange {
				parts[i] += " (may change it)"
			}
		}
		return "shown to " + strings.Join(parts, ", ")
	}
	return "Director only"
}

// seenBlock is one placed tracker's audience, written so it reads back as a
// block that changes nothing. Only chosen players need it: the kind's own
// default covers the other two.
func seenBlock(name, page string, t TrackerOnPage) string {
	if t.Visibility != trackerChosen || len(t.Players) == 0 {
		return ""
	}
	var shown, may []string
	for _, p := range t.Players {
		shown = append(shown, yamlStr(p.Name))
		if p.CanChange {
			may = append(may, yamlStr(p.Name))
		}
	}
	out := "---\nkind: tracker\naction: update\nname: " + yamlStr(name) + "\npage: " + yamlStr(page) + "\nshown_to: [" + strings.Join(shown, ", ") + "]\n"
	if len(may) > 0 {
		out += "may_change: [" + strings.Join(may, ", ") + "]\n"
	}
	return out + "---\n\n"
}

package systems

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MarkView is one stored mark as the Directors' screen shows it.
type MarkView struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// PlayerMark is a mark for one player.
type PlayerMark struct {
	UserID     string `json:"userId"`
	PlayerName string `json:"playerName"`
	MarkView
}

// DirectorGroup is the row that marks a whole source at once.
type DirectorGroup struct {
	Source string `json:"source"`
	Key    string `json:"key"`
	Label  string `json:"label"`
	// Everyone is the mark for everyone, nil when there is none.
	Everyone *MarkView    `json:"everyone"`
	Players  []PlayerMark `json:"players"`
}

// DirectorEntry is one entry of the list with its marks.
type DirectorEntry struct {
	Key     string `json:"key"`
	Source  string `json:"source"`
	Slug    string `json:"slug,omitempty"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	// EntryID, Visibility, Status and ForName describe a campaign entry.
	EntryID    int64  `json:"entryId,omitempty"`
	Visibility string `json:"visibility,omitempty"`
	Status     string `json:"status,omitempty"`
	ForName    string `json:"forName,omitempty"`
	// Everyone and Players are the marks stored on this entry itself, or for a
	// rewrite with none of its own, on the standard entry it replaces; a mark
	// sent on the rewrite reaches those rows (followRewrite).
	Everyone *MarkView    `json:"everyone"`
	Players  []PlayerMark `json:"players"`
	// State is what the entry is for the viewed audience (everyone, or the
	// player in "as"): open, ask_first, not_offered, required, or left_out
	// when a required short list does not include it.
	State string `json:"state"`
	// Inherited says State came from a group row rather than the entry's own.
	Inherited bool `json:"inherited,omitempty"`
	// Reason is the reason of the mark that decided State.
	Reason string `json:"reason,omitempty"`
	// RuleCount is how many rules (any audience) name this entry.
	RuleCount int `json:"ruleCount"`
}

// DirectorEntryList is a pick list's every entry with its marks, for the
// Directors' screen.
type DirectorEntryList struct {
	FieldKey   string `json:"fieldKey"`
	SystemID   string `json:"systemId"`
	SystemName string `json:"systemName"`
	// As is the player the list is shown as, "" for everyone.
	As       string          `json:"as,omitempty"`
	Required bool            `json:"required,omitempty"`
	Groups   []DirectorGroup `json:"groups"`
	Entries  []DirectorEntry `json:"entries"`
	Members  []ChoiceMember  `json:"members"`
	// Orphans are package-entry rules whose entry is no longer in the
	// package: shown as "no longer in <system>", and inert.
	Orphans []ChoiceRule `json:"orphans"`
	// Warnings are plain sentences about the list, such as two entries that
	// share a name.
	Warnings []string `json:"warnings"`
}

func markView(r ChoiceRule) *MarkView { return &MarkView{State: r.State, Reason: r.Reason} }

func (s *choiceRuleService) DirectorEntries(ctx context.Context, campaignID string, actor EntryActor, fieldKey, asUser string) (*DirectorEntryList, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	m, err := s.system(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if err := s.checkField(m, fieldKey); err != nil {
		return nil, err
	}
	asUser = strings.TrimSpace(asUser)
	if len(asUser) > 36 {
		return nil, apperror.NewValidation("invalid player")
	}
	all, err := s.directorList(ctx, campaignID, fieldKey)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, campaignID, m.ID, fieldKey)
	if err != nil {
		return nil, err
	}
	members := s.members(ctx, campaignID)
	names := memberNames(members)

	// The entries the audience can read: everything for Directors' "everyone"
	// view of the list, only what that player sees when shown as them.
	audience := all.Choices
	if asUser != "" {
		pl, err := s.playerList(ctx, campaignID, fieldKey, asUser)
		if err != nil {
			return nil, err
		}
		readable := map[ruleRef]bool{}
		for _, c := range pl.Choices {
			readable[ruleRef{source: c.Source, key: c.Key}] = true
		}
		audience = nil
		for _, c := range all.Choices {
			if readable[ruleRef{source: c.Source, key: c.Key}] {
				audience = append(audience, c)
			}
		}
	}
	ix := indexRules(rows, asUser)
	classes, required := classifyChoices(audience, ix, asUser)

	// Every stored row by entry, split into the mark for everyone and the
	// marks for single players.
	type marks struct {
		everyone *MarkView
		players  []PlayerMark
		count    int
	}
	byEntry := map[ruleRef]*marks{}
	for _, r := range rows {
		ref := ruleRef{source: r.Source, key: r.Key}
		mk := byEntry[ref]
		if mk == nil {
			mk = &marks{}
			byEntry[ref] = mk
		}
		mk.count++
		if r.Target == "" {
			mk.everyone = markView(r)
		} else {
			mk.players = append(mk.players, PlayerMark{UserID: r.Target, PlayerName: names[r.Target], MarkView: *markView(r)})
		}
	}
	for _, mk := range byEntry {
		sort.Slice(mk.players, func(i, j int) bool { return mk.players[i].PlayerName < mk.players[j].PlayerName })
	}

	out := &DirectorEntryList{
		FieldKey: fieldKey, SystemID: m.ID, SystemName: all.SystemName, As: asUser, Required: required,
		Groups: []DirectorGroup{}, Entries: []DirectorEntry{}, Members: members,
		Orphans: []ChoiceRule{}, Warnings: []string{},
	}
	if out.Members == nil {
		out.Members = []ChoiceMember{}
	}
	for _, g := range []struct{ source, label string }{
		{ChoiceSourceCampaign, "This campaign's own"},
		{ChoiceSourcePackage, "Everything from " + all.SystemName},
	} {
		dg := DirectorGroup{Source: g.source, Key: RuleGroupKey, Label: g.label, Players: []PlayerMark{}}
		if mk := byEntry[ruleRef{source: g.source, key: RuleGroupKey}]; mk != nil {
			dg.Everyone = mk.everyone
			if mk.players != nil {
				dg.Players = mk.players
			}
		}
		out.Groups = append(out.Groups, dg)
	}

	present := map[ruleRef]bool{}
	for _, c := range all.Choices {
		present[ruleRef{source: c.Source, key: c.Key}] = true
		// Rules on a standard entry follow its rewrite, so they aren't orphans.
		if c.Origin == OriginRewritten && c.StandsFor != "" {
			present[ruleRef{source: ChoiceSourcePackage, key: c.StandsFor}] = true
		}
	}
	for i, c := range audience {
		cl := classes[i]
		e := DirectorEntry{
			Key: c.Key, Source: c.Source, Slug: c.Slug, Name: c.Name, Summary: c.Summary,
			EntryID: c.EntryID, Visibility: c.Visibility, Status: c.Status, ForName: c.ForName,
			Players: []PlayerMark{}, State: cl.State,
		}
		if cl.Rule != nil {
			e.Reason = cl.Rule.Reason
			e.Inherited = cl.Rule.IsGroup()
		}
		if !cl.listed && cl.State != RuleNotOffered {
			e.State = "left_out"
		}
		mk := byEntry[ruleRef{source: c.Source, key: c.Key}]
		if mk == nil && c.Origin == OriginRewritten && c.StandsFor != "" {
			mk = byEntry[ruleRef{source: ChoiceSourcePackage, key: c.StandsFor}]
		}
		if mk != nil {
			e.Everyone, e.RuleCount = mk.everyone, mk.count
			if mk.players != nil {
				e.Players = mk.players
			}
		}
		if c.Source == ChoiceSourceCampaign {
			e.EntryID = entryIDOf(c)
		}
		out.Entries = append(out.Entries, e)
	}
	for _, r := range rows {
		if !r.IsGroup() && !present[ruleRef{source: r.Source, key: r.Key}] {
			out.Orphans = append(out.Orphans, r)
		}
	}
	out.Warnings = nameClashWarnings(all.Choices)
	return out, nil
}

// entryIDOf is a campaign choice's entry id, from its key.
func entryIDOf(c Choice) int64 {
	var id int64
	if _, err := fmt.Sscan(c.Key, &id); err != nil {
		return 0
	}
	return id
}

// nameClashWarnings says so when a campaign entry shares a name with a game
// entry: heroes keep the name, and a mark on one does not reach the other.
func nameClashWarnings(choices []Choice) []string {
	pkg := map[string]string{}
	for _, c := range choices {
		if c.Source == ChoiceSourcePackage {
			pkg[strings.ToLower(strings.TrimSpace(c.Name))] = c.Name
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, c := range choices {
		low := strings.ToLower(strings.TrimSpace(c.Name))
		if c.Source != ChoiceSourceCampaign || seen[low] {
			continue
		}
		if name, ok := pkg[low]; ok {
			seen[low] = true
			out = append(out, fmt.Sprintf("This campaign has its own %s and the game has one too. A mark on one doesn't reach the other.", name))
		}
	}
	return out
}

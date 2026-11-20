package systems

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// "Before you play in <campaign>" is one page every member can read,
// compiled by nobody: it gathers, across every kind, what the campaign wrote
// of its own, what it rewrote and what it left out, each as the reader may
// see it. It is built from the same per-reader catalog as every list, so it
// never shows a reader an entry the lists would not. The Director also gets
// their amber words: a new entry, the line that says the package changed an
// entry they rewrote, old names to free, and the way to players' drafts.

// The three runs, in the order the page draws them.
const (
	RunOnly    = "only"
	RunChanged = "changed"
	RunNotIn   = "not-in"
)

// beforeRow is one entry on the gathered page.
type beforeRow struct {
	Name string
	URL  string
	// Kind ends the row in words ("Ancestry").
	Kind string
	Line string
	// Quoted is true when Line is the Director's sentence on a left-out entry.
	Quoted        bool
	DirectorsOnly bool
	// Change is the Director's line when the package changed the standard
	// entry this one rewrote.
	Change *changeLine
}

// changeLine is "Draw Steel changed its Dwarf since you wrote yours: ...".
type changeLine struct {
	Text       string
	EntryID    int64
	CompareURL string
	// AsideURL opens the entry's page on the "Set aside" question; "" when
	// players already read the printed one.
	AsideURL   string
	AsideLabel string
}

// beforeRun is one run of rows under its heading.
type beforeRun struct {
	ID    string
	Title string
	Rows  []beforeRow
}

// kindLink is one kind the Director's "New entry" can start.
type kindLink struct {
	Label string
	URL   string
}

// oldNameRow is an old name that still leads to an entry.
type oldNameRow struct {
	ID      int64
	Name    string
	Leads   string
	LeadURL string
	// AfterFree says, with the real names, what heroes still carrying the
	// name read once it is freed.
	AfterFree string
}

// BeforeView is the gathered page as one reader has it.
type BeforeView struct {
	CampaignID   string
	CampaignName string
	SystemID     string
	SystemName   string
	URL          string
	// Runs holds only the runs that have rows.
	Runs       []beforeRun
	IsDirector bool
	// For the Director: kinds a new entry can be, old names, and the editor's
	// page with how many players' drafts wait there.
	NewKinds  []kindLink
	OldNames  []oldNameRow
	DraftsURL string
	Waiting   int
	APIBase   string
}

// Empty reports whether no run has a row.
func (v *BeforeView) Empty() bool { return len(v.Runs) == 0 }

// BeforeURL is the gathered page's address.
func BeforeURL(campaignID, systemID string) string {
	return "/campaigns/" + url.PathEscape(campaignID) + "/systems/" + url.PathEscape(systemID) + "/before-you-play"
}

// kindCategories are the rules categories that are kinds, in the manifest's
// order, with their character field.
func kindCategories(m *SystemManifest) []CategoryDef {
	var out []CategoryDef
	for _, c := range m.Categories {
		if kindField(m, c.Slug) != "" {
			out = append(out, c)
		}
	}
	return out
}

// Before builds the gathered page. Only the campaign's own system has one.
func (p *RulesPages) Before(ctx context.Context, sys System, r RulesReader) (*BeforeView, error) {
	m := sys.Info()
	if !p.overlay(ctx, sys, r.CampaignID) {
		return nil, apperror.NewNotFound("this game system is not the campaign's own")
	}
	v := &BeforeView{CampaignID: r.CampaignID, CampaignName: r.CampaignName, SystemID: m.ID, SystemName: m.Name,
		URL: BeforeURL(r.CampaignID, m.ID), IsDirector: r.Actor.IsDirector, APIBase: entryAPIBase(r.CampaignID)}
	only := beforeRun{ID: RunOnly, Title: "Only in " + r.CampaignName}
	changed := beforeRun{ID: RunChanged, Title: "Changed in " + r.CampaignName}
	notIn := beforeRun{ID: RunNotIn, Title: "Not in " + r.CampaignName}
	byEntry := map[int64]beforeRow{}
	firstField := ""
	// For the old names: each kind's category and its left-out names.
	catOf := map[string]string{}
	leftOutOf := map[string]map[string]bool{}
	for _, cat := range kindCategories(m) {
		if hiddenCategory(m, cat.Slug, r) {
			continue
		}
		l, err := p.catalog.Catalog(ctx, r.CampaignID, cat.Slug, r.Actor)
		if err != nil {
			return nil, err
		}
		if l.SystemName != "" {
			v.SystemName = l.SystemName
		}
		field := kindField(m, cat.Slug)
		if firstField == "" {
			firstField = field
		}
		catOf[field] = cat.Slug
		outNames := map[string]bool{}
		for _, e := range l.LeftOut {
			outNames[strings.ToLower(strings.TrimSpace(e.Name))] = true
		}
		leftOutOf[field] = outNames
		kind := kindWord(&cat, field)
		if r.Actor.IsDirector {
			v.NewKinds = append(v.NewKinds, kindLink{Label: kind, URL: newEntryURL(r.CampaignID, field, v.URL)})
		}
		for _, e := range l.Entries {
			if e.Status == EntryStatusSent && r.Actor.IsDirector {
				v.Waiting++
			}
			// A player's draft is reviewed in the editor, not shown here.
			if e.Origin == OriginStandard || e.Status != "" {
				continue
			}
			row := beforeRow{Name: e.Name, URL: EntryURL(r.CampaignID, m.ID, cat.Slug, e.Slug, false), Kind: kind,
				Line: cleanSummary(e.Summary), DirectorsOnly: e.DirectorsOnly && r.Actor.IsDirector}
			if e.Origin == OriginRewritten {
				if r.Actor.IsDirector {
					ch, err := p.changeLine(ctx, sys, cat.Slug, e, v.SystemName, r)
					if err != nil {
						return nil, err
					}
					row.Change = ch
				}
				changed.Rows = append(changed.Rows, row)
			} else {
				only.Rows = append(only.Rows, row)
			}
			byEntry[e.EntryID] = row
		}
		// Every reader's lists end with the left-out entries that have a
		// sentence; the Director also sees the rest, in amber.
		outs := l.LeftOutNoted
		if r.Actor.IsDirector {
			outs = l.LeftOut
		}
		for _, e := range outs {
			row := beforeRow{Name: e.Name, URL: EntryURL(r.CampaignID, m.ID, cat.Slug, e.Slug, false), Kind: kind,
				Line: e.Sentence, Quoted: e.Sentence != "", DirectorsOnly: e.Sentence == "" && r.Actor.IsDirector}
			notIn.Rows = append(notIn.Rows, row)
		}
	}
	for _, run := range []beforeRun{only, changed, notIn} {
		if len(run.Rows) == 0 {
			continue
		}
		sort.SliceStable(run.Rows, func(a, b int) bool {
			return strings.ToLower(run.Rows[a].Name) < strings.ToLower(run.Rows[b].Name)
		})
		v.Runs = append(v.Runs, run)
	}
	if r.Actor.IsDirector {
		v.DraftsURL = "/campaigns/" + url.PathEscape(r.CampaignID) + "/characters/entries?field=" + url.QueryEscape(firstField)
		names, err := p.catalog.OldNames(ctx, r.CampaignID, "", r.Actor)
		if err != nil {
			return nil, err
		}
		pkgNames := map[string]map[string]string{}
		for _, n := range names {
			row := oldNameRow{ID: n.ID, Name: n.Name}
			if to, ok := byEntry[n.EntryID]; ok {
				row.Leads, row.LeadURL = to.Name, to.URL
			}
			std, err := packageEntryNamed(sys, catOf[n.FieldKey], n.Name, pkgNames)
			if err != nil {
				return nil, err
			}
			row.AfterFree = afterFreeLine(n.Name, std, leftOutOf[n.FieldKey][strings.ToLower(std)], v.SystemName, r.CampaignName)
			v.OldNames = append(v.OldNames, row)
		}
	}
	return v, nil
}

// packageEntryNamed is the name of the game system's own entry in category
// that a freed name falls back to, or "" when the system has none. seen caches
// each category's names, lowercased to the printed name.
func packageEntryNamed(sys System, category, name string, seen map[string]map[string]string) (string, error) {
	dp := sys.DataProvider()
	if category == "" || dp == nil {
		return "", nil
	}
	names, ok := seen[category]
	if !ok {
		items, err := dp.List(category)
		if err != nil {
			return "", err
		}
		names = map[string]string{}
		for _, it := range items {
			names[strings.ToLower(strings.TrimSpace(it.Name))] = strings.TrimSpace(it.Name)
		}
		seen[category] = names
	}
	return names[strings.ToLower(strings.TrimSpace(name))], nil
}

// afterFreeLine tells the Director what heroes still called name read once
// it is freed: the system's own entry of that name, its left-out line, or
// nothing until they are renamed.
func afterFreeLine(name, std string, stdLeftOut bool, systemName, campaignName string) string {
	switch {
	case std == "":
		return "Heroes still called " + name + " will have no entry to read until you rename them."
	case stdLeftOut:
		return "Heroes still called " + name + " will read that " + std + " is not in " + campaignName + "."
	}
	return "Heroes still called " + name + " will read " + systemName + "’s " + std + " again."
}

// changeLine is the Director's notice that the package changed the standard
// entry since the rewrite was made or last marked seen; nil when it did not.
func (p *RulesPages) changeLine(ctx context.Context, sys System, category string, e CatalogEntry, systemName string, r RulesReader) (*changeLine, error) {
	what, err := p.catalog.StandardChange(ctx, r.CampaignID, r.Actor, e.EntryID)
	if err != nil || what == "" {
		return nil, err
	}
	printed := e.StandsForName
	if printed == "" {
		printed = e.Name
	}
	ch := &changeLine{EntryID: e.EntryID,
		Text:       systemName + " changed its " + printed + " since you wrote yours: " + what,
		CompareURL: EntryURL(r.CampaignID, sys.Info().ID, category, e.StandsFor, true)}
	if !e.DirectorsOnly {
		ch.AsideURL = EntryURL(r.CampaignID, sys.Info().ID, category, e.Slug, false) + "#set-aside"
		ch.AsideLabel = "Set " + r.CampaignName + "’s " + e.Name + " aside"
	}
	return ch, nil
}

// newEntryURL opens the entry editor on a new entry of a kind.
func newEntryURL(campaignID, fieldKey, back string) string {
	q := url.Values{}
	q.Set("field", fieldKey)
	q.Set("new", "1")
	q.Set("return", back)
	return "/campaigns/" + url.PathEscape(campaignID) + "/characters/entries?" + q.Encode()
}

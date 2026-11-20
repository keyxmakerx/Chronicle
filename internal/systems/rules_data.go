package systems

import (
	"context"
	"encoding/json"
)

// A kind's raw data file (ancestries.json) is read by widgets that look an
// entry up by the name a hero stores, such as a sheet pricing the traits a
// hero keeps. Served verbatim, a player would get the printed entry the
// campaign rewrote and a sheet would price Therin's Human by the printed one.
// So for the campaign's own system a kind's file is answered from the
// reader's catalog in the file's own shape: the package's objects untouched
// for the entries this reader reads as printed, the campaign's entries in
// their place, and the left-out entries still present (marked leftOut), since
// a hero who already is one keeps its traits. It is a look-up file, not a list
// anyone picks from; pick lists come from the character-choices route.

// kindDataEntry is a campaign entry in a data file's shape. The optional
// fields beyond a package item's say whose words these are.
type kindDataEntry struct {
	Slug          string         `json:"slug"`
	Name          string         `json:"name"`
	Summary       string         `json:"summary"`
	Description   string         `json:"description,omitempty"`
	Properties    map[string]any `json:"properties"`
	Tags          []string       `json:"tags,omitempty"`
	Source        string         `json:"source"`
	Origin        string         `json:"origin"`
	StandsFor     string         `json:"standsFor,omitempty"`
	DirectorsOnly bool           `json:"directorsOnly,omitempty"`
	ToldRecently  bool           `json:"toldRecently,omitempty"`
}

// KindData answers a data file for one reader. handled is false when the file
// is not a kind of the campaign's own system, and the caller serves it as is.
func (p *RulesPages) KindData(ctx context.Context, sys System, category string, raw []byte, r RulesReader) (out []byte, handled bool, err error) {
	if !p.overlay(ctx, sys, r.CampaignID) || kindField(sys.Info(), category) == "" {
		return nil, false, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		// Not a list of entries: nothing to overlay, so leave it alone.
		return nil, false, nil
	}
	bySlug := map[string]map[string]json.RawMessage{}
	for _, it := range items {
		var slug string
		if json.Unmarshal(it["slug"], &slug) == nil && slug != "" {
			if _, dup := bySlug[slug]; !dup {
				bySlug[slug] = it
			}
		}
	}
	list, err := p.catalog.Catalog(ctx, r.CampaignID, category, r.Actor)
	if err != nil {
		return nil, false, err
	}
	res := make([]any, 0, len(list.Entries))
	// with copies a package object and adds marks to it, never to the shared one.
	with := func(it map[string]json.RawMessage, marks map[string]any) map[string]json.RawMessage {
		cp := make(map[string]json.RawMessage, len(it)+len(marks))
		for k, v := range it {
			cp[k] = v
		}
		for k, v := range marks {
			b, _ := json.Marshal(v)
			cp[k] = b
		}
		return cp
	}
	for _, e := range list.Entries {
		if e.Origin == OriginStandard {
			it, ok := bySlug[e.Slug]
			if !ok {
				continue
			}
			if e.Printed {
				it = with(it, map[string]any{"printed": true})
			}
			res = append(res, it)
			continue
		}
		props := withTwins(e.Properties, findCategory(sys.Info(), category))
		res = append(res, kindDataEntry{Slug: e.Slug, Name: e.Name, Summary: e.Summary, Description: e.Description,
			Properties: props, Tags: e.Tags, Source: r.CampaignName, Origin: e.Origin, StandsFor: e.StandsFor,
			DirectorsOnly: e.DirectorsOnly, ToldRecently: e.ToldRecently})
	}
	outs, err := p.catalog.LeftOutList(ctx, r.CampaignID, list.FieldKey)
	if err != nil {
		return nil, false, err
	}
	for _, l := range outs {
		if list.Replaced(l.Slug) {
			// Left out and rewritten at once (a race the service refuses):
			// the reader has the rewrite, never the printed words beside it.
			continue
		}
		if it, ok := bySlug[l.Slug]; ok {
			marks := map[string]any{"leftOut": true}
			if l.Sentence != "" {
				marks["sentence"] = l.Sentence
			}
			res = append(res, with(it, marks))
		}
	}
	out, err = json.Marshal(res)
	return out, err == nil, err
}

// withTwins copies a campaign entry's properties and adds the generated
// twins the manifest's fields name, so a widget that prints a twin, as it
// would a package object's, finds the same text the entry page shows.
func withTwins(props map[string]any, def *CategoryDef) map[string]any {
	out := make(map[string]any, len(props)+4)
	for k, v := range props {
		out[k] = v
	}
	if def == nil {
		return out
	}
	for _, f := range def.Fields {
		if t, ok := twinText(props, f.Key); ok && t != "" {
			out[f.Key] = t
		}
	}
	return out
}

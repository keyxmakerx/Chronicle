package app

// decks_facts.go answers what a card's tests are judged on. The decks plugin
// owns the Facts interface; this adapter reads it from the entities, tags,
// trackers and transformations services so decks never imports another
// plugin.
//
// Everything is read as a Director would read it: a test may name a Director-
// only tag, a hidden tracker or a layer no player can see. Who is then told
// anything about the result is decided in the decks plugin, never here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// errDecksNoPage is a page that is gone or belongs to another campaign. It is
// quiet like an unavailable plugin: a deleted page is an ordinary state, not a
// fault worth a log line after every save.
var errDecksNoPage = fmt.Errorf("%w: the page is not in this campaign", decks.ErrUnavailable)

// decksCharacterTypes names the categories whose pages are the party's.
type decksCharacterTypes interface {
	CharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// decksFactsAdapter implements decks.Facts. trackers and layers are bound after
// they are built and stay nil while their plugin is degraded, which reads as
// "not available" and never as an empty answer.
type decksFactsAdapter struct {
	entities   entities.EntityService
	characters decksCharacterTypes
	tags       tags.TagService
	trackers   trackers.Service
	layers     transformations.Service
}

var directorTrackers = trackers.Viewer{MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner, Director: true}
var directorLayers = transformations.Viewer{VisibilityRole: permissions.RoleOwner}

// page finds a live page of this campaign.
func (a *decksFactsAdapter) page(ctx context.Context, campaignID, pageID string) (*entities.Entity, error) {
	e, err := a.entities.GetByID(ctx, pageID)
	if err != nil {
		if isNotFound(err) {
			return nil, errDecksNoPage
		}
		return nil, err
	}
	if e == nil || e.CampaignID != campaignID {
		return nil, errDecksNoPage
	}
	return e, nil
}

// numberFields is a page's number fields: its category's, less those it hides,
// with its own added. A field the page re-typed away from number is not one.
func numberFields(defs []entities.FieldDefinition, ov *entities.FieldOverrides) []decks.NumberField {
	hidden := map[string]bool{}
	retyped := map[string]string{}
	var added []entities.FieldDefinition
	if ov != nil {
		for _, k := range ov.Hidden {
			hidden[k] = true
		}
		for k, m := range ov.Modified {
			if m.Type != nil {
				retyped[k] = *m.Type
			}
		}
		added = ov.Added
	}
	var out []decks.NumberField
	seen := map[string]bool{}
	add := func(d entities.FieldDefinition) {
		typ := d.Type
		if t, ok := retyped[d.Key]; ok {
			typ = t
		}
		if typ != "number" || hidden[d.Key] || seen[d.Key] || d.Key == "" {
			return
		}
		seen[d.Key] = true
		f := decks.NumberField{Key: d.Key, Label: d.Label}
		if d.Play != nil {
			f.MaxKey = d.Play.MaxField
		}
		out = append(out, f)
	}
	for _, d := range defs {
		add(d)
	}
	for _, d := range added {
		add(d)
	}
	return out
}

func (a *decksFactsAdapter) fieldsOf(ctx context.Context, e *entities.Entity) ([]decks.NumberField, error) {
	et, err := a.entities.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil {
		return nil, err
	}
	var defs []entities.FieldDefinition
	if et != nil {
		defs = et.Fields
	}
	return numberFields(defs, e.FieldOverrides), nil
}

func (a *decksFactsAdapter) Fields(ctx context.Context, campaignID, pageID string) ([]decks.NumberField, error) {
	e, err := a.page(ctx, campaignID, pageID)
	if err != nil {
		return nil, err
	}
	return a.fieldsOf(ctx, e)
}

// storedNumber reads a stored sheet value as a number. A blank or non-numeric
// value is not a number, so a test on it reads as missing.
func storedNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

// Numbers reads the stored sheet values and applies the page's layers in order.
// A layer's "set" replaces and "add" adds to a value that exists; an "add" on a
// value that is not there stays missing, because there is nothing to add to.
// List edits ("append", "remove") are not numbers and are ignored.
func (a *decksFactsAdapter) Numbers(ctx context.Context, campaignID, pageID string) (decks.NumberSet, error) {
	e, err := a.page(ctx, campaignID, pageID)
	if err != nil {
		return decks.NumberSet{}, err
	}
	fields, err := a.fieldsOf(ctx, e)
	if err != nil {
		return decks.NumberSet{}, err
	}
	set := decks.NumberSet{Sheet: len(e.FieldsData) > 0, Values: map[string]float64{}, Sources: map[string]string{}, ChangedAt: e.UpdatedAt}
	isNumber := map[string]bool{}
	for _, f := range fields {
		isNumber[f.Key] = true
		if v, ok := e.FieldsData[f.Key]; ok {
			if n, ok := storedNumber(v); ok {
				set.Values[f.Key] = n
			}
		}
	}
	if a.layers == nil {
		// Without the plugin the layers cannot be read; a number a layer might
		// have changed is not one to judge on.
		return decks.NumberSet{}, decks.ErrUnavailable
	}
	pl, err := a.layers.ListForPage(ctx, campaignID, pageID, directorLayers)
	if err != nil {
		return decks.NumberSet{}, err
	}
	for _, l := range pl.Layers {
		for _, c := range l.Changes {
			if !isNumber[c.Field] {
				continue
			}
			var n float64
			if json.Unmarshal(c.Value, &n) != nil {
				continue
			}
			switch c.Op {
			case transformations.OpSet:
				set.Values[c.Field] = n
				set.Sources[c.Field] = "layer"
			case transformations.OpAdd:
				if base, ok := set.Values[c.Field]; ok {
					set.Values[c.Field] = base + n
					set.Sources[c.Field] = "layer"
				}
			}
		}
	}
	return set, nil
}

func (a *decksFactsAdapter) Trackers(ctx context.Context, campaignID, pageID string) ([]decks.TrackerFact, error) {
	if a.trackers == nil {
		return nil, decks.ErrUnavailable
	}
	if _, err := a.page(ctx, campaignID, pageID); err != nil {
		return nil, err
	}
	pv, err := a.trackers.Page(ctx, campaignID, pageID, directorTrackers)
	if err != nil {
		return nil, err
	}
	out := make([]decks.TrackerFact, 0, len(pv.Trackers))
	for _, t := range pv.Trackers {
		out = append(out, decks.TrackerFact{KindID: t.KindID, Name: t.Name, Value: t.Value})
	}
	return out, nil
}

func (a *decksFactsAdapter) Tags(ctx context.Context, campaignID, pageID string) ([]decks.TagFact, error) {
	if a.tags == nil {
		return nil, decks.ErrUnavailable
	}
	if _, err := a.page(ctx, campaignID, pageID); err != nil {
		return nil, err
	}
	list, err := a.tags.GetEntityTags(ctx, pageID, true)
	if err != nil {
		return nil, err
	}
	out := make([]decks.TagFact, 0, len(list))
	for _, t := range list {
		out = append(out, decks.TagFact{ID: t.ID, Name: t.Name})
	}
	return out, nil
}

func (a *decksFactsAdapter) Layers(ctx context.Context, campaignID, pageID string) ([]decks.LayerFact, error) {
	if a.layers == nil {
		return nil, decks.ErrUnavailable
	}
	if _, err := a.page(ctx, campaignID, pageID); err != nil {
		return nil, err
	}
	pl, err := a.layers.ListForPage(ctx, campaignID, pageID, directorLayers)
	if err != nil {
		return nil, err
	}
	out := make([]decks.LayerFact, 0, len(pl.Layers))
	for _, l := range pl.Layers {
		out = append(out, decks.LayerFact{Name: l.Name, Source: l.Source})
	}
	return out, nil
}

// Options is what the editor may offer: the party, the number fields of the
// party's category, and the tracker kinds and tags that exist. A part whose
// plugin is not running is left empty rather than failing the whole list.
func (a *decksFactsAdapter) Options(ctx context.Context, campaignID string) (decks.WhenOptions, error) {
	out := decks.WhenOptions{Heroes: []decks.OptionHero{}, Fields: []decks.NumberField{}, Trackers: []decks.TrackerKindOption{}, Tags: []decks.TagOption{}}
	party, err := a.entities.ListClaimed(ctx, campaignID, permissions.RoleOwner, "")
	if err != nil {
		return out, err
	}
	for _, e := range party {
		out.Heroes = append(out.Heroes, decks.OptionHero{ID: e.ID, Name: e.Name})
	}
	if a.characters != nil {
		ids, err := a.characters.CharacterTypeIDs(ctx, campaignID)
		if err != nil && !errors.Is(err, context.Canceled) {
			return out, err
		}
		seen := map[string]bool{}
		for _, id := range ids {
			et, err := a.entities.GetEntityTypeByID(ctx, id)
			if err != nil || et == nil {
				continue
			}
			for _, f := range numberFields(et.Fields, nil) {
				if !seen[f.Key] {
					seen[f.Key] = true
					out.Fields = append(out.Fields, f)
				}
			}
		}
		sort.SliceStable(out.Fields, func(i, j int) bool { return out.Fields[i].Label < out.Fields[j].Label })
	}
	if a.trackers != nil {
		kinds, _, err := a.trackers.Kinds(ctx, campaignID, directorTrackers)
		if err != nil {
			return out, err
		}
		for _, k := range kinds {
			out.Trackers = append(out.Trackers, decks.TrackerKindOption{ID: k.ID, Name: k.Name})
		}
	}
	if a.tags != nil {
		list, err := a.tags.ListByCampaign(ctx, campaignID, true)
		if err != nil {
			return out, err
		}
		for _, t := range list {
			out.Tags = append(out.Tags, decks.TagOption{ID: t.ID, Name: t.Name})
		}
	}
	return out, nil
}

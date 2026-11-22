package entities

// field_switch.go is the one-time switch a game system offers for a category
// the campaign already has: Draw Steel's treasure fields for the Items every
// campaign starts with. Nothing changes until the owner accepts. Accepting
// puts the system's fields on the category, takes the fields the system names
// off every page and keeps what they held aside, GM-only; switching back
// reverses it. Nothing is deleted either way.
//
// This file is the pure part: the states, the offer, and how the field list
// and each page's values change. field_switch_service.go runs it.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Field switch states, one row per answered category. No row: not answered.
const (
	FieldSwitchSwitched  = "switched"
	FieldSwitchDismissed = "dismissed"
	FieldSwitchBack      = "back"
)

// FieldSwitch is a category's answer to a system's switch offer.
type FieldSwitch struct {
	EntityTypeID int
	CampaignID   string
	SystemID     string
	PresetSlug   string
	State        string
	// OldFields is the category's field list from before the switch, nil
	// when the category was made with the system's fields to begin with.
	OldFields []FieldDefinition
	// OldPresetCategory is the category's preset category from before.
	OldPresetCategory *string
	UpdatedBy         string
	UpdatedAt         time.Time
}

// FieldSwitchOffer is what a game system offers a category. The app layer
// builds it from the enabled system's manifest, so this plugin never reads a
// manifest itself.
type FieldSwitchOffer struct {
	SystemID   string
	SystemName string
	PresetSlug string
	// Category is the preset category the switched category takes.
	Category string
	// Label names the fields in the offer ("treasure fields").
	Label string
	// Summary says what the switch adds in a few words; "" lists the labels.
	Summary string
	// Fields are the system's field definitions, in its order.
	Fields []FieldDefinition
	// SetAside are the category's own keys the switch takes off every page.
	SetAside []string
	// Reuse are keys both lists have whose old values may not fit the new
	// field; such a value is set aside too.
	Reuse []string
}

// FieldSwitchView is what the owner sees of an offer: on the category's list
// (only when unanswered) and in the category's Fields settings (always).
type FieldSwitchView struct {
	Offer FieldSwitchOffer
	// State is the stored answer, or "" when the owner has not answered.
	State string
	// Adds names the fields the switch would add or replace, in order.
	Adds []string
	// Removes names the fields on this category the switch takes off.
	Removes []string
	// CanSwitchBack is true when a switched category has a list to go back to.
	CanSwitchBack bool
}

// Question is the offer as the owner reads it.
func (v *FieldSwitchView) Question(categoryPlural string) string {
	return fmt.Sprintf("Use %s’s %s for %s?", v.Offer.SystemName, v.Offer.Label, categoryPlural)
}

// SetAsideValue is one value a switch took off a page, as the GM's fold shows it.
type SetAsideValue struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// SwitchTarget is the category a preset of the given category takes over: a
// type already bound to that preset category, else the type every campaign
// starts with under that slug (the seeded "item"). Nil when the campaign has
// neither.
func SwitchTarget(types []EntityType, category string) *EntityType {
	if category == "" {
		return nil
	}
	for i := range types {
		if types[i].PresetCategory != nil && *types[i].PresetCategory == category {
			return &types[i]
		}
	}
	for i := range types {
		t := &types[i]
		if t.Slug == category && t.ParentTypeID == nil && (t.PresetCategory == nil || *t.PresetCategory == "") {
			return t
		}
	}
	return nil
}

// fieldKeySet is the set of keys in fs.
func fieldKeySet(fs []FieldDefinition) map[string]bool {
	out := make(map[string]bool, len(fs))
	for _, f := range fs {
		out[f.Key] = true
	}
	return out
}

func keySet(keys []string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

// switchedFieldList is the category's field list after accepting: the
// system's fields in its order (a shared key takes the system's definition),
// then the category's own fields that are neither set aside nor the system's.
func switchedFieldList(current []FieldDefinition, offer FieldSwitchOffer) []FieldDefinition {
	theirs := fieldKeySet(offer.Fields)
	aside := keySet(offer.SetAside)
	out := make([]FieldDefinition, 0, len(offer.Fields)+len(current))
	out = append(out, cloneFieldDefs(offer.Fields)...)
	for _, f := range current {
		if theirs[f.Key] || aside[f.Key] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// switchedBackFieldList is the category's field list after switching back:
// the list from before the switch, then any field added since that is not
// one of the system's.
func switchedBackFieldList(current, old []FieldDefinition, offer FieldSwitchOffer) []FieldDefinition {
	had := fieldKeySet(old)
	theirs := fieldKeySet(offer.Fields)
	out := make([]FieldDefinition, 0, len(old)+len(current))
	out = append(out, cloneFieldDefs(old)...)
	for _, f := range current {
		if had[f.Key] || theirs[f.Key] {
			continue
		}
		out = append(out, f)
	}
	return out
}

func cloneFieldDefs(fs []FieldDefinition) []FieldDefinition {
	out := make([]FieldDefinition, len(fs))
	copy(out, fs)
	for i := range out {
		if out[i].Options != nil {
			out[i].Options = append([]string(nil), out[i].Options...)
		}
	}
	return out
}

// valueMove says which keys leave a page's values for the kept set, which
// come back from it, and which are reused (swapped when the kept value fits
// and the page's does not).
type valueMove struct {
	leaving  map[string]bool
	arriving map[string]bool
	reuse    map[string]func(any) bool
}

// acceptMove is the value move for accepting: the set-aside keys the category
// has leave; the system's keys it lacks arrive (a value kept from an earlier
// switch back comes back); a reused key keeps only a value that is one of the
// system field's options.
func acceptMove(current []FieldDefinition, offer FieldSwitchOffer) valueMove {
	have := fieldKeySet(current)
	m := valueMove{leaving: map[string]bool{}, arriving: map[string]bool{}, reuse: map[string]func(any) bool{}}
	for _, k := range offer.SetAside {
		if have[k] {
			m.leaving[k] = true
		}
	}
	for _, f := range offer.Fields {
		if !have[f.Key] {
			m.arriving[f.Key] = true
		}
	}
	for _, k := range offer.Reuse {
		if !have[k] {
			continue
		}
		for _, f := range offer.Fields {
			if f.Key == k {
				m.reuse[k] = fitsOptions(f.Options)
			}
		}
	}
	return m
}

// backMove is the value move for switching back: the system's keys the old
// list lacks leave; the old keys the system lacks come back; a reused key
// takes its kept value back, keeping the system's value aside in its place.
func backMove(old []FieldDefinition, offer FieldSwitchOffer) valueMove {
	had := fieldKeySet(old)
	theirs := fieldKeySet(offer.Fields)
	m := valueMove{leaving: map[string]bool{}, arriving: map[string]bool{}, reuse: map[string]func(any) bool{}}
	for _, f := range offer.Fields {
		if !had[f.Key] {
			m.leaving[f.Key] = true
		}
	}
	for _, f := range old {
		if !theirs[f.Key] {
			m.arriving[f.Key] = true
		}
	}
	for _, k := range offer.Reuse {
		if had[k] && theirs[k] {
			m.reuse[k] = func(any) bool { return true }
		}
	}
	return m
}

// fitsOptions reports whether a value is blank or one of options (any case).
// With no options every value fits.
func fitsOptions(options []string) func(any) bool {
	return func(v any) bool {
		if isBlankValue(v) || len(options) == 0 {
			return true
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		for _, o := range options {
			if strings.EqualFold(o, s) {
				return true
			}
		}
		return false
	}
}

func isBlankValue(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// apply moves one page's values. data and kept are not modified; the
// returned maps are fresh. changed is false when nothing moved, so the
// caller writes nothing for that page.
func (m valueMove) apply(data, kept map[string]any) (map[string]any, map[string]any, bool) {
	d := make(map[string]any, len(data))
	for k, v := range data {
		d[k] = v
	}
	kp := make(map[string]any, len(kept))
	for k, v := range kept {
		kp[k] = v
	}
	changed := false
	for k := range m.leaving {
		if v, ok := d[k]; ok {
			kp[k] = v
			delete(d, k)
			changed = true
		}
	}
	for k := range m.arriving {
		v, ok := kp[k]
		if !ok {
			continue
		}
		// A value typed under the same key since wins; the kept one stays kept.
		if cur, has := d[k]; has && !isBlankValue(cur) {
			continue
		}
		d[k] = v
		delete(kp, k)
		changed = true
	}
	for k, fits := range m.reuse {
		cur, hasCur := d[k]
		old, hasOld := kp[k]
		switch {
		case hasOld && fits(old):
			d[k] = old
			if hasCur && !isBlankValue(cur) {
				kp[k] = cur
			} else {
				delete(kp, k)
			}
			changed = true
		case hasCur && !fits(cur) && !hasOld:
			kp[k] = cur
			delete(d, k)
			changed = true
		}
	}
	return d, kp, changed
}

// setAsideValues lists a page's kept values for the GM's fold, labelled from
// the field lists that knew them (the old list first), in that order, then
// any other key by name.
func setAsideValues(kept map[string]any, labelled ...[]FieldDefinition) []SetAsideValue {
	if len(kept) == 0 {
		return nil
	}
	type known struct {
		label string
		kind  string
		order int
	}
	byKey := map[string]known{}
	n := 0
	for _, fs := range labelled {
		for _, f := range fs {
			if _, seen := byKey[f.Key]; seen {
				continue
			}
			byKey[f.Key] = known{label: f.Label, kind: f.Type, order: n}
			n++
		}
	}
	out := make([]SetAsideValue, 0, len(kept))
	for k, v := range kept {
		info, ok := byKey[k]
		label := k
		if ok && info.label != "" {
			label = info.label
		}
		out = append(out, SetAsideValue{Key: k, Label: label, Value: setAsideText(v, info.kind)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := byKey[out[i].Key]
		b, bok := byKey[out[j].Key]
		if aok != bok {
			return aok
		}
		if aok && a.order != b.order {
			return a.order < b.order
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// setAsideText is a kept value as words: a tick as Yes or No, a blank as
// "empty", anything else as written.
func setAsideText(v any, kind string) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "Yes"
		}
		return "No"
	case nil:
		return "empty"
	case string:
		if kind == "checkbox" {
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "true", "on", "1", "yes":
				return "Yes"
			default:
				return "No"
			}
		}
		if strings.TrimSpace(x) == "" {
			return "empty"
		}
		return x
	}
	return fmt.Sprint(v)
}

// switchView builds what the owner sees for a category and its offer.
func switchView(et *EntityType, offer FieldSwitchOffer, sw *FieldSwitch) *FieldSwitchView {
	v := &FieldSwitchView{Offer: offer}
	if sw != nil {
		v.State = sw.State
		v.CanSwitchBack = sw.State == FieldSwitchSwitched && sw.OldFields != nil
	}
	have := fieldKeySet(et.Fields)
	for _, f := range offer.Fields {
		v.Adds = append(v.Adds, f.Label)
	}
	for _, k := range offer.SetAside {
		if !have[k] {
			continue
		}
		for _, f := range et.Fields {
			if f.Key == k {
				v.Removes = append(v.Removes, f.Label)
			}
		}
	}
	return v
}

// joinAnd joins words as a sentence list: "a", "a and b", "a, b and c".
func joinAnd(words []string, last string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " " + last + " " + words[len(words)-1]
}

// AddsLine is the offer's first line: what the switch puts on the pages.
func (v *FieldSwitchView) AddsLine() string {
	if v.Offer.Summary != "" {
		return "Adds " + v.Offer.Summary + "."
	}
	return "Adds " + joinAnd(v.Adds, "and") + "."
}

// RemovesLine is the offer's second line: what comes off the pages and that
// it is kept. A reused field's values that won't fit are named too, since
// they leave the visible field. "" when nothing comes off.
func (v *FieldSwitchView) RemovesLine(categoryFields []FieldDefinition) string {
	var parts []string
	if len(v.Removes) > 0 {
		parts = append(parts, joinAnd(v.Removes, "and")+" come off the pages")
	}
	have := fieldKeySet(categoryFields)
	for _, k := range v.Offer.Reuse {
		if !have[k] {
			continue
		}
		for _, f := range v.Offer.Fields {
			if f.Key == k && len(f.Options) > 0 {
				parts = append(parts, fmt.Sprintf("so does a %s that isn’t %s", f.Label, joinAnd(f.Options, "or")))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(v.Removes) == 0 {
		// Only a reused field's misfits leave: start the sentence with them.
		parts[0] = "A " + strings.TrimPrefix(parts[0], "so does a ") + " comes off the pages"
	}
	return strings.Join(parts, ", and ") + ". What they held is kept in a fold only you see."
}

// BackLine says what switching back does, for the category's Fields tab.
func (v *FieldSwitchView) BackLine() string {
	return "Switching back puts the fields from before back on the pages, with what they held. The " +
		v.Offer.Label + " come off, and what they hold is kept the same way."
}

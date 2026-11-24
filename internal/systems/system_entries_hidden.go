package systems

import (
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A campaign entry's details can be kept from players one by one: the
// detail EntryHiddenKey lists the names of the details only Directors read
// (a complication's drawback, a kit's secret rider). It is the one place
// that says so, and visible() drops those details and the list itself for
// everyone else, so the sheet, the pick lists, the rules pages and the
// Foundry sync, which all read through it, never carry them.
const EntryHiddenKey = "director_only"

// HiddenDetails is the set of detail names p keeps from players.
func HiddenDetails(p map[string]any) map[string]bool {
	out := map[string]bool{}
	list, _ := p[EntryHiddenKey].([]any)
	for _, v := range list {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out[strings.TrimSpace(s)] = true
		}
	}
	if strs, ok := p[EntryHiddenKey].([]string); ok {
		for _, s := range strs {
			if strings.TrimSpace(s) != "" {
				out[strings.TrimSpace(s)] = true
			}
		}
	}
	return out
}

// PlayerProperties is p as a player may read it: without the hidden details
// and without the list naming them. p itself is not changed.
func PlayerProperties(p map[string]any) map[string]any {
	if _, ok := p[EntryHiddenKey]; !ok {
		return p
	}
	hidden := HiddenDetails(p)
	out := make(map[string]any, len(p))
	for k, v := range p {
		if k == EntryHiddenKey || hidden[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// validateHiddenList checks EntryHiddenKey names only details the entry has,
// so a typo never leaves a detail open that the Director meant to hide.
func validateHiddenList(p map[string]any) error {
	v, ok := p[EntryHiddenKey]
	if !ok {
		return nil
	}
	list, isList := v.([]any)
	if !isList {
		return apperror.NewValidation(fmt.Sprintf("The detail %q must be a list of the names of details kept from players.", EntryHiddenKey))
	}
	for _, it := range list {
		name, _ := it.(string)
		name = strings.TrimSpace(name)
		if name == "" || name == EntryHiddenKey {
			return apperror.NewValidation(fmt.Sprintf("The detail %q must list detail names.", EntryHiddenKey))
		}
		if _, has := p[name]; !has {
			return apperror.NewValidation(fmt.Sprintf("The detail %q names %q, which this entry does not have.", EntryHiddenKey, name))
		}
	}
	return nil
}

// keepHidden is what a non-Director's save may do to the hidden details: it
// never sees them, so they and the list come back as stored, and it may not
// hide anything of its own.
func keepHidden(next, stored map[string]any) map[string]any {
	if next == nil {
		next = map[string]any{}
	}
	delete(next, EntryHiddenKey)
	if _, ok := stored[EntryHiddenKey]; !ok {
		return next
	}
	for k := range HiddenDetails(stored) {
		if v, ok := stored[k]; ok {
			next[k] = v
		}
	}
	next[EntryHiddenKey] = stored[EntryHiddenKey]
	return next
}

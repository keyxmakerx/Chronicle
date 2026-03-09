// Package dnd5e implements the D&D 5th Edition game system for Chronicle.
// It loads SRD reference data (spells, monsters, items, etc.) from JSON
// files and provides tooltip rendering for hover previews.
package dnd5e

import (
	"fmt"
	"html"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

func init() {
	systems.RegisterFactory("dnd5e", func(manifest *systems.SystemManifest, dataDir string) (systems.System, error) {
		return New(manifest, dataDir)
	})
}

// DnD5eSystem is the concrete System implementation for D&D 5th Edition.
// It wraps a JSONProvider for data access and a tooltip renderer for
// hover preview HTML fragments.
type DnD5eSystem struct {
	manifest *systems.SystemManifest
	provider *systems.JSONProvider
	renderer *TooltipRenderer
}

// New creates and initializes a D&D 5e system from its manifest and
// data directory. Returns an error if the data files cannot be loaded.
func New(manifest *systems.SystemManifest, dataDir string) (*DnD5eSystem, error) {
	provider, err := systems.NewJSONProvider(manifest.ID, dataDir)
	if err != nil {
		return nil, fmt.Errorf("dnd5e: loading data: %w", err)
	}

	return &DnD5eSystem{
		manifest: manifest,
		provider: provider,
		renderer: &TooltipRenderer{},
	}, nil
}

// Info returns the system's manifest metadata.
func (m *DnD5eSystem) Info() *systems.SystemManifest {
	return m.manifest
}

// DataProvider returns the JSON-file data provider for SRD content.
func (m *DnD5eSystem) DataProvider() systems.DataProvider {
	return m.provider
}

// TooltipRenderer returns the D&D-specific tooltip renderer.
func (m *DnD5eSystem) TooltipRenderer() systems.TooltipRenderer {
	return m.renderer
}

// TooltipRenderer produces HTML tooltip fragments for D&D 5e reference
// items. It formats spell properties (level, school, casting time, etc.)
// into a compact preview card.
type TooltipRenderer struct{}

// RenderTooltip returns an HTML fragment for a D&D 5e reference item.
// The output is a self-contained tooltip card suitable for hover previews.
func (r *TooltipRenderer) RenderTooltip(item *systems.ReferenceItem) (string, error) {
	if item == nil {
		return "", fmt.Errorf("nil reference item")
	}

	var b strings.Builder

	b.WriteString(`<div class="system-tooltip system-tooltip--dnd5e">`)
	b.WriteString(`<div class="system-tooltip__header">`)
	b.WriteString(`<strong>`)
	b.WriteString(html.EscapeString(item.Name))
	b.WriteString(`</strong>`)

	// Category badge.
	b.WriteString(`<span class="system-tooltip__badge">`)
	b.WriteString(html.EscapeString(item.Category))
	b.WriteString(`</span>`)
	b.WriteString(`</div>`)

	// Properties table (varies by category).
	if len(item.Properties) > 0 {
		b.WriteString(`<div class="system-tooltip__props">`)
		writeCategoryProperties(&b, item)
		b.WriteString(`</div>`)
	}

	// Summary.
	if item.Summary != "" {
		b.WriteString(`<div class="system-tooltip__summary">`)
		b.WriteString(html.EscapeString(item.Summary))
		b.WriteString(`</div>`)
	}

	// Source badge.
	if item.Source != "" {
		b.WriteString(`<div class="system-tooltip__source">`)
		b.WriteString(html.EscapeString(item.Source))
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	return b.String(), nil
}

// SupportedCategories returns all D&D 5e category slugs.
func (r *TooltipRenderer) SupportedCategories() []string {
	return []string{"spells", "monsters", "items", "classes", "races", "conditions"}
}

// writeCategoryProperties renders the appropriate property rows based on
// the item's category. Each category shows only its relevant fields.
func writeCategoryProperties(b *strings.Builder, item *systems.ReferenceItem) {
	switch item.Category {
	case "spells":
		writeProperty(b, item.Properties, "level", "Level")
		writeProperty(b, item.Properties, "school", "School")
		writeProperty(b, item.Properties, "casting_time", "Casting Time")
		writeProperty(b, item.Properties, "range", "Range")
		writeProperty(b, item.Properties, "components", "Components")
		writeProperty(b, item.Properties, "duration", "Duration")
	case "monsters":
		writeProperty(b, item.Properties, "cr", "CR")
		writeProperty(b, item.Properties, "type", "Type")
		writeProperty(b, item.Properties, "size", "Size")
		writeProperty(b, item.Properties, "alignment", "Alignment")
		writeProperty(b, item.Properties, "hp", "HP")
		writeProperty(b, item.Properties, "ac", "AC")
	case "items":
		writeProperty(b, item.Properties, "rarity", "Rarity")
		writeProperty(b, item.Properties, "type", "Type")
		writeProperty(b, item.Properties, "attunement", "Attunement")
	case "classes":
		writeProperty(b, item.Properties, "hit_die", "Hit Die")
		writeProperty(b, item.Properties, "primary_ability", "Primary Ability")
		writeProperty(b, item.Properties, "saving_throws", "Saving Throws")
	case "races":
		writeProperty(b, item.Properties, "speed", "Speed")
		writeProperty(b, item.Properties, "size", "Size")
		writeProperty(b, item.Properties, "ability_bonuses", "Ability Bonuses")
	case "conditions":
		writeProperty(b, item.Properties, "effect", "Effect")
	default:
		// Unknown category: render all properties generically.
		for k := range item.Properties {
			writeProperty(b, item.Properties, k, k)
		}
	}
}

// writeProperty appends a label/value row if the property exists.
func writeProperty(b *strings.Builder, props map[string]any, key, label string) {
	val, ok := props[key]
	if !ok {
		return
	}
	b.WriteString(`<div class="system-tooltip__prop">`)
	b.WriteString(`<span class="system-tooltip__label">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`:</span> `)
	b.WriteString(html.EscapeString(fmt.Sprintf("%v", val)))
	b.WriteString(`</div>`)
}

// Package modules — generic tooltip renderer.
//
// GenericTooltipRenderer produces HTML tooltip fragments for any module
// by reading the field schema from the module manifest. No custom code
// is needed — just declare fields in manifest.json and the renderer
// will display matching properties from each ReferenceItem.
//
// Game-system modules can override this by implementing their own
// TooltipRenderer for richer formatting (e.g., stat blocks, icons).
package systems

import (
	"fmt"
	"html"
	"strings"
)

// GenericTooltipRenderer produces HTML tooltips by iterating the manifest's
// field definitions for each category. Works for any module without
// custom Go code — just drop data files and a manifest.json.
type GenericTooltipRenderer struct {
	// moduleName is used in the CSS class modifier (e.g., "module-tooltip--dnd5e").
	moduleName string

	// fields maps category slugs to their ordered field definitions.
	// Loaded from the module manifest at construction time.
	fields map[string][]FieldDef

	// categories lists all supported category slugs.
	categories []string
}

// NewGenericTooltipRenderer creates a tooltip renderer that uses the
// manifest's field definitions to render properties for each category.
func NewGenericTooltipRenderer(manifest *SystemManifest) *GenericTooltipRenderer {
	fields := make(map[string][]FieldDef, len(manifest.Categories))
	cats := make([]string, 0, len(manifest.Categories))

	for _, cat := range manifest.Categories {
		fields[cat.Slug] = cat.Fields
		cats = append(cats, cat.Slug)
	}

	return &GenericTooltipRenderer{
		moduleName: manifest.ID,
		fields:     fields,
		categories: cats,
	}
}

// RenderTooltip returns an HTML fragment for a reference item, using
// the manifest's field definitions to determine which properties to show.
func (r *GenericTooltipRenderer) RenderTooltip(item *ReferenceItem) (string, error) {
	if item == nil {
		return "", fmt.Errorf("nil reference item")
	}

	var b strings.Builder

	b.WriteString(`<div class="module-tooltip module-tooltip--`)
	b.WriteString(html.EscapeString(r.moduleName))
	b.WriteString(`">`)

	// Header with name and category badge.
	b.WriteString(`<div class="module-tooltip__header">`)
	b.WriteString(`<strong>`)
	b.WriteString(html.EscapeString(item.Name))
	b.WriteString(`</strong>`)
	b.WriteString(`<span class="module-tooltip__badge">`)
	b.WriteString(html.EscapeString(item.Category))
	b.WriteString(`</span>`)
	b.WriteString(`</div>`)

	// Properties from manifest field definitions.
	if len(item.Properties) > 0 {
		fieldDefs := r.fields[item.Category]
		if len(fieldDefs) > 0 {
			b.WriteString(`<div class="module-tooltip__props">`)
			for _, f := range fieldDefs {
				val, ok := item.Properties[f.Key]
				if !ok {
					continue
				}
				b.WriteString(`<div class="module-tooltip__prop">`)
				b.WriteString(`<span class="module-tooltip__label">`)
				b.WriteString(html.EscapeString(f.Label))
				b.WriteString(`:</span> `)
				b.WriteString(html.EscapeString(fmt.Sprintf("%v", val)))
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}
	}

	// Summary.
	if item.Summary != "" {
		b.WriteString(`<div class="module-tooltip__summary">`)
		b.WriteString(html.EscapeString(item.Summary))
		b.WriteString(`</div>`)
	}

	// Source badge.
	if item.Source != "" {
		b.WriteString(`<div class="module-tooltip__source">`)
		b.WriteString(html.EscapeString(item.Source))
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	return b.String(), nil
}

// SupportedCategories returns all category slugs from the manifest.
func (r *GenericTooltipRenderer) SupportedCategories() []string {
	return r.categories
}

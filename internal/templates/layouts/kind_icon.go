package layouts

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/colour"
)

// KindIconStyle is the inline custom property that hands a category's saved
// colour to the Category icon colour theme. It is empty unless the campaign
// picked that theme and the colour is a plain #RRGGBB, so a campaign that
// never chose it renders exactly as before and a hand-edited value can never
// reach a style attribute.
//
// The colour is a property, not a `color:` declaration: the stylesheet decides
// whether to use it, and it mixes it toward the text colour first so every
// category reads with similar strength in light and dark.
func KindIconStyle(ctx context.Context, hex string) string {
	if a := GetAppearance(ctx); a == nil || a.IconColour != "category" {
		return ""
	}
	if !colour.ValidHex(hex) {
		return ""
	}
	return "--kind-colour:" + colour.Lower(hex)
}

// KindIconAttrs is KindIconStyle as spread attributes, so an element that has
// no colour to carry gets no empty style attribute.
func KindIconAttrs(ctx context.Context, hex string) templ.Attributes {
	if s := KindIconStyle(ctx, hex); s != "" {
		return templ.Attributes{"style": s}
	}
	return nil
}

// navRowKindAttrs carries a colour only on a category's row: an app or a link
// stands for no category, so a colour set on one is ignored.
func navRowKindAttrs(ctx context.Context, row NavRowView) templ.Attributes {
	if row.Kind != "category" {
		return nil
	}
	return KindIconAttrs(ctx, row.Color)
}

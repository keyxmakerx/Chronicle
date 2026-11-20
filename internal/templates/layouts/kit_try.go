package layouts

import (
	"context"
	"strings"
)

// czKitOrder is the order the style kits are offered in, Material first, as
// in Customize › Look › Pages (campaigns.AppearanceStyleKits).
var czKitOrder = []string{"material", "sharp", "ornate", "flowing", "lumen", "sketchbook"}

// kitTryResets clears every token a saved kit or Material sets, so a kit
// tried over another starts from nothing: "initial" makes a custom property
// unset, and each var() then takes its fallback as it would with no kit.
const kitTryResets = "--kit-hf:initial;--kit-lf:initial;--kit-bf:initial;--kit-user-acc:initial;" +
	"--annals-hf:initial;--annals-hcase:initial;--annals-hw:initial;--annals-hls:initial;"

// KitTryCSS is each style kit's tokens and faces for this campaign, written
// as AppearanceCSS writes the saved kit's (kitCSS, and Material's tall
// capitals) but keyed on html[data-kit-try="<kit>"], for the header brush's
// "Try a style kit here". The selectors out-rank AppearanceCSS's :root
// blocks, so a tried kit replaces the saved one's tokens while
// style_kits.css draws it from data-cz-kit as usual. Empty when the
// campaign's page look is not Material, where no kit is drawn.
func KitTryCSS(ctx context.Context) string {
	a := GetAppearance(ctx)
	if a == nil || a.PageLook != "annals" {
		return ""
	}
	accent := GetAccentColor(ctx)
	var extra, out strings.Builder
	faces := map[string]bool{}
	for _, id := range czKitOrder {
		var root, dark, kitExtra strings.Builder
		root.WriteString(kitTryResets)
		if k, ok := czKits[id]; ok {
			kitCSS(&root, &dark, &kitExtra, a, k, accent)
		} else if _, ok := czHeadingFaces[a.HeadingFont]; !ok {
			kitExtra.WriteString(fontFaceCSS("oswald"))
			root.WriteString("--annals-hf:'Oswald', 'Arial Narrow', sans-serif;--annals-hcase:uppercase;--annals-hw:600;--annals-hls:.01em;")
		}
		// One face can serve two kits; its rules are written once.
		if f := kitExtra.String(); f != "" && !faces[f] {
			faces[f] = true
			extra.WriteString(f)
		}
		out.WriteString(`:root[data-kit-try="` + id + `"]{` + root.String() + "}")
		if dark.Len() > 0 {
			out.WriteString(`:root.dark[data-kit-try="` + id + `"]{` + dark.String() + "}")
		}
	}
	return extra.String() + out.String()
}

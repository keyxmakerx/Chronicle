package entities

import "embed"

// PluginSlug is the entities plugin's registry key. Used to mount the plugin's
// embedded static assets at /static/plugins/entities/.
const PluginSlug = "entities"

// StaticAssetsFS contains the entities plugin's static assets (JS/CSS), served
// by Echo at /static/plugins/entities/ once registered in the App's plugin
// registry. Currently holds js/characters.js (the Characters page's mini→full
// launch enhancement). Per cordinator/decisions/2026-05-25-plugin-static-assets.md.
//
//go:embed static
var StaticAssetsFS embed.FS

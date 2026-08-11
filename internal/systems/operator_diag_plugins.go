// Package systems — operator_diag_plugins.go is the EXTENSION-TIER half of the
// host diagnostics. host.build says which BINARY is running and
// host.assets/host.embedded say which BYTES it serves; these two say which
// PLUGINS that binary registered and which WIDGET assets it is serving.
//
// WHY it exists. The operator's actual words on 2026-08-11 were "calendar (and
// other widget) versions" — and Chronicle could not answer that question in any
// form. It could fingerprint every installed system PACKAGE down to the sha256
// of each served file, and could say nothing at all about its own widgets or
// its own plugins.
//
// Two things about that question have to be said out loud rather than assumed,
// because guessing at either one is how the original hour was lost:
//
//   - **Widgets have no version number.** Nothing declares one, nothing stores
//     one, and there is no server-side widget registry to consult: a widget is a
//     JS file that registers itself with boot.js when the browser mounts a
//     `data-widget` element. So the honest answer to "what version is the
//     calendar widget?" is a CONTENT FINGERPRINT plus a build time, and this
//     file says so in the Desc rather than inventing a number that would look
//     authoritative and mean nothing.
//
//   - **Widget assets live in two unrelated places**, and the calendar widget in
//     particular lives only in the one a shell cannot see. `static/js/widgets/`
//     is on disk; each plugin's `js/` is `//go:embed`-ed into the executable.
//     Grepping the container filesystem for the calendar widget returns nothing
//     and always will. That empty grep was read as missing code once already.
//
// And "is this plugin even loaded?" has a trap of its own: Chronicle has no
// plugin LOADER. Every plugin is compiled in and its routes are registered
// unconditionally. The two registries this file reads are opt-in METADATA, so
// absence from them is not absence from the build — host.plugins states that
// every time rather than letting a missing row read as a missing feature.
package systems

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// widgetsSubdir is where Chronicle keeps its central, on-disk widget JS,
// relative to the static root. Plugin-embedded widget JS sits under a bare
// "js/" inside each plugin's own filesystem instead — the two layouts differ,
// which is exactly why a single walk cannot cover both.
const widgetsSubdir = "js/widgets/"

// embeddedJSSubdir is the prefix under which a plugin's embedded widget scripts
// live, relative to that plugin's static root.
const embeddedJSSubdir = "js/"

// HostPlugin is one plugin as the APP layer sees it, flattened for reporting.
//
// Injected rather than imported for the same dependency inversion as
// EmbeddedAssetSet: internal/systems must not import internal/app or the plugin
// packages, so the app layer merges its registries and hands over plain data.
//
// The fields deliberately keep the two registries APART instead of collapsing
// them into one "registered" boolean. Chronicle has a metadata registry
// (PluginRegistration — slug, health hook, StaticFS) and a schema registry
// (database.PluginSchema — migrations), they contain different plugins, and a
// row present in one and absent from the other is normal rather than broken.
type HostPlugin struct {
	// Slug is the plugin's canonical identifier as the reporting registry
	// spells it.
	Slug string

	// SchemaKey is the key the migration runner and health registry use, when
	// it differs from Slug. It DOES differ in practice — foundry_vtt registers
	// as `foundry-vtt` in the metadata registry and `foundry_vtt` in the schema
	// runner — so an operator grepping logs for one spelling will miss the
	// other. Empty when the two agree.
	SchemaKey string

	// InMetadataRegistry reports presence in App.registeredPlugins.
	InMetadataRegistry bool

	// HasHealthCheck reports whether the metadata registration carried a health
	// callback; HealthErr is that callback's result ("" = healthy or not run).
	HasHealthCheck bool
	HealthErr      string

	// StaticURLPrefix is where the plugin's embedded assets are served, "" when
	// it registered none. StaticFS is that filesystem, walked here for the
	// count and aggregate size.
	StaticURLPrefix string
	StaticFS        fs.FS

	// DeclaresMigrations reports whether the plugin handed a MigrationsFS to
	// the schema runner at startup.
	DeclaresMigrations bool

	// SchemaKnown is true when the migration health registry holds a record for
	// this plugin. Without it, SchemaHealthy false would advertise a failure
	// that was never observed.
	SchemaKnown   bool
	SchemaHealthy bool
	SchemaVersion int // highest APPLIED migration version
	SchemaLatest  int // highest AVAILABLE migration version
	SchemaError   string
}

// hostPluginsFn returns the merged plugin registries, or nil when the app layer
// has not wired it — in which case host.plugins SAYS so rather than reporting
// an empty registry, because "no plugins" and "nobody told me about the
// plugins" point at opposite conclusions.
var hostPluginsFn func() []HostPlugin

// SetHostPluginsProvider wires the app's plugin registries into host.plugins.
// Called once at startup from the app wiring.
func SetHostPluginsProvider(fn func() []HostPlugin) { hostPluginsFn = fn }

// hostPluginsDiagnostic answers "is this plugin even loaded, and did its schema
// actually run?".
func hostPluginsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:  "host.plugins",
		Title: "Plugins this binary registered — static mounts, migrations, schema health",
		Desc:  "Per plugin: slug, whether it mounted an embedded static filesystem (and at what URL prefix), how many assets that holds and their total size, whether it contributed migrations, and the applied-vs-available schema version from the migration runner. Note Chronicle has no plugin loader — every plugin is compiled in and registered unconditionally, so these registries are opt-in metadata and a missing row is NOT a missing feature.",
		Run:   func(string) string { return renderHostPlugins() },
	}
}

// hostWidgetsDiagnostic answers the operator's original question.
func hostWidgetsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "host.widgets",
		Title:   "Widget assets this process serves, with the fingerprint that stands in for a version",
		Desc:    "THE 'what version of the calendar (and other) widgets am I running?' check — and widgets carry NO version number, so the honest answer is a content fingerprint plus a build time. Per widget: where its asset lives (central on-disk `static/js/widgets/` vs compiled INTO the binary by a plugin), size, sha256[:16], mtime where one exists, and the `?v=` token this process serves. There is no server-side widget registry, so this walks those two known locations and says which one each result came from. Optional argument = a name substring.",
		ArgHint: "[<name-substring>]",
		Run:     renderHostWidgets,
	}
}

// ── host.plugins ───────────────────────────────────────────────────────────

// renderHostPlugins wires the live provider into the pure renderer.
func renderHostPlugins() string { return renderHostPluginsFrom(hostPluginsFn) }

// renderHostPluginsFrom is the pure renderer. The PROVIDER (not its result) is
// injected so the unwired case — a nil func, which must degrade to a message
// rather than a nil-deref or an empty-looking "no plugins" — is directly
// testable.
func renderHostPluginsFrom(provider func() []HostPlugin) string {
	var b strings.Builder
	b.WriteString("## host.plugins — plugins this binary registered\n\n")

	if provider == nil {
		b.WriteString("_Provider not wired (the app layer did not inject the plugin registries at startup), so nothing can be listed. **This is not \"no plugins are loaded\"** — it means nothing told this diagnostic about them. Expected wiring: `systems.SetHostPluginsProvider(...)` in the app's route registration._\n")
		return b.String()
	}
	plugins := provider()
	if len(plugins) == 0 {
		b.WriteString("_The provider is wired and reports no plugin in either registry. Unlike the not-wired case, this IS an answer — but see the note below: it would still not mean the features are absent._\n\n")
		writePluginRegistryNote(&b)
		return b.String()
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Slug < plugins[j].Slug })

	withStatic, withMigrations := 0, 0
	for _, p := range plugins {
		if p.StaticURLPrefix != "" {
			withStatic++
		}
		if p.DeclaresMigrations {
			withMigrations++
		}
	}
	fmt.Fprintf(&b, "**%d plugin(s) in the registries** — %d mount embedded static assets, %d contribute migrations.\n\n", len(plugins), withStatic, withMigrations)

	for _, p := range plugins {
		writePluginRow(&b, p)
	}
	writePluginRegistryNote(&b)
	return b.String()
}

// writePluginRow renders one plugin. Every sub-line states a fact the operator
// can act on; a plugin that registered nothing still gets its row, because the
// absence is the answer to "is it loaded?" and hiding it would leave the
// question open.
func writePluginRow(b *strings.Builder, p HostPlugin) {
	fmt.Fprintf(b, "### `%s`\n\n", p.Slug)
	if p.SchemaKey != "" && p.SchemaKey != p.Slug {
		fmt.Fprintf(b, "- **also spelled `%s`** — the metadata registry and the migration runner use different spellings for this plugin, so a grep for one will miss the other (both are real; neither is a typo).\n", p.SchemaKey)
	}

	// Metadata registry: the only registry that carries a StaticFS.
	if p.InMetadataRegistry {
		b.WriteString("- metadata registry: **registered**")
		switch {
		case !p.HasHealthCheck:
			b.WriteString(" (no health callback)\n")
		case p.HealthErr != "":
			fmt.Fprintf(b, " · health callback reports: **%s**\n", singleLine(p.HealthErr))
		default:
			b.WriteString(" · health callback reports healthy\n")
		}
	} else {
		b.WriteString("- metadata registry: not registered — it contributes no StaticFS and no health hook. Routes are registered unconditionally regardless, so this says nothing about whether the plugin works.\n")
	}

	writePluginStatic(b, p)
	writePluginSchema(b, p)
	b.WriteString("\n")
}

// writePluginStatic reports the plugin's embedded static mount. The count and
// aggregate size are computed by walking the SAME filesystem the app serves,
// so a plugin that mounted an empty or nil FS is reported as such instead of
// being credited with assets it does not have.
func writePluginStatic(b *strings.Builder, p HostPlugin) {
	if p.StaticURLPrefix == "" {
		b.WriteString("- embedded static assets: none registered (its front-end is Templ + on-disk assets, or it has no front-end). Nothing is served under `/static/plugins/" + p.Slug + "/`.\n")
		return
	}
	if p.StaticFS == nil {
		fmt.Fprintf(b, "- embedded static assets: **mounted at `%s` with a nil filesystem** — the prefix is claimed but nothing is served under it. That is a defect worth reporting.\n", p.StaticURLPrefix)
		return
	}
	files, total, walkErr := countEmbeddedFiles(p.StaticFS)
	fmt.Fprintf(b, "- embedded static assets: **%d file(s), %s**, served at `%s` — compiled into the binary, so `ls` over the container filesystem will not show them. `host.embedded %s` lists them individually.\n",
		files, humanBytes(total), p.StaticURLPrefix, p.Slug)
	if walkErr != "" {
		fmt.Fprintf(b, "  - _walk reported an error, so the count may be partial: %s_\n", walkErr)
	}
}

// writePluginSchema reports migrations. Applied-vs-available is taken from the
// migration runner's own record rather than counted here: the runner is what
// actually executed them, and a second count computed from the files would be a
// second opinion that can disagree with the database.
func writePluginSchema(b *strings.Builder, p HostPlugin) {
	if !p.DeclaresMigrations {
		b.WriteString("- migrations: none contributed — this plugin owns no tables of its own.\n")
		return
	}
	if !p.SchemaKnown {
		b.WriteString("- migrations: **contributed, but the migration health registry holds no record for it.** That is not \"they failed\" and not \"they succeeded\" — nothing observed the outcome. Check the boot log.\n")
		return
	}
	state := "healthy"
	if !p.SchemaHealthy {
		state = "**UNHEALTHY**"
	}
	fmt.Fprintf(b, "- migrations: %s · applied version **%d** of %d available\n", state, p.SchemaVersion, p.SchemaLatest)
	if p.SchemaHealthy && p.SchemaLatest > p.SchemaVersion {
		fmt.Fprintf(b, "  - ⚠️ %d migration(s) shipped in this binary have NOT been applied. The plugin is marked healthy, so they were not attempted-and-failed — most likely this binary is newer than the database it is pointed at.\n", p.SchemaLatest-p.SchemaVersion)
	}
	if p.SchemaError != "" {
		fmt.Fprintf(b, "  - error: %s\n", singleLine(p.SchemaError))
	}
}

// writePluginRegistryNote is printed on every run, including a clean one. The
// single most misleading thing this diagnostic could do is let a missing row
// read as a missing feature, and a caveat that only appears when something is
// wrong teaches nothing on the run where the answer is "not this".
func writePluginRegistryNote(b *strings.Builder) {
	b.WriteString("### How to read this\n\n")
	b.WriteString("- **Chronicle has no plugin loader.** Every plugin under `internal/plugins/` is compiled into this binary and its routes are registered unconditionally at startup. Nothing is dynamically loaded, enabled or disabled at runtime.\n")
	b.WriteString("- **These are two opt-in registries, not a manifest of what exists.** A plugin joins the metadata registry only if it needs a static mount or a health hook, and joins the schema registry only if it owns tables. Most plugins need neither and appear here not at all — their absence is normal and is NOT evidence that a feature is missing.\n")
	b.WriteString("- **The source tree cannot be enumerated from here.** Only the binary and the static root ship to a deployment; `internal/plugins/` does not exist in the container, so no diagnostic can list the plugin packages that were compiled in but registered nothing.\n")
	b.WriteString("- To see what an embedded static mount actually contains, run `host.embedded [<slug>]`; to check the CONTENT of one of those files, `host.embedded-contains <slug>:<path>:<marker>`.\n")
}

// countEmbeddedFiles totals one embedded filesystem. Best effort by design: an
// unreadable entry is skipped and reported rather than aborting the count,
// because a partial number with a stated caveat beats no number at all.
func countEmbeddedFiles(fsys fs.FS) (files int, total int64, walkErr string) {
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		files++
		if info, statErr := d.Info(); statErr == nil {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		walkErr = err.Error()
	}
	return files, total, walkErr
}

// ── host.widgets ───────────────────────────────────────────────────────────

// widgetAsset is one file that carries a widget.
type widgetAsset struct {
	Rel string // path within its own scope (static root, or plugin static root)
	URL string // the URL this process serves it at, without the cache-buster
	// Token is the `?v=` value layouts.AssetURL emits for URL. It is captured
	// here rather than re-derived at render time so the served token and the
	// bytes it is compared against are read in the same pass.
	Token     string
	Size      int64
	SHA256    string
	ModTime   time.Time
	HasMod    bool // false for embedded assets: embed.FS records no modification time
	Principal bool // the .js that IS the widget, as opposed to a same-named companion
	Err       string
}

// widgetEntry is one widget: a name, where it came from, and the asset(s) that
// carry it. Scope is carried per entry rather than inferred from the name
// because the same name could exist in both places, and reporting the wrong
// storage mechanism is precisely the mistake this diagnostic exists to prevent.
type widgetEntry struct {
	Name   string
	Scope  string // human label, e.g. "on-disk" or "embedded (calendar)"
	Assets []widgetAsset
}

// renderHostWidgets wires the live sources into the pure renderer.
func renderHostWidgets(arg string) string {
	return renderHostWidgetsFrom(resolveStaticRoot(), embeddedAssetsFn, arg, layouts.AssetURL, layouts.BuildToken())
}

// renderHostWidgetsFrom is the pure renderer. The static root, the embedded
// provider, AssetURL and the build token are all injected: a `go test` binary's
// working directory is its own package dir, so none of the interesting states
// (a missing static root, an unwired provider, a served token that no longer
// matches the bytes) is reachable through the live functions.
func renderHostWidgetsFrom(root staticRoot, provider func() []EmbeddedAssetSet, filter string, assetURL func(string) string, buildToken string) string {
	var b strings.Builder
	b.WriteString("## host.widgets — widget assets this process serves\n\n")
	b.WriteString("**Widgets carry no version number.** Nothing declares one and there is no server-side widget registry to consult — a widget is a JS file that registers itself with `boot.js` when the browser mounts a `data-widget` element. So the identity below is a content fingerprint (sha256 of the bytes being served) plus, where the storage mechanism records one, an mtime. Two builds serving the same sha are the same widget; two builds serving different shas are not.\n\n")

	filter = strings.TrimSpace(filter)
	onDisk, diskNote := onDiskWidgets(root, assetURL)
	embedded, embedNote := embeddedWidgets(provider, assetURL)

	// A section that could not be walked at all reports its REASON and is not
	// counted as scanned, so a filter that matched nothing is never blamed for
	// an inventory that was never read.
	scanned := 0
	if diskNote == "" {
		scanned++
	}
	if embedNote == "" {
		scanned++
	}

	shown := 0
	shown += writeWidgetSection(&b, "Central, on-disk — `<static root>/"+widgetsSubdir+"`",
		diskNote, filterWidgets(onDisk, filter), buildToken)
	shown += writeWidgetSection(&b, "Plugin-embedded — inside the executable",
		embedNote, filterWidgets(embedded, filter), buildToken)

	if filter != "" && shown == 0 && scanned > 0 {
		fmt.Fprintf(&b, "_No widget name contains `%s`. The filter is a plain substring over the widget name (the JS filename without its extension), e.g. `calendar`, `editor`, `map`. Run with no argument for the full list._\n\n", filter)
	}
	writeWidgetNote(&b, filter)
	return b.String()
}

// writeWidgetSection renders one storage mechanism's widgets and returns how
// many it printed. A section with nothing in it still prints its heading and
// its reason — an operator scanning for a widget needs to see that the place it
// would live was looked at.
func writeWidgetSection(b *strings.Builder, heading, note string, entries []widgetEntry, buildToken string) int {
	fmt.Fprintf(b, "### %s\n\n", heading)
	if note != "" {
		b.WriteString(note + "\n\n")
		return 0
	}
	if len(entries) == 0 {
		b.WriteString("_Nothing here matched._\n\n")
		return 0
	}
	for _, e := range entries {
		writeWidgetEntry(b, e, buildToken)
	}
	b.WriteString("\n")
	return len(entries)
}

// writeWidgetEntry renders one widget: a compact line for the script that IS
// the widget, plus a sub-line for each same-named companion asset. The CSS is
// listed separately rather than folded into the script's fingerprint because a
// build can change the stylesheet alone and the JS sha will not move — an
// operator comparing only the script would call that deploy a no-op.
func writeWidgetEntry(b *strings.Builder, e widgetEntry, buildToken string) {
	for _, a := range e.Assets {
		if !a.Principal {
			continue
		}
		if a.Err != "" {
			fmt.Fprintf(b, "- **`%s`** — %s · `%s` — **unreadable**: %s\n", e.Name, e.Scope, a.Rel, a.Err)
			break
		}
		fmt.Fprintf(b, "- **`%s`** — %s · `%s` · %s · %s · `?v=%s` %s\n",
			e.Name, e.Scope, orUnrecorded(a.SHA256), humanBytes(a.Size),
			widgetModTime(a), orUnrecorded(a.Token), tokenVerdict(a.Token, a.SHA256, buildToken))
		break
	}
	for _, a := range e.Assets {
		if a.Principal {
			continue
		}
		if a.Err != "" {
			fmt.Fprintf(b, "  - companion `%s` — **unreadable**: %s\n", a.Rel, a.Err)
			continue
		}
		fmt.Fprintf(b, "  - companion `%s` — `%s` · %s · `?v=%s`\n", a.Rel, orUnrecorded(a.SHA256), humanBytes(a.Size), orUnrecorded(a.Token))
	}
}

// widgetModTime renders the mtime, or states why there is none. An embedded
// asset reports the zero time, and printing "0001-01-01" would be a fabricated
// field dressed as data.
func widgetModTime(a widgetAsset) string {
	if !a.HasMod {
		return "mtime n/a _(embedded — `embed.FS` records none)_"
	}
	return a.ModTime.UTC().Format(time.RFC3339)
}

// filterWidgets applies the optional name substring.
func filterWidgets(entries []widgetEntry, filter string) []widgetEntry {
	if filter == "" {
		return entries
	}
	out := make([]widgetEntry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(e.Name, filter) {
			out = append(out, e)
		}
	}
	return out
}

// writeWidgetNote is unconditional. The two facts it carries — that this walked
// directories because no registry exists, and that the Go widget packages are
// invisible from here — are the ones a reader would otherwise supply wrongly
// from their own assumptions.
func writeWidgetNote(b *strings.Builder, filter string) {
	b.WriteString("### How this list was produced, and what it cannot see\n\n")
	b.WriteString("- **There is no widget registry to consult, so this WALKED two known directories**: `<static root>/" + widgetsSubdir + "` on disk, and the `" + embeddedJSSubdir + "` directory of every plugin filesystem compiled into this binary. A widget served from anywhere else would not appear here.\n")
	b.WriteString("- **A widget's assets can live in either place, and the calendar widget lives only in the second.** `//go:embed`-ed bytes are inside the executable: `ls` and `grep` over the container filesystem will never find them, so an empty grep for a plugin widget is the expected result and not evidence of missing code.\n")
	b.WriteString("- **The Go widget packages under `internal/widgets/` are not listed and cannot be.** They render server-side Templ, ship no assets of their own, and the source tree does not exist in a deployment — so nothing at runtime can enumerate them. Their absence here is a limit of the vantage point, not a finding.\n")
	b.WriteString("- **Companion assets are matched by name** (`" + embeddedJSSubdir + "<name>.js` ↔ `css/<name>.css` within the same scope). A stylesheet that does not follow that convention belongs to no widget here and shows up in `host.assets` / `host.embedded` instead.\n")
	if filter == "" {
		b.WriteString("- Pass a name substring to narrow this list; pass the same name to `host.asset-contains` or `host.embedded-contains` to check for a specific marker inside the file.\n")
	}
}

// onDiskWidgets collects the central widget scripts. It reuses walkStaticFiles
// rather than walking again, so the size/sha/mtime shown here are produced by
// the exact code host.assets uses — one walker, one set of answers.
func onDiskWidgets(root staticRoot, assetURL func(string) string) ([]widgetEntry, string) {
	if root.CWDErr != "" {
		return nil, fmt.Sprintf("_Working directory could not be determined (%s), so the static root cannot be resolved and no on-disk widget can be listed._", root.CWDErr)
	}
	if !root.Exists {
		return nil, fmt.Sprintf("_Static root `%s` is not usable: %s. No on-disk widget can be listed — and every on-disk asset URL this process emits will be falling back to the per-build token. See `host.assets`._", root.Path, orUnrecorded(root.StatErr))
	}
	files, _ := walkStaticFiles(root.Path)
	index := make(map[string]assetFile, len(files))
	for _, f := range files {
		index[f.Rel] = f
	}

	var out []widgetEntry
	for _, f := range files {
		if !strings.HasPrefix(f.Rel, widgetsSubdir) || strings.ToLower(path.Ext(f.Rel)) != ".js" {
			continue
		}
		name := strings.TrimSuffix(path.Base(f.Rel), path.Ext(f.Rel))
		e := widgetEntry{Name: name, Scope: "on-disk"}
		e.Assets = append(e.Assets, diskAsset(f, assetURL, true))
		if comp, ok := index["css/"+name+".css"]; ok {
			e.Assets = append(e.Assets, diskAsset(comp, assetURL, false))
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, ""
}

// diskAsset converts a walked file into a widget asset, resolving the URL the
// app actually serves it at through layouts.AssetURL — never a second
// reimplementation of the cache-busting rule.
func diskAsset(f assetFile, assetURL func(string) string, principal bool) widgetAsset {
	url := layouts.StaticURLPrefix + f.Rel
	return widgetAsset{
		Rel:       f.Rel,
		URL:       url,
		Token:     versionToken(assetURL, url),
		Size:      f.Size,
		SHA256:    f.SHA256,
		ModTime:   f.ModTime,
		HasMod:    !f.ModTime.IsZero(),
		Principal: principal,
		Err:       f.Err,
	}
}

// embeddedWidgets collects the widget scripts compiled into the binary.
func embeddedWidgets(provider func() []EmbeddedAssetSet, assetURL func(string) string) ([]widgetEntry, string) {
	if provider == nil {
		return nil, "_Provider not wired (the app layer did not inject the plugin registry at startup), so the widgets compiled into this binary cannot be listed. **This is not \"there are none\"** — it is \"nothing told this diagnostic about them\"._"
	}
	sets := provider()
	if len(sets) == 0 {
		return nil, "_The provider is wired and reports no plugin registering a static filesystem, so no widget is served from inside the binary in this build. Unlike the not-wired case, this IS an answer._"
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].Slug < sets[j].Slug })

	var out []widgetEntry
	for _, s := range sets {
		if s.FS == nil {
			continue // reported by host.plugins / host.embedded; not a widget question
		}
		index := embeddedIndex(s.FS)
		names := make([]string, 0, len(index))
		for rel := range index {
			if strings.HasPrefix(rel, embeddedJSSubdir) && strings.ToLower(path.Ext(rel)) == ".js" {
				names = append(names, rel)
			}
		}
		sort.Strings(names)
		for _, rel := range names {
			name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
			e := widgetEntry{Name: name, Scope: "embedded in `" + s.Slug + "`"}
			e.Assets = append(e.Assets, embeddedAsset(s, rel, index[rel], assetURL, true))
			if comp, ok := index["css/"+name+".css"]; ok {
				e.Assets = append(e.Assets, embeddedAsset(s, "css/"+name+".css", comp, assetURL, false))
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Scope < out[j].Scope
	})
	return out, ""
}

// embeddedFile is one file read out of a plugin's embedded filesystem.
type embeddedFile struct {
	Size   int64
	SHA256 string
	Err    string
}

// embeddedIndex fingerprints every file in one embedded filesystem, keyed by
// its path within that filesystem.
func embeddedIndex(fsys fs.FS) map[string]embeddedFile {
	index := map[string]embeddedFile{}
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, p)
		if readErr != nil {
			index[p] = embeddedFile{Err: readErr.Error()}
			return nil
		}
		index[p] = embeddedFile{Size: int64(len(data)), SHA256: shortSHA(data)}
		return nil
	})
	return index
}

// embeddedAsset converts an indexed embedded file into a widget asset.
func embeddedAsset(s EmbeddedAssetSet, rel string, f embeddedFile, assetURL func(string) string, principal bool) widgetAsset {
	url := strings.TrimSuffix(s.URLPrefix, "/") + "/" + rel
	return widgetAsset{
		Rel:       rel,
		URL:       url,
		Token:     versionToken(assetURL, url),
		Size:      f.Size,
		SHA256:    f.SHA256,
		Principal: principal,
		Err:       f.Err,
		// HasMod stays false: embed.FS records no modification times at all.
	}
}

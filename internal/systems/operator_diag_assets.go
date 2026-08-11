// Package systems — operator_diag_assets.go is the "where did my CSS/JS
// actually go?" half of the host diagnostics. host.build (operator_diag_host.go)
// answers which BINARY is running; these answer which ASSETS that binary is
// serving, and they answer it for both places Chronicle keeps them.
//
// WHY it exists. Chronicle serves its own front-end from TWO storage mechanisms
// that look nothing alike from a shell:
//
//   - the on-disk static root, a bare relative path ("static") resolved against
//     the PROCESS WORKING DIRECTORY — /app/static in the container, <repo>/static
//     in dev. Nothing configures it; it is hardcoded identically in
//     internal/app/app.go (e.Static) and internal/templates/layouts/assets.go
//     (os.DirFS). A diagnostic that hardcoded either absolute path would be
//     wrong in the other environment and, worse, would never surface the real
//     failure mode: the app looking in a directory that is not there.
//
//   - each plugin's //go:embed-ed static FS, which exists ONLY inside the
//     binary. On 2026-08-11 an operator grepped /app/static for a calendar
//     feature's assets, found nothing, and read that as missing code. The code
//     was present and serving — it was compiled in. No `ls`, no `grep`, no
//     `find` over the container filesystem can ever see those bytes. host.embedded
//     is the only way to look at them, which is why its description says so in
//     plain words rather than assuming the reader knows.
//
// The `?v=` token. Every asset URL a template emits is cache-busted by
// layouts.AssetURL. These diagnostics call THAT function rather than
// reimplementing its hashing, because a second copy of a cache-busting rule is
// a copy that can disagree with the one the browser is actually served — and it
// would disagree precisely while an operator was using it to decide whether a
// browser is holding a stale file. The comparison between the token AssetURL
// emits and the sha256 of the bytes on disk right now is itself the finding: a
// mismatch means either the file changed under a running process (the digest is
// memoised for the process lifetime by design) or the asset never resolved at
// all and fell back to the per-build token.
package systems

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// staticDirName is the literal relative path Chronicle's two static mounts use.
// It is a constant here for the same reason it is duplicated there: there is no
// config field and no environment variable for it anywhere in the tree, so the
// only honest resolution is "this literal, joined to the working directory".
const staticDirName = "static"

// maxAssetRows bounds the no-argument host.assets listing. A bound is fine; a
// bound nobody is told about is the defect — when it trips, the output says so
// and names the argument that narrows the walk.
const maxAssetRows = 400

// EmbeddedAssetSet is one plugin's //go:embed-ed static filesystem as the app
// layer sees it: the plugin slug, the URL prefix App.mountPluginStatic() serves
// it under, and the FS itself (already rooted at the plugin's static/ subdir by
// echo.MustSubFS, so a walk yields "js/foo.js", not "static/js/foo.js").
//
// Injected rather than imported: internal/systems must not import the plugins
// packages, so the app layer builds this list from its own registry. Same
// dependency inversion as SetInstalledPackagesProvider.
type EmbeddedAssetSet struct {
	Slug      string
	URLPrefix string
	FS        fs.FS
}

// embeddedAssetsFn returns the registered plugins' embedded static filesystems,
// or nil when the app layer has not wired it — in which case the diagnostics
// SAY "provider not wired" rather than reporting an empty inventory, because
// "no embedded assets" and "nobody told me about the embedded assets" are
// opposite conclusions for someone hunting missing code.
var embeddedAssetsFn func() []EmbeddedAssetSet

// SetEmbeddedAssetsProvider wires the app's plugin registry into the embedded
// asset diagnostics. Called once at startup from the app wiring.
func SetEmbeddedAssetsProvider(fn func() []EmbeddedAssetSet) { embeddedAssetsFn = fn }

// hostAssetsDiagnostic lists the on-disk static root the running process is
// actually serving from.
func hostAssetsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:  "host.assets",
		Title: "On-disk static assets: what is there, and what `?v=` the app serves for each",
		Desc:  "THE 'is my new CSS/JS actually being served?' check. Resolves the static root the way the process does (working directory + `static`, never a hardcoded /app/static), then per file: size, sha256[:16], mtime, and the exact `?v=` cache-buster layouts.AssetURL emits — flagging any file whose served token does NOT match its bytes on disk. No argument = css/js only (the files that carry features) plus a count of the rest; PASS A PATH SUBSTRING to filter, which is almost always what you want.",
		ArgHint: "[<path-substring>]",
		// FullDump because the no-argument listing is genuinely large: measured
		// against this repo's static root it is 89 rows / ~15 KB, the same order
		// as system.health. The gate applies to BATCHES only (operator_batch.go),
		// so the admin page's Run button is unaffected — it just stops 15 KB of
		// asset inventory being swept into an assistant's context unasked, when
		// a substring argument answers the actual question in a tenth the size.
		FullDump: true,
		Run:      renderHostAssets,
	}
}

// hostAssetContainsDiagnostic is the content check for one on-disk asset — the
// same question system.file-contains answers for a system package's files.
func hostAssetContainsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "host.asset-contains",
		Title:   "Check ONE on-disk static file for marker string(s)",
		Desc:    "Reads one file under the static root (clamped: `..` traversal and absolute paths are refused, and the output says which rule refused them) and reports whether each comma-separated marker is present, plus the file's sha256[:16] and mtime. Confirms the served build's CONTENT, not just its hash — e.g. `css/calendar-block.css:moonpick`.",
		ArgHint: "<relative-path>:<marker[,marker2]>",
		Run:     renderHostAssetContains,
	}
}

// hostEmbeddedDiagnostic is the one that closes the 2026-08-11 trap.
func hostEmbeddedDiagnostic() Diagnostic {
	return Diagnostic{
		Name:  "host.embedded",
		Title: "Plugin assets compiled INTO the binary (invisible to ls and grep)",
		Desc:  "Lists every //go:embed-ed plugin static file the running binary serves, with size, sha256[:16] and the URL it is served at. THESE FILES ARE NOT ON DISK: they exist only inside the executable, so `ls /app/static` will not list them and `grep -r` over the container filesystem will find nothing — an empty grep here is not evidence of missing code. Use this whenever a plugin feature looks absent from the filesystem. Optional argument filters to one plugin slug.",
		ArgHint: "[<plugin-slug>]",
		Run:     renderHostEmbedded,
	}
}

// hostEmbeddedContainsDiagnostic is host.asset-contains for bytes that have no
// path on the filesystem to point a grep at.
func hostEmbeddedContainsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "host.embedded-contains",
		Title:   "Check ONE embedded plugin asset for marker string(s)",
		Desc:    "The marker check for assets compiled into the binary — the only way to ask 'does the shipped build of this plugin file contain X?', since the bytes are not on disk to grep. Path is relative to the plugin's static root (e.g. `calendar:js/gm_panel.js:moonPhase`); `..` and absolute paths are refused.",
		ArgHint: "<plugin-slug>:<relative-path>:<marker[,marker2]>",
		Run:     renderHostEmbeddedContains,
	}
}

// ── static root resolution ─────────────────────────────────────────────────

// staticRoot is where the process believes its on-disk assets live, plus
// whether that belief holds. Every field that can fail carries its error rather
// than collapsing to an empty string, so "could not determine the working
// directory" never renders as "the static root is `static`".
type staticRoot struct {
	CWD     string
	CWDErr  string
	Path    string // absolute, "" when the working directory could not be read
	Exists  bool
	StatErr string
}

// resolveStaticRoot reproduces the resolution the app performs at runtime:
// the literal relative "static", resolved against the process working
// directory. It deliberately does NOT consult any config, because none exists —
// inventing one here would describe a Chronicle that does not ship.
func resolveStaticRoot() staticRoot {
	r := staticRoot{}
	cwd, err := os.Getwd()
	if err != nil {
		r.CWDErr = err.Error()
		return r
	}
	r.CWD = cwd
	r.Path = filepath.Join(cwd, staticDirName)
	info, err := os.Stat(r.Path)
	switch {
	case err != nil:
		r.StatErr = err.Error()
	case !info.IsDir():
		r.StatErr = "exists but is not a directory"
	default:
		r.Exists = true
	}
	return r
}

// assetFile is one on-disk static file as the diagnostic sees it.
type assetFile struct {
	Rel     string // slash-separated, relative to the static root
	Size    int64
	SHA256  string // first 16 hex chars of the content hash, or "too-large"
	ModTime time.Time
	Err     string // set when the file could not be read; SHA256 is then empty
}

// walkStaticFiles fingerprints every regular file under root. Best effort by
// design: an unreadable file is REPORTED as unreadable rather than aborting the
// walk, because a diagnostic that returns nothing when one file is bad is
// useless exactly when something is wrong.
func walkStaticFiles(root string) ([]assetFile, error) {
	var out []assetFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable directory: skip, keep walking
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		f := assetFile{Rel: filepath.ToSlash(rel)}
		info, infoErr := d.Info()
		if infoErr != nil {
			f.Err = infoErr.Error()
			out = append(out, f)
			return nil
		}
		f.Size = info.Size()
		f.ModTime = info.ModTime()
		switch {
		case info.Size() > maxFingerprintBytes:
			f.SHA256 = "too-large"
		default:
			data, readErr := os.ReadFile(p)
			if readErr != nil {
				f.Err = readErr.Error()
			} else {
				sum := sha256.Sum256(data)
				f.SHA256 = hex.EncodeToString(sum[:])[:16]
			}
		}
		out = append(out, f)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, err
}

// ── host.assets ────────────────────────────────────────────────────────────

// renderHostAssets wires the live sources into the pure renderer.
func renderHostAssets(arg string) string {
	return renderHostAssetsFrom(resolveStaticRoot(), arg, layouts.AssetURL, layouts.BuildToken())
}

// renderHostAssetsFrom is the pure renderer. assetURL and buildToken are
// injected so a test can drive the token-divergence branches, which are the
// interesting ones and are otherwise unreachable: a `go test` binary's working
// directory is the package dir, so the real AssetURL cannot resolve anything.
func renderHostAssetsFrom(root staticRoot, filter string, assetURL func(string) string, buildToken string) string {
	var b strings.Builder
	b.WriteString("## host.assets — on-disk static assets this process serves\n\n")

	if !renderStaticRootHeader(&b, root) {
		return b.String()
	}

	files, walkErr := walkStaticFiles(root.Path)
	if walkErr != nil {
		fmt.Fprintf(&b, "_Walk reported an error (listing may be partial): %v_\n\n", walkErr)
	}
	if len(files) == 0 {
		b.WriteString("_The static root exists but contains no files. Nothing on disk is being served._\n")
		return b.String()
	}

	filter = strings.TrimSpace(filter)
	shown, skipped := selectAssets(files, filter)
	if filter == "" {
		fmt.Fprintf(&b, "No path filter given, so this lists **css/js only** — the files that carry features. %s\n\n", summariseSkipped(skipped))
	} else {
		fmt.Fprintf(&b, "Filtered to paths containing `%s`: **%d of %d** file(s) match.\n\n", filter, len(shown), len(files))
	}
	if len(shown) == 0 {
		fmt.Fprintf(&b, "_No files matched. The filter is a plain substring over the path relative to the static root (e.g. `calendar`, `js/widgets`, `.css`) — there are %d file(s) here in total. Note that generated assets (Tailwind's `css/app.css`) exist only in a built deployment, not in a source checkout._\n", len(files))
		return b.String()
	}

	truncated := false
	if len(shown) > maxAssetRows {
		shown, truncated = shown[:maxAssetRows], true
	}

	b.WriteString("Each line: `path` — size · sha256[:16] of the bytes on disk · mtime · the `?v=` token the app serves.\n\n")
	diverged := 0
	for _, f := range shown {
		token := versionToken(assetURL, layouts.StaticURLPrefix+f.Rel)
		verdict := tokenVerdict(token, f.SHA256, buildToken)
		if strings.HasPrefix(verdict, "⚠️") {
			diverged++
		}
		if f.Err != "" {
			fmt.Fprintf(&b, "- `%s` — %d · **unreadable**: %s\n", f.Rel, f.Size, f.Err)
			continue
		}
		fmt.Fprintf(&b, "- `%s` — %d · `%s` · %s · `?v=%s` %s\n",
			f.Rel, f.Size, f.SHA256, f.ModTime.UTC().Format(time.RFC3339), token, verdict)
	}
	if truncated {
		fmt.Fprintf(&b, "\n**Listing truncated at %d rows** — there are more matches. Pass a path substring argument to narrow the walk; nothing was silently dropped from the counts above.\n", maxAssetRows)
	}

	renderAssetTokenNote(&b, diverged, len(shown))
	return b.String()
}

// renderStaticRootHeader prints where the process looks for on-disk assets and
// returns whether a walk is worth attempting. The path is RESOLVED, never
// assumed: there is no config field and no environment variable for it, so
// "working directory + static" is the only truthful answer — and printing the
// working directory is what makes "the app is looking in a directory that isn't
// there" visible at all.
func renderStaticRootHeader(b *strings.Builder, root staticRoot) bool {
	b.WriteString("### Where this process looks for static assets\n\n")
	if root.CWDErr != "" {
		fmt.Fprintf(b, "- working directory: **could not be determined** (%s)\n", root.CWDErr)
		b.WriteString("- The static root cannot be resolved without it. Nothing further can be reported here.\n")
		return false
	}
	fmt.Fprintf(b, "- working directory: `%s`\n", root.CWD)
	fmt.Fprintf(b, "- static root: `%s`\n", root.Path)
	b.WriteString("- Chronicle has no config field and no environment variable for this path. Both mounts hardcode the bare relative string `static`, so the root is whatever the working directory makes it: `/app/static` in the container (`WORKDIR /app`), `<repo>/static` in dev.\n")
	if !root.Exists {
		fmt.Fprintf(b, "- **THE STATIC ROOT IS NOT USABLE**: %s\n", orUnrecorded(root.StatErr))
		b.WriteString("- Every on-disk asset URL will fall back to the per-build `?v=` token and every request for one will 404. If this process is not the container, check what directory it was started from.\n")
		return false
	}
	b.WriteString("- root exists and is readable.\n\n")
	return true
}

// selectAssets applies the argument: a substring filter when given, otherwise
// css/js only (the files that carry features). Returns the selection plus the
// files it left out, so the caller can summarise rather than silently hide them.
func selectAssets(files []assetFile, filter string) (shown, skipped []assetFile) {
	for _, f := range files {
		keep := false
		if filter != "" {
			keep = strings.Contains(f.Rel, filter)
		} else {
			ext := strings.ToLower(path.Ext(f.Rel))
			keep = ext == ".css" || ext == ".js"
		}
		if keep {
			shown = append(shown, f)
		} else {
			skipped = append(skipped, f)
		}
	}
	return shown, skipped
}

// summariseSkipped counts the unlisted files by extension so their absence is a
// stated fact with a number rather than an omission a reader has to notice.
func summariseSkipped(skipped []assetFile) string {
	if len(skipped) == 0 {
		return "Nothing else is present."
	}
	byExt := map[string]int{}
	for _, f := range skipped {
		ext := strings.ToLower(path.Ext(f.Rel))
		if ext == "" {
			ext = "(no extension)"
		}
		byExt[ext]++
	}
	exts := make([]string, 0, len(byExt))
	for e := range byExt {
		exts = append(exts, e)
	}
	sort.Slice(exts, func(i, j int) bool {
		if byExt[exts[i]] != byExt[exts[j]] {
			return byExt[exts[i]] > byExt[exts[j]]
		}
		return exts[i] < exts[j]
	})
	parts := make([]string, 0, len(exts))
	for _, e := range exts {
		parts = append(parts, fmt.Sprintf("%d %s", byExt[e], e))
	}
	return fmt.Sprintf("%d other file(s) not listed: %s. Pass a path substring to see them.", len(skipped), strings.Join(parts, ", "))
}

// renderAssetTokenNote explains how to read the `?v=` column. It is printed
// unconditionally, including when nothing diverged: an operator reading a clean
// run needs to know what "clean" ruled out, and a note that only appears on
// failure teaches nothing on the run where the answer is "not this".
func renderAssetTokenNote(b *strings.Builder, diverged, total int) {
	b.WriteString("\n### How to read the `?v=` column\n\n")
	if diverged == 0 {
		fmt.Fprintf(b, "- **%d of %d file(s) agree**: every served token is the first 10 hex chars of that file's sha256 as it is on disk right now. A browser holding an old copy under one of these URLs is holding it under an OLD `?v=`, so a reload will fetch the new bytes.\n", total, total)
	} else {
		fmt.Fprintf(b, "- **%d of %d file(s) do NOT agree** (marked ⚠️ above). Read each marker literally:\n", diverged, total)
	}
	b.WriteString("- `BUILD-TOKEN FALLBACK` — layouts.AssetURL could not open the file through any registered resolver, so it used the per-build token instead of a content hash. The file is on disk (this diagnostic just hashed it) but the app is not finding it there. For on-disk assets the cause is almost always a working directory other than the one the static root is relative to — compare the working directory printed above against where the file actually is.\n")
	b.WriteString("- `STALE` — the app is serving a digest computed from earlier bytes. Digests are memoised for the process lifetime on purpose (static assets are not supposed to change under a running binary), so this means the file was replaced while the process ran — a live edit or a bind-mounted volume. Browsers will keep the old copy until the process restarts.\n")
	b.WriteString("- `NOT VERSIONED` — AssetURL returned the path unchanged, which it does only for non-`/static/` paths or paths already carrying a query string. Unexpected here.\n")
	b.WriteString("- This column reports what THIS process will emit. It says nothing about what any particular browser has cached; for that, compare against the `?v=` in the page source (see the `probes` diagnostic).\n")
}

// ── host.asset-contains ────────────────────────────────────────────────────

// renderHostAssetContains wires the live static root into the pure renderer.
func renderHostAssetContains(arg string) string {
	return renderHostAssetContainsFrom(resolveStaticRoot(), arg)
}

// renderHostAssetContainsFrom is the pure renderer.
func renderHostAssetContainsFrom(root staticRoot, arg string) string {
	var b strings.Builder
	b.WriteString("## host.asset-contains\n\n")

	// SplitN with 2 keeps colons inside a marker (a URL, a timestamp) intact —
	// same reason system.file-contains splits with 3.
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		b.WriteString("_Usage: `<relative-path>:<marker[,marker2,...]>` — path is relative to the static root, e.g. `css/calendar-block.css:moonpick`._\n")
		return b.String()
	}
	rawPath, markerCSV := strings.TrimSpace(parts[0]), parts[1]

	if root.CWDErr != "" {
		fmt.Fprintf(&b, "_Working directory could not be determined (%s), so the static root cannot be resolved._\n", root.CWDErr)
		return b.String()
	}
	if !root.Exists {
		fmt.Fprintf(&b, "_Static root `%s` is not usable: %s. Nothing on disk can be read._\n", root.Path, orUnrecorded(root.StatErr))
		return b.String()
	}

	relPath, strippedPrefix := trimStaticPrefix(rawPath)
	full, refusal := clampStaticRel(root.Path, relPath)
	if refusal != "" {
		fmt.Fprintf(&b, "- **Refused `%s`** — %s.\n", rawPath, refusal)
		b.WriteString("- Nothing was read. This diagnostic only reads inside the static root, so a path that leaves it is rejected rather than resolved.\n")
		return b.String()
	}
	if strippedPrefix != "" {
		fmt.Fprintf(&b, "_Stripped the leading `%s` from the argument — paths here are relative to the static root._\n\n", strippedPrefix)
	}

	info, statErr := os.Stat(full)
	switch {
	case statErr != nil:
		fmt.Fprintf(&b, "- `%s` — **not found** under `%s` (%s).\n", relPath, root.Path, statErr)
		b.WriteString("- If you expected this file to be compiled into the binary rather than on disk, run `host.embedded` instead: plugin assets are `//go:embed`-ed and never appear under the static root.\n")
		return b.String()
	case info.IsDir():
		fmt.Fprintf(&b, "- `%s` is a directory, not a file. Use `host.assets %s` to list what is under it.\n", relPath, relPath)
		return b.String()
	case info.Size() > maxFingerprintBytes:
		fmt.Fprintf(&b, "- `%s` is %d bytes, over the %d-byte read cap — not scanned.\n", relPath, info.Size(), int64(maxFingerprintBytes))
		return b.String()
	}

	data, readErr := os.ReadFile(full)
	if readErr != nil {
		fmt.Fprintf(&b, "- `%s` could not be read: %s\n", relPath, readErr)
		return b.String()
	}
	sum := sha256.Sum256(data)
	fmt.Fprintf(&b, "read `%s` — %d bytes · `%s` · %s\n\n",
		full, len(data), hex.EncodeToString(sum[:])[:16], info.ModTime().UTC().Format(time.RFC3339))
	markerReport(&b, string(data), markerCSV)
	return b.String()
}

// ── host.embedded ──────────────────────────────────────────────────────────

// renderHostEmbedded wires the injected provider into the pure renderer.
func renderHostEmbedded(arg string) string {
	return renderHostEmbeddedFrom(embeddedAssetsFn, arg, layouts.AssetURL, layouts.BuildToken())
}

// renderHostEmbeddedFrom is the pure renderer. The PROVIDER (not its result) is
// injected so the unwired case — a nil func, which must degrade to a message
// and not a nil-deref — is directly testable.
func renderHostEmbeddedFrom(provider func() []EmbeddedAssetSet, slugFilter string, assetURL func(string) string, buildToken string) string {
	var b strings.Builder
	b.WriteString("## host.embedded — plugin assets compiled INTO this binary\n\n")
	b.WriteString("**These files are not on disk.** They are `//go:embed`-ed into the executable and served from memory, so `ls` and `grep` over the container filesystem will never find them. An empty grep for one of these is not evidence that the code is missing — it is the expected result.\n\n")

	sets, ok := embeddedSets(&b, provider)
	if !ok {
		return b.String()
	}

	slugFilter = strings.TrimSpace(slugFilter)
	matched := 0
	for _, s := range sets {
		if slugFilter != "" && s.Slug != slugFilter {
			continue
		}
		matched++
		renderEmbeddedSet(&b, s, assetURL, buildToken)
	}
	if matched == 0 {
		slugs := make([]string, 0, len(sets))
		for _, s := range sets {
			slugs = append(slugs, "`"+s.Slug+"`")
		}
		fmt.Fprintf(&b, "_No registered plugin with slug `%s`. Plugins that embed static assets: %s._\n", slugFilter, strings.Join(slugs, ", "))
		return b.String()
	}

	b.WriteString("### Notes\n\n")
	b.WriteString("- **No mtime column, deliberately.** `embed.FS` records no modification times — every embedded file reports the zero time — so an mtime here would be a fabricated field. The sha256 is the only content identity these files have, which is what makes it the thing to compare between two builds.\n")
	b.WriteString("- Only plugins that registered a `StaticFS` appear. A plugin whose front-end is Templ + CSS with no separate asset files has nothing to list here and is still perfectly present in the binary — absence from this list is not absence from the build.\n")
	b.WriteString("- To confirm the CONTENT of one of these, use `host.embedded-contains <slug>:<path>:<marker>` — there is no file on disk to grep.\n")
	b.WriteString("- A `BUILD-TOKEN FALLBACK` in the `?v=` column here does NOT mean a missing working directory (embedded assets have none). It means the plugin's filesystem was mounted for serving but never registered for content hashing, so its URLs bust on every deploy instead of on every change. Startup does both in one place (`App.mountPluginStatic`), so seeing this is a real defect worth reporting.\n")
	return b.String()
}

// embeddedSets resolves the provider, writing the "why there is nothing to show"
// explanation itself. The nil-provider and empty-result cases are printed
// SEPARATELY on purpose: for someone hunting code that looks missing, "nobody
// wired the inventory" and "the inventory is genuinely empty" point at opposite
// conclusions, and collapsing them into one message would answer the question
// wrongly at the exact moment it matters.
func embeddedSets(b *strings.Builder, provider func() []EmbeddedAssetSet) ([]EmbeddedAssetSet, bool) {
	if provider == nil {
		b.WriteString("_Provider not wired (the app layer did not inject the plugin registry at startup), so this binary's embedded assets cannot be listed. This says nothing about whether embedded assets exist — only that nothing told this diagnostic about them._\n")
		return nil, false
	}
	sets := provider()
	if len(sets) == 0 {
		b.WriteString("_The provider is wired and reports no plugin registering a static filesystem. Unlike the not-wired case above, this IS an answer: no plugin in this build serves embedded assets._\n")
		return nil, false
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].Slug < sets[j].Slug })
	return sets, true
}

// renderEmbeddedSet walks one plugin's embedded filesystem.
func renderEmbeddedSet(b *strings.Builder, s EmbeddedAssetSet, assetURL func(string) string, buildToken string) {
	fmt.Fprintf(b, "### `%s` — served at `%s`\n\n", s.Slug, s.URLPrefix)
	if s.FS == nil {
		b.WriteString("- _registered with a nil filesystem — nothing is served under this prefix._\n\n")
		return
	}
	files := 0
	var total int64
	walkErr := fs.WalkDir(s.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil // keep walking; a partial inventory beats none
		}
		data, readErr := fs.ReadFile(s.FS, p)
		if readErr != nil {
			fmt.Fprintf(b, "- `%s` — **unreadable**: %s\n", p, readErr)
			files++
			return nil
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])[:16]
		url := strings.TrimSuffix(s.URLPrefix, "/") + "/" + p
		token := versionToken(assetURL, url)
		fmt.Fprintf(b, "- `%s` — %d · `%s` · `?v=%s` %s\n", p, len(data), sha, token, tokenVerdict(token, sha, buildToken))
		files++
		total += int64(len(data))
		return nil
	})
	if walkErr != nil {
		fmt.Fprintf(b, "- _walk error (listing may be partial): %v_\n", walkErr)
	}
	if files == 0 {
		b.WriteString("- _registered, but the filesystem contains no files._\n")
	} else {
		fmt.Fprintf(b, "\n**%d file(s), %s embedded in the binary.**\n", files, humanBytes(total))
	}
	b.WriteString("\n")
}

// ── host.embedded-contains ─────────────────────────────────────────────────

// renderHostEmbeddedContains wires the injected provider into the pure renderer.
func renderHostEmbeddedContains(arg string) string {
	return renderHostEmbeddedContainsFrom(embeddedAssetsFn, arg)
}

// renderHostEmbeddedContainsFrom is the pure renderer.
func renderHostEmbeddedContainsFrom(provider func() []EmbeddedAssetSet, arg string) string {
	var b strings.Builder
	b.WriteString("## host.embedded-contains\n\n")

	// SplitN with 3 keeps colons inside a marker intact, matching
	// system.file-contains.
	parts := strings.SplitN(arg, ":", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		b.WriteString("_Usage: `<plugin-slug>:<relative-path>:<marker[,marker2,...]>` — path is relative to the plugin's embedded static root, e.g. `calendar:js/gm_panel.js:moonPhase`. Run `host.embedded` for the slugs and paths._\n")
		return b.String()
	}
	slug, rawPath, markerCSV := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), parts[2]

	if provider == nil {
		b.WriteString("_Provider not wired (the app layer did not inject the plugin registry at startup), so no embedded filesystem can be opened. This says nothing about whether the file exists._\n")
		return b.String()
	}

	var target *EmbeddedAssetSet
	sets := provider()
	for i := range sets {
		if sets[i].Slug == slug {
			target = &sets[i]
			break
		}
	}
	if target == nil {
		slugs := make([]string, 0, len(sets))
		for _, s := range sets {
			slugs = append(slugs, "`"+s.Slug+"`")
		}
		if len(slugs) == 0 {
			fmt.Fprintf(&b, "_No plugin in this build registers embedded static assets, so there is no `%s` to read._\n", slug)
			return b.String()
		}
		fmt.Fprintf(&b, "_No registered plugin with slug `%s`. Plugins that embed static assets: %s._\n", slug, strings.Join(slugs, ", "))
		return b.String()
	}
	if target.FS == nil {
		fmt.Fprintf(&b, "_Plugin `%s` is registered with a nil filesystem — there is nothing to read._\n", slug)
		return b.String()
	}

	relPath, refusal := clampEmbeddedRel(rawPath)
	if refusal != "" {
		fmt.Fprintf(&b, "- **Refused `%s`** — %s.\n", rawPath, refusal)
		b.WriteString("- Nothing was read.\n")
		return b.String()
	}

	// Size-check before reading: an embedded file is bounded by the binary, but
	// the cap keeps this diagnostic's cost predictable and matches the on-disk
	// path's behaviour rather than quietly differing from it.
	if info, err := fs.Stat(target.FS, relPath); err == nil && info.Size() > maxFingerprintBytes {
		fmt.Fprintf(&b, "- `%s` is %d bytes, over the %d-byte read cap — not scanned.\n", relPath, info.Size(), int64(maxFingerprintBytes))
		return b.String()
	}

	data, err := fs.ReadFile(target.FS, relPath)
	if err != nil {
		fmt.Fprintf(&b, "- `%s` not found in `%s`'s embedded filesystem (%s).\n", relPath, slug, err)
		b.WriteString("- Run `host.embedded " + slug + "` for the exact paths this plugin embeds — they are relative to its `static/` dir, so there is no leading `static/` or `/static/plugins/…`.\n")
		return b.String()
	}
	sum := sha256.Sum256(data)
	fmt.Fprintf(&b, "read `%s` from `%s`'s EMBEDDED filesystem (not from disk) — %d bytes · `%s`\n",
		relPath, slug, len(data), hex.EncodeToString(sum[:])[:16])
	fmt.Fprintf(&b, "served at `%s`\n\n", strings.TrimSuffix(target.URLPrefix, "/")+"/"+relPath)
	markerReport(&b, string(data), markerCSV)
	return b.String()
}

// ── shared helpers ─────────────────────────────────────────────────────────

// versionToken extracts the bare `?v=` value AssetURL would emit for urlPath.
// Returns "" when AssetURL declined to version the URL at all (a non-/static/
// path), which is itself worth printing rather than hiding.
func versionToken(assetURL func(string) string, urlPath string) string {
	if assetURL == nil {
		return ""
	}
	full := assetURL(urlPath)
	if i := strings.Index(full, "?v="); i >= 0 {
		return full[i+len("?v="):]
	}
	return ""
}

// tokenVerdict classifies a served `?v=` token against the file's live bytes.
// The three outcomes are genuinely different findings and must not be printed
// the same way — see the per-case comments.
func tokenVerdict(token, liveSHA, buildToken string) string {
	switch {
	case token == "":
		return "⚠️ NOT VERSIONED — AssetURL returned this path unchanged"
	case token == buildToken:
		// The per-build fallback: AssetURL could not open the file through ANY
		// registered resolver. The URL still busts on deploy, so nothing looks
		// broken — but the app is not seeing this file where it is looking.
		// The CAUSE differs by storage mechanism (a wrong working directory on
		// disk, an unregistered FS when embedded), so the verdict states the
		// fact and each diagnostic's notes explain its own cause. Naming one
		// cause here would give the other caller confidently wrong advice.
		return "⚠️ BUILD-TOKEN FALLBACK — the app could not resolve this file through any registered resolver"
	case liveSHA == "" || liveSHA == "too-large":
		return "— (bytes not hashed, cannot compare)"
	case strings.HasPrefix(liveSHA, token):
		return "✓ matches bytes on disk"
	default:
		// Digests are memoised for the process lifetime (correct for caching,
		// a trap for a diagnostic): the file changed under a running binary.
		return "⚠️ STALE — the process serves an older digest; this file changed after it was first requested. Restart to re-hash."
	}
}

// markerReport renders the present/absent lines for a comma-separated marker
// list, in the same shape system.file-contains uses so the two read as one tool.
func markerReport(b *strings.Builder, content, markerCSV string) {
	for _, m := range strings.Split(markerCSV, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if strings.Contains(content, m) {
			fmt.Fprintf(b, "- ✓ `%s` — FOUND (%d occurrence(s))\n", m, strings.Count(content, m))
		} else {
			fmt.Fprintf(b, "- ✗ `%s` — not found (stale/wrong build, or marker typo)\n", m)
		}
	}
}

// trimStaticPrefix accepts the two forms an operator will naturally type — the
// URL ("/static/css/app.css") and the repo-relative path ("static/css/app.css")
// — and returns the path relative to the static root, plus what it stripped.
//
// Stripping happens BEFORE the clamp and cannot be used to bypass it:
// "/static/../../etc/passwd" still escapes and is still refused.
func trimStaticPrefix(rel string) (string, string) {
	for _, p := range []string{layouts.StaticURLPrefix, staticDirName + "/"} {
		if strings.HasPrefix(rel, p) {
			return strings.TrimPrefix(rel, p), p
		}
	}
	return rel, ""
}

// clampStaticRel validates an operator-supplied path against the static root.
// It returns the refusal REASON rather than a bare bool because "you asked for
// something outside the root" and "the file is not there" are different answers
// and printing them identically would send a reader looking for the wrong bug.
func clampStaticRel(root, rel string) (full string, refusal string) {
	switch {
	case strings.TrimSpace(rel) == "":
		return "", "the path is empty"
	case filepath.IsAbs(rel) || strings.HasPrefix(rel, "/"):
		// Refused explicitly. filepath.Join would otherwise quietly reinterpret
		// "/etc/passwd" as "<root>/etc/passwd" — an escape attempt that reads
		// as a plain miss, which is exactly the confusion worth refusing loudly.
		return "", "absolute paths are refused — give a path relative to the static root"
	}
	full, ok := clampedPath(root, rel)
	if !ok {
		return "", "the path escapes the static root (`..` traversal) and was refused"
	}
	return full, ""
}

// clampEmbeddedRel validates a path against an fs.FS's rules. fs.ValidPath is
// the stdlib's own definition of "unrooted, no `.` or `..` elements", so the
// clamp is the same one every fs.FS implementation already enforces rather than
// a hand-rolled second opinion.
func clampEmbeddedRel(rel string) (string, string) {
	clean := path.Clean(strings.TrimSpace(rel))
	switch {
	case rel == "":
		return "", "the path is empty"
	case strings.HasPrefix(rel, "/"):
		return "", "absolute paths are refused — give a path relative to the plugin's static root"
	case !fs.ValidPath(clean) || strings.HasPrefix(clean, ".."):
		return "", "the path escapes the plugin's embedded root (`..` traversal) and was refused"
	}
	return clean, ""
}

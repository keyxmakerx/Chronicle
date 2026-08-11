// Package systems — operator_diag_deploy.go is the one-shot answer to the
// question that started this whole workstream: "I deployed, and I see no
// change. Did it land?"
//
// WHY a COMPOSITE. The facts that settle that question already exist across
// four diagnostics, and on 2026-08-11 nobody assembled them — an hour went into
// shell archaeology and produced a retracted conclusion drawn from a Docker
// image label. This is the "paste this one thing after a deploy" diagnostic: it
// reads the build identity from inside the process, the fingerprints of the two
// or three assets that move on essentially every build, an optional marker
// search that spans BOTH places Chronicle keeps assets, and the existing
// installed-vs-loaded package summary.
//
// It DELEGATES rather than restates. The package summary is
// renderInstalledVsLoaded() itself, the build identity comes from
// hostIdentityLines() beside host.build's own renderer, and every token comes
// from layouts.AssetURL. A composite that reimplemented any of those would be a
// second opinion that can disagree with the diagnostic it summarises — and it
// would disagree precisely while someone was using it to decide whether a
// deploy landed.
//
// The marker search is the part that closes BOTH wrong turns from the incident
// at once: it looks in the on-disk static root AND inside the binary's embedded
// plugin filesystems, and prints a line per scope. A marker found only in the
// embedded scope is the exact result that a `grep /app/static` reported as
// "missing code".
package systems

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// bellwetherAssets are the on-disk files that move on essentially every build.
// They are named explicitly, with the reason, rather than picked heuristically:
// a heuristic ("the biggest file", "the newest CSS") would silently choose a
// different file after any refactor and quietly change what the diagnostic
// means. A named file that has gone missing is itself reported.
var bellwetherAssets = []struct {
	Rel string
	Why string
}{
	{"css/app.css", "the Tailwind build — every visual change in the app lands in this one generated file, so its hash moves on almost any front-end deploy. It is GENERATED at build time and not committed, so its absence is expected in a source checkout and alarming in a deployment"},
	{"js/boot.js", "the widget mounting core — it changes when the widget contract changes, and nothing in the UI mounts without it"},
	{"js/theme.js", "loaded on every page; a small, stable file whose hash moving means the static tree was genuinely rebuilt"},
}

// markerScanExtensions bounds the marker search to text formats. Searching a
// font or a PNG for a source marker cannot succeed, and the scan reports how
// many files it skipped for this reason so the bound is a stated fact rather
// than a silent omission.
var markerScanExtensions = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".html": true, ".htm": true,
	".json": true, ".svg": true, ".txt": true, ".md": true, ".map": true,
}

// maxMarkers caps how many markers one invocation may search for. Each marker
// is checked against every scanned file in memory, so the cap is about output
// size, not cost.
const maxMarkers = 10

// maxMarkerHits caps how many matching paths are listed per marker per scope.
// The interesting answer is "is it there and where", not a full concordance.
const maxMarkerHits = 8

// hostDeployCheckDiagnostic is the composite.
func hostDeployCheckDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "host.deploy-check",
		Title:   "Did my deploy land? — build identity, bellwether assets, marker search, package state",
		Desc:    "THE one thing to run after a deploy. Four sections: (1) which binary this process is, in three lines; (2) the fingerprint + mtime + served `?v=` of the assets that move on almost every build; (3) for each comma-separated marker you pass, whether it is present in the on-disk static root AND inside the binary's embedded plugin assets, reported separately — a marker found only in the embedded scope is exactly the result a `grep /app/static` misreports as missing code; (4) the installed-vs-loaded system package summary. Argument is optional: without it, sections 1, 2 and 4 still run.",
		ArgHint: "[<marker[,marker2]>]",
		Run:     renderHostDeployCheck,
	}
}

// renderHostDeployCheck wires the live sources into the pure renderer.
func renderHostDeployCheck(arg string) string {
	return renderHostDeployCheckFrom(deployCheckSources{
		Build:      hostinfo.ReadBuild(),
		Exe:        hostinfo.ReadExecutable(),
		Proc:       readHostProcess(),
		Root:       resolveStaticRoot(),
		Embedded:   embeddedAssetsFn,
		AssetURL:   layouts.AssetURL,
		BuildToken: layouts.BuildToken(),
		Packages:   renderInstalledVsLoaded,
		Now:        time.Now(),
	}, arg)
}

// deployCheckSources is every live input the composite reads, injected as one
// struct so a test can drive each section's degraded path. The degraded paths
// matter more than usual here: an unstamped build, a missing static root and an
// unwired plugin registry are all states a real deployment can be in, and none
// of them is reachable from a `go test` binary through the live functions.
type deployCheckSources struct {
	Build      hostinfo.Build
	Exe        hostinfo.Executable
	Proc       hostProcess
	Root       staticRoot
	Embedded   func() []EmbeddedAssetSet
	AssetURL   func(string) string
	BuildToken string
	Packages   func() string
	Now        time.Time
}

// renderHostDeployCheckFrom is the pure renderer.
func renderHostDeployCheckFrom(src deployCheckSources, arg string) string {
	var b strings.Builder
	b.WriteString("## host.deploy-check — did this deploy land?\n\n")

	b.WriteString("### 1. Which binary is running\n\n")
	for _, line := range hostIdentityLines(src.Build, src.Exe, src.Proc, src.Now) {
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("- _Image labels are NOT part of this answer. `docker inspect <tag>` describes whichever image holds that tag now, not the image this container was created from; on 2026-08-11 those labels were six months out of date about a binary built minutes earlier. Run `host.build` for the full account._\n\n")

	writeBellwetherSection(&b, src)
	writeMarkerSection(&b, src, arg)

	b.WriteString("### 4. Installed vs loaded system packages\n\n")
	if src.Packages == nil {
		b.WriteString("_Package summary unavailable._\n\n")
	} else {
		// Delegated verbatim to the existing diagnostic. Its own heading is
		// kept: the operator should be able to see which check produced this
		// block and go run it on its own.
		b.WriteString(strings.TrimSpace(src.Packages()) + "\n\n")
	}

	b.WriteString("### If this says the deploy did NOT land\n\n")
	b.WriteString("- The executable mtime in section 1 is the strongest single signal: it is when the file this process is executing was written. If it predates your deploy, the container is running an older binary no matter what any tag or label says.\n")
	b.WriteString("- `docker compose up -d` alone neither rebuilds nor re-pulls when an image with that tag already exists locally, and Chronicle's compose file declares BOTH `image:` and `build:` for the same service — so the tag has two possible producers and either can be the stale one. Force the one you meant: `docker compose build` or `docker compose pull`, then `up -d`.\n")
	b.WriteString("- If the binary is new but a feature is still absent, the front-end and the back-end can be out of step: compare section 2's mtimes against section 1's. A binary newer than the static tree means the image shipped stale assets.\n")

	return b.String()
}

// writeBellwetherSection reports the handful of assets that move on almost
// every build. Three files, each with the reason it was chosen, so the reader
// can tell what a moved (or unmoved) hash actually proves.
func writeBellwetherSection(b *strings.Builder, src deployCheckSources) {
	b.WriteString("### 2. The assets that move on almost every build\n\n")
	switch {
	case src.Root.CWDErr != "":
		fmt.Fprintf(b, "_Working directory could not be determined (%s), so the static root cannot be resolved and no asset can be checked._\n\n", src.Root.CWDErr)
		return
	case !src.Root.Exists:
		fmt.Fprintf(b, "_Static root `%s` is not usable: %s._\n\n", src.Root.Path, orUnrecorded(src.Root.StatErr))
		b.WriteString("_**This alone explains a deploy that appears not to have landed**: every on-disk asset request will 404 and every asset URL will fall back to the per-build token. Check what working directory this process was started from — the path above is the literal `static` resolved against it, and nothing configures it._\n\n")
		return
	}
	fmt.Fprintf(b, "Static root: `%s`\n\n", src.Root.Path)

	newest := ""
	var newestMod time.Time
	for _, a := range bellwetherAssets {
		full, refusal := clampStaticRel(src.Root.Path, a.Rel)
		if refusal != "" { // unreachable for the literals above; refuse loudly rather than silently skip
			fmt.Fprintf(b, "- `%s` — refused: %s\n", a.Rel, refusal)
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			fmt.Fprintf(b, "- `%s` — **not present** (%s). _%s_\n", a.Rel, err, a.Why)
			continue
		}
		sha := hashFileShort(full)
		url := layouts.StaticURLPrefix + a.Rel
		token := versionToken(src.AssetURL, url)
		fmt.Fprintf(b, "- `%s` — %s · `%s` · %s · `?v=%s` %s\n",
			a.Rel, humanBytes(info.Size()), orUnrecorded(sha),
			info.ModTime().UTC().Format(time.RFC3339), orUnrecorded(token),
			tokenVerdict(token, sha, src.BuildToken))
		fmt.Fprintf(b, "  - _%s_\n", a.Why)
		if info.ModTime().After(newestMod) {
			newest, newestMod = a.Rel, info.ModTime()
		}
	}
	if newest != "" {
		fmt.Fprintf(b, "\nNewest of these: `%s`, written %s (%s). If that predates your deploy, the static tree in this image is older than you think.\n\n",
			newest, newestMod.UTC().Format(time.RFC3339), relativeTo(newestMod, src.Now))
		return
	}
	b.WriteString("\n_None of the named assets is present. In a source checkout `css/app.css` is generated rather than committed, so this is expected in dev and alarming in a deployment._\n\n")
}

// writeMarkerSection searches both storage mechanisms for each marker. The two
// scopes are reported SEPARATELY and always both, even when one is empty: the
// entire point is that "absent from the filesystem" and "absent from the build"
// are different statements, and collapsing them into one present/absent verdict
// would rebuild the trap this diagnostic exists to disarm.
func writeMarkerSection(b *strings.Builder, src deployCheckSources, arg string) {
	b.WriteString("### 3. Marker search — on-disk AND inside the binary\n\n")
	markers, note := parseMarkers(arg)
	if note != "" {
		b.WriteString(note + "\n\n")
	}
	if len(markers) == 0 {
		b.WriteString("_No marker given, so nothing was searched._ Pass a comma-separated list of strings your new build should contain — a function name, a CSS class, a data attribute — e.g. `host.deploy-check moonPhase,--moon-tint`. Each is reported present/absent in the on-disk static root and, separately, in the plugin assets compiled into this binary.\n\n")
		return
	}

	disk, diskNote := scanDiskForMarkers(src.Root, markers)
	embed, embedNote := scanEmbeddedForMarkers(src.Embedded, markers)

	fmt.Fprintf(b, "%s\n\n%s\n\n", diskNote, embedNote)
	for _, m := range markers {
		fmt.Fprintf(b, "- **`%s`**\n", m)
		writeMarkerScope(b, "on-disk static root", disk[m], disk != nil)
		writeMarkerScope(b, "embedded in the binary", embed[m], embed != nil)
	}
	b.WriteString("\n**A marker found only in the embedded scope is not missing.** Those bytes live inside the executable and are served from memory, so `ls` and `grep` over the container filesystem cannot see them — that is the expected result, and reading it as absent code is the mistake that cost an hour on 2026-08-11.\n")
	b.WriteString("**A marker absent from BOTH scopes** means this build genuinely does not contain it: either the deploy did not land (check section 1) or the marker is misspelled.\n\n")
}

// writeMarkerScope prints one scope's result for one marker. `scanned` false
// means the scope could not be read at all, which must never render as "not
// found" — that is the same absence-of-evidence error in miniature.
func writeMarkerScope(b *strings.Builder, scope string, hits []string, scanned bool) {
	if !scanned {
		fmt.Fprintf(b, "  - %s: _not scanned_ (see the note above) — this is not \"absent\".\n", scope)
		return
	}
	if len(hits) == 0 {
		fmt.Fprintf(b, "  - ✗ %s: not found\n", scope)
		return
	}
	shown := hits
	extra := 0
	if len(shown) > maxMarkerHits {
		extra = len(shown) - maxMarkerHits
		shown = shown[:maxMarkerHits]
	}
	fmt.Fprintf(b, "  - ✓ %s: found in %d file(s) — `%s`", scope, len(hits), strings.Join(shown, "`, `"))
	if extra > 0 {
		fmt.Fprintf(b, " _(+%d more not listed)_", extra)
	}
	b.WriteString("\n")
}

// parseMarkers splits and bounds the argument.
func parseMarkers(arg string) ([]string, string) {
	var out []string
	seen := map[string]bool{}
	for _, m := range strings.Split(arg, ",") {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	if len(out) > maxMarkers {
		dropped := out[maxMarkers:]
		return out[:maxMarkers], fmt.Sprintf("_Given %d markers; searching the first %d. Not searched: `%s`._",
			len(out), maxMarkers, strings.Join(dropped, "`, `"))
	}
	return out, ""
}

// scanDiskForMarkers reads every text file under the static root once and
// records which markers each contains. Returns nil when the root could not be
// read, so the caller can print "not scanned" rather than "not found".
func scanDiskForMarkers(root staticRoot, markers []string) (map[string][]string, string) {
	if root.CWDErr != "" {
		return nil, fmt.Sprintf("_On-disk scope NOT scanned: the working directory could not be determined (%s)._", root.CWDErr)
	}
	if !root.Exists {
		return nil, fmt.Sprintf("_On-disk scope NOT scanned: static root `%s` is not usable (%s)._", root.Path, orUnrecorded(root.StatErr))
	}
	files, _ := walkStaticFiles(root.Path)
	hits := map[string][]string{}
	scanned, skippedExt, skippedSize := 0, 0, 0
	for _, f := range files {
		if !markerScanExtensions[strings.ToLower(path.Ext(f.Rel))] {
			skippedExt++
			continue
		}
		if f.Size > maxFingerprintBytes {
			skippedSize++
			continue
		}
		data, err := os.ReadFile(filepath.Join(root.Path, filepath.FromSlash(f.Rel)))
		if err != nil {
			continue
		}
		scanned++
		recordMarkerHits(hits, markers, string(data), f.Rel)
	}
	return hits, fmt.Sprintf("_On-disk scope: scanned %d text file(s) under `%s` (skipped %d non-text, %d over the size cap)._",
		scanned, root.Path, skippedExt, skippedSize)
}

// scanEmbeddedForMarkers reads every file in every plugin's embedded filesystem
// once. Returns nil when the provider is unwired — "nobody told me about the
// embedded assets" is not "the marker is not in them".
func scanEmbeddedForMarkers(provider func() []EmbeddedAssetSet, markers []string) (map[string][]string, string) {
	if provider == nil {
		return nil, "_Embedded scope NOT scanned: the plugin registry provider is not wired, so the assets compiled into this binary cannot be read. This is not \"the marker is absent from the binary\"._"
	}
	sets := provider()
	if len(sets) == 0 {
		return map[string][]string{}, "_Embedded scope: the provider is wired and no plugin in this build registers embedded assets, so there was nothing to scan. Unlike an unwired provider, this IS an answer._"
	}
	hits := map[string][]string{}
	scanned, skippedExt := 0, 0
	for _, s := range sets {
		if s.FS == nil {
			continue
		}
		// Walked directly rather than through embeddedIndex: the index hashes
		// every file, and this scan needs the bytes, not the hash. Reusing it
		// would read the whole embedded tree twice.
		_ = fs.WalkDir(s.FS, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if !markerScanExtensions[strings.ToLower(path.Ext(p))] {
				skippedExt++
				return nil
			}
			data, readErr := fs.ReadFile(s.FS, p)
			if readErr != nil {
				return nil
			}
			scanned++
			// Labelled `<slug>:<path>` because these paths do not exist on the
			// filesystem — printing a bare "js/gm_panel.js" would invite
			// someone to go looking for it with `ls`.
			recordMarkerHits(hits, markers, string(data), s.Slug+":"+p)
			return nil
		})
	}
	return hits, fmt.Sprintf("_Embedded scope: scanned %d text file(s) across %d plugin filesystem(s) INSIDE the binary (skipped %d non-text)._", scanned, len(sets), skippedExt)
}

// recordMarkerHits appends label to every marker that content contains.
func recordMarkerHits(hits map[string][]string, markers []string, content, label string) {
	for _, m := range markers {
		if strings.Contains(content, m) {
			hits[m] = append(hits[m], label)
		}
	}
}

// hashFileShort returns the first 16 hex chars of a file's sha256, or "" when
// it could not be read. "" renders as _not recorded_ rather than as a hash of
// nothing.
func hashFileShort(full string) string {
	info, err := os.Stat(full)
	if err != nil || info.Size() > maxFingerprintBytes {
		return ""
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	return shortSHA(data)
}

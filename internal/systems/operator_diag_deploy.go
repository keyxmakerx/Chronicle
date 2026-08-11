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
// at once. It reports a line per scope across all THREE places a shipped string
// can live: the on-disk static root, the binary's embedded plugin filesystems,
// and the compiled-in strings of the executable itself. A marker found only in
// the embedded scope is the exact result that a `grep /app/static` reported as
// "missing code".
//
// The third scope was added after the first two shipped, because two scopes
// were not enough to justify the conclusion the section printed. Chronicle is
// Templ-first, so markup lives in compiled Go string literals and appears in
// NEITHER of the other scopes — and the section nevertheless told the operator
// that absence from both meant the deploy had not landed. Measured against this
// repo, that fired on a real attribute (`data-cal-moon-tab`) which `grep -a`
// finds in the built binary. Re-creating the incident's own error inside the
// diagnostic written to prevent it is the worst available outcome, so the scope
// is searched rather than the conclusion merely softened.
package systems

import (
	"bytes"
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

// executableScanChunk is the streaming read size for the executable scan. The
// binary is tens of megabytes and must never be held in memory whole, so it is
// read in chunks with an overlap large enough that a marker straddling a chunk
// boundary is still found.
const executableScanChunk = 1 << 20 // 1 MiB

// maxExecutableScanBytes bounds the executable scan. Chronicle's binary is
// ~57 MB (measured), so this is far above any real case and exists only so that
// a pathological file cannot turn an admin diagnostic into an unbounded read.
// When it clamps, the scan SAYS so — a bound nobody is told about is the defect.
const maxExecutableScanBytes = 512 << 20 // 512 MiB

// hostDeployCheckDiagnostic is the composite.
func hostDeployCheckDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "host.deploy-check",
		Title:   "Did my deploy land? — build identity, bellwether assets, marker search, package state",
		Desc:    "THE one thing to run after a deploy: build identity, the fingerprint + mtime + served `?v=` of the assets that move on almost every build, the installed-vs-loaded package summary, and an optional marker search across the static root, the embedded plugin assets and the executable. **A marker hit proves the byte SHIPPED and nothing about whether it RENDERS** — for \"is it on my page?\" run `calendar.render`.",
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
	b.WriteString("- Check how the image got here. Historically `docker compose up -d` neither rebuilt nor re-pulled when an image with that tag already existed locally, and the `chronicle` service declared BOTH `image:` and `build:` — one tag with two producers, which is what made a correct deploy look like a failed one on 2026-08-11. The shipped compose file no longer does that: the service has **no `build:` section** and sets `pull_policy: always`, so `up -d` fetches the published image. **Confirm your own compose file matches** — if the `chronicle` service still has a `build:` block, you are running the old shape and a local build can still be masquerading as the published tag. Run `docker compose pull` explicitly anyway: its output is what tells you whether anything new arrived.\n")
	b.WriteString("- To run from source, use the override that keeps the two apart — `docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build` (or `make docker-all-local`), which tags the result `chronicle:local`. Plain `docker compose build chronicle` now fails by design, because the published tag has exactly one producer.\n")
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
	exec, execNote := scanExecutableForMarkers(src.Exe, markers)

	fmt.Fprintf(b, "%s\n\n%s\n\n%s\n\n", diskNote, embedNote, execNote)
	for _, m := range markers {
		fmt.Fprintf(b, "- **`%s`**\n", m)
		writeMarkerScope(b, "on-disk static root", disk[m], disk != nil)
		writeMarkerScope(b, "embedded in the binary", embed[m], embed != nil)
		writeMarkerScope(b, "compiled into the executable", exec[m], exec != nil)
	}
	b.WriteString("\n**A marker found only in the embedded scope is not missing.** Those bytes live inside the executable and are served from memory, so `ls` and `grep` over the container filesystem cannot see them — that is the expected result, and reading it as absent code is the mistake that cost an hour on 2026-08-11.\n")
	b.WriteString("**A marker found only in the executable scope is not missing either — for most UI markers that is the EXPECTED place, and the only place.** Chronicle is Templ-first: markup written in a `.templ` file is compiled into Go string literals inside this binary, so a `data-` attribute or a CSS class you copied out of page source is normally in neither the static tree nor an embedded filesystem. Absent from those two is the correct result for it, not a finding.\n")
	b.WriteString("_A hit in the executable scope proves the byte sequence is somewhere in the binary — usually the template that emits it, but any Go string literal counts, including this diagnostic's own source. Prefer a marker distinctive enough that this cannot mislead._\n")
	// The failure this file's own header records is "a label was read as
	// evidence". A marker hit read as "the feature works" is the SAME mistake
	// one layer up, and it is the most likely wrong answer an assistant would
	// give to "is RSVP on my calendar?" — so the refusal is printed here, where
	// somebody is looking at a ✓, rather than only in the Desc.
	b.WriteString("\n**A HIT PROVES THE BYTE SHIPPED. IT PROVES NOTHING ABOUT WHETHER IT RENDERS.** A marker can be present in the build and still reach no user: it can sit behind a role floor, inside a collapsed disclosure, in a layer the viewer turned off, under a CSS rule that hides it at their width, on a Block the producer did not seat, or behind a campaign addon that is switched off. Every one of those has happened here. Do NOT answer \"is this feature on my page?\" from this section — run **`calendar.render <campaignId>:<userId>`** for a calendar surface, or **`campaign.config`** for an addon or a placed block. This section answers only \"did the deploy land\".\n")
	// The all-absent conclusion is only sound when all three scopes were
	// actually READ. Asserting it over an unscanned scope is precisely the
	// absence-of-evidence error the rest of this file is built to refuse.
	if disk != nil && embed != nil && exec != nil {
		b.WriteString("**A marker absent from ALL THREE scopes** means this build genuinely does not contain it: either the deploy did not land (check section 1) or the marker is misspelled.\n\n")
		return
	}
	b.WriteString("**At least one scope could not be scanned** (see its note above), so nothing in this section is evidence that the build lacks a marker. Resolve the unscanned scope before concluding that anything is missing.\n\n")
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

// scanExecutableForMarkers searches the running binary's own bytes.
//
// WHY this scope exists — it is the one that was missing, and its absence made
// the flagship post-deploy diagnostic assert the opposite of the truth.
// Chronicle is TEMPL-FIRST: markup authored in a `.templ` file is compiled into
// Go string literals inside the executable. It is therefore in neither of the
// other two scopes by design — not on the static filesystem, not in a plugin's
// embedded FS. Most markers an operator can actually copy out of page source (a
// `data-` attribute, a CSS class emitted by a template) live only here.
//
// Measured on this repo: with only the disk and embedded scopes, searching for
// `data-cal-moon-tab` — a real attribute in internal/widgets/calendar_block/
// moonpanel.templ — reported "not found" in both and the diagnostic concluded
// the deploy had not landed, while `grep -a` on the built binary found it. That
// is the same shape as the 2026-08-11 mistake this file exists to prevent: a
// scope that was never searched being reported as evidence of absence.
//
// Returns nil when the executable could not be read, so the caller prints
// "not scanned" rather than "not found".
func scanExecutableForMarkers(exe hostinfo.Executable, markers []string) (map[string][]string, string) {
	const cannot = " This is not \"the marker is absent from this build\"._"
	if exe.Path == "" {
		return nil, "_Executable scope NOT scanned: this process could not name its own executable" +
			func() string {
				if exe.Err != "" {
					return " (" + exe.Err + ")"
				}
				return ""
			}() + ", so the strings compiled into it cannot be searched." + cannot
	}
	f, err := os.Open(exe.Path)
	if err != nil {
		return nil, fmt.Sprintf("_Executable scope NOT scanned: `%s` could not be opened (%v).%s", exe.Path, err, cannot)
	}
	defer func() { _ = f.Close() }()

	// The overlap must be one byte short of the longest marker: that is the
	// most of a marker that can sit on the far side of a chunk boundary.
	overlap := 0
	for _, m := range markers {
		if len(m) > overlap {
			overlap = len(m)
		}
	}
	if overlap > 0 {
		overlap--
	}

	buf := make([]byte, executableScanChunk+overlap)
	hits := map[string][]string{}
	found := map[string]bool{}
	var total int64
	carry := 0
	clamped := false
	for {
		n, readErr := f.Read(buf[carry:])
		if n > 0 {
			total += int64(n)
			window := buf[:carry+n]
			for _, m := range markers {
				if !found[m] && bytes.Contains(window, []byte(m)) {
					found[m] = true
					// One label, because there is only one file: the
					// executable this process is running.
					hits[m] = append(hits[m], exe.Path)
				}
			}
			if carry+n > overlap {
				copy(buf, window[carry+n-overlap:])
				carry = overlap
			} else {
				carry += n
			}
		}
		if total >= maxExecutableScanBytes {
			clamped = true
			break
		}
		if readErr != nil {
			break
		}
	}

	note := fmt.Sprintf("_Executable scope: scanned %s of `%s` — the strings compiled INTO this binary, which is where Templ markup and every Go string literal live._", humanBytes(total), exe.Path)
	if clamped {
		note = fmt.Sprintf("_Executable scope: scanned only the first %s of `%s` (the %s cap), so a marker reported absent here may sit beyond the cap._",
			humanBytes(total), exe.Path, humanBytes(maxExecutableScanBytes))
	}
	return hits, note
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

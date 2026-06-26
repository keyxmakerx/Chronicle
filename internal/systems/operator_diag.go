// Package systems — operator_diag.go is the operator-facing analogue of the
// campaign AI-Export: a catalog of named, read-only diagnostics the admin runs
// and pastes to the AI assistant. It implements the "AI's only hands" model from
// cordinator/plans/2026-06-26-debug-cockpit-and-ai-assist-capability-spec.md §B
// (named, parameterized, read-only diagnostics) + §C2 (read-only, secret-redacted).
//
// Why a CATALOG and not one big dump: a monolithic export wastes the assistant's
// context with data it didn't ask for. Instead the assistant requests ONE named
// diagnostic at a time (e.g. "run `system.files drawsteel`"); the operator runs
// just that and pastes a small, targeted result. The full dump (`system.health`)
// exists but is opt-in — requested by name only when actually needed.
//
// Safety: read-only by construction (only stats/hashes files the loader already
// serves; the probe commands are SUGGESTED, never executed by Chronicle). All
// output passes through redactSecrets as defense-in-depth, so a future diagnostic
// that accidentally surfaces a config value can't leak a credential.
package systems

import (
	"fmt"
	"regexp"
	"strings"
)

// Diagnostic is one named, read-only check in the catalog. Adding a diagnostic =
// appending one of these — the renderer, route, and redaction are unchanged
// ("modular and templated"). Run returns markdown; Arg is "" for no-argument
// diagnostics.
type Diagnostic struct {
	Name    string                  // dotted id the assistant requests, e.g. "system.files"
	Title   string                  // human title
	Desc    string                  // one-line "what you get / when to use"
	ArgHint string                  // "" when the diagnostic takes no argument
	Run     func(arg string) string // produces the (pre-redaction) markdown result
}

// diagnosticCatalog is the registry. Ordered cheapest/most-common first.
func diagnosticCatalog() []Diagnostic {
	return []Diagnostic{
		{
			Name:  "system.versions",
			Title: "Loaded system versions (compact)",
			Desc:  "One line per loaded system: id, served version, source, served dir. The first thing to check for 'is the new version live?'.",
			Run: func(string) string {
				var b strings.Builder
				b.WriteString("## system.versions\n\n")
				h := LoadedHealth()
				if len(h) == 0 {
					b.WriteString("_No systems loaded._\n")
					return b.String()
				}
				for _, s := range h {
					fmt.Fprintf(&b, "- `%s` v**%s** (%s) — `%s`\n", s.ID, s.Version, s.Source, s.Dir)
				}
				return b.String()
			},
		},
		{
			Name:    "system.files",
			Title:   "Served file fingerprints for ONE system",
			Desc:    "size + sha256[:16] + mtime of each widget/manifest file for the given system id. Use to prove which build the loader serves.",
			ArgHint: "<system-id>",
			Run: func(arg string) string {
				var b strings.Builder
				fmt.Fprintf(&b, "## system.files %s\n\n", arg)
				h := LoadedHealth()
				if arg == "" {
					b.WriteString("_Needs a system id. Loaded ids: ")
					ids := make([]string, 0, len(h))
					for _, s := range h {
						ids = append(ids, "`"+s.ID+"`")
					}
					b.WriteString(strings.Join(ids, ", ") + "._\n")
					return b.String()
				}
				for _, s := range h {
					if s.ID != arg {
						continue
					}
					fmt.Fprintf(&b, "loaded v**%s** from `%s`\n\n", s.Version, s.Dir)
					for _, f := range s.Files {
						if f.Exists {
							fmt.Fprintf(&b, "- `%s` — %d · `%s` · %s\n", f.Path, f.Size, f.SHA256, f.ModTime)
						} else {
							fmt.Fprintf(&b, "- `%s` — **MISSING**\n", f.Path)
						}
					}
					return b.String()
				}
				fmt.Fprintf(&b, "_No loaded system with id %q._\n", arg)
				return b.String()
			},
		},
		{
			Name:  "system.health",
			Title: "FULL systems health (all systems + all file hashes)",
			Desc:  "The complete served-reality dump. Larger — request only when a targeted diagnostic isn't enough.",
			Run: func(string) string {
				var b strings.Builder
				renderSystemsSection(&b, LoadedHealth())
				return b.String()
			},
		},
		{
			Name:  "probes",
			Title: "Run-and-paste-back probe library",
			Desc:  "docker / browser-console / SQL / admin-URL commands for state the server CAN'T self-report (served ?v=, on-disk folders, logs, image digest).",
			Run: func(string) string {
				var b strings.Builder
				renderProbesSection(&b, defaultProbes())
				return b.String()
			},
		},
	}
}

// renderCatalog is the default landing (no ?name): a tiny menu the assistant
// reads to decide what to request next. Deliberately small — no payload data.
func renderCatalog(cat []Diagnostic) string {
	var b strings.Builder
	b.WriteString("# Chronicle Operator Diagnostics — catalog\n\n")
	b.WriteString("Read-only, secret-redacted. The AI assistant names ONE diagnostic; you run it and paste the (small, targeted) result back. Run with `GET /admin/diagnostics?name=<name>[&arg=<arg>]`.\n\n")
	for _, d := range cat {
		arg := ""
		if d.ArgHint != "" {
			arg = " `" + d.ArgHint + "`"
		}
		fmt.Fprintf(&b, "- **`%s`**%s — %s\n", d.Name, arg, d.Desc)
	}
	b.WriteString("\nThe assistant should request the cheapest diagnostic that answers its question; `system.health` (full dump) only when a targeted one won't do.\n")
	return b.String()
}

// RunDiagnostic looks up a named diagnostic and returns its REDACTED markdown, or
// ("", false) if no such name. Pure dispatch — unit-tested directly.
func RunDiagnostic(cat []Diagnostic, name, arg string) (string, bool) {
	for _, d := range cat {
		if d.Name == name {
			return redactSecrets(d.Run(arg)), true
		}
	}
	return "", false
}

// secretLine matches "<secret-ish key> [:=] <value...>" so an accidental config
// echo can't leak a credential. The optional [\w-]* prefix catches prefixed env
// names like DB_PASSWORD / MY_API_KEY (underscore is a word char, so a plain \b
// would miss them); a required [:=] separator plus a trailing \b avoids mangling
// prose like "secretive" or a bare "sha256: …" hash line. The value is redacted
// to end-of-line so a token after "Authorization:" goes too.
var secretLine = regexp.MustCompile(`(?i)([\w-]*(?:password|passwd|secret|token|api[-_ ]?key|access[-_ ]?key|private[-_ ]?key|authorization|bearer))\b\s*[:=]\s*\S.*`)

// redactSecrets scrubs obvious credential-bearing substrings from diagnostic
// output (defense-in-depth; the systems diagnostics are secret-free by
// construction, but future ones may not be).
func redactSecrets(s string) string {
	return secretLine.ReplaceAllString(s, "$1=[REDACTED]")
}

// --- shared renderers (also used by the full bundle) -----------------------

// renderSystemsSection renders the served-reality table for each loaded system.
func renderSystemsSection(b *strings.Builder, systems []SystemHealth) {
	b.WriteString("## Loaded systems — what the server is ACTUALLY serving\n\n")
	if len(systems) == 0 {
		b.WriteString("_No systems loaded._\n\n")
		return
	}
	for _, s := range systems {
		fmt.Fprintf(b, "### `%s` — %s\n", s.ID, s.Name)
		fmt.Fprintf(b, "- loaded_version: **%s**  ·  source: %s\n", s.Version, s.Source)
		fmt.Fprintf(b, "- served dir: `%s`\n", s.Dir)
		if len(s.Files) > 0 {
			b.WriteString("- files (size · sha256[:16] · mtime):\n")
			for _, f := range s.Files {
				if f.Exists {
					fmt.Fprintf(b, "  - `%s` — %d · `%s` · %s\n", f.Path, f.Size, f.SHA256, f.ModTime)
				} else {
					fmt.Fprintf(b, "  - `%s` — **MISSING**\n", f.Path)
				}
			}
		}
		b.WriteString("\n")
	}
}

// renderProbesSection renders the run-and-paste-back probe library.
func renderProbesSection(b *strings.Builder, probes []Probe) {
	b.WriteString("## Probes — run each and paste the output back to the assistant\n\n")
	b.WriteString("Placeholders to substitute locally: `<chronicle>` / `<db>` container names, `<media>` in-container media path (see served dir above), `<campaignId>` the campaign UUID.\n\n")
	for _, p := range probes {
		fmt.Fprintf(b, "### [%s] %s\n", p.Where, p.Title)
		fmt.Fprintf(b, "_Why:_ %s\n\n", p.Why)
		b.WriteString("```\n")
		b.WriteString(p.Command)
		b.WriteString("\n```\n")
		b.WriteString("PASTE OUTPUT BELOW:\n\n\n")
	}
}

// ProbeWhere names where a probe command is run.
type ProbeWhere string

const (
	ProbeDocker  ProbeWhere = "docker"          // host shell, against the running containers
	ProbeConsole ProbeWhere = "browser-console" // DevTools console on the relevant page
	ProbeSQL     ProbeWhere = "sql"             // a DB query (run via the DB container)
	ProbeURL     ProbeWhere = "url"             // open an admin URL and copy the response
)

// Probe is one run-and-paste-back diagnostic command. Adding one is appending to
// defaultProbes (the modular property).
type Probe struct {
	ID      string
	Title   string
	Where   ProbeWhere
	Command string // may carry <placeholder> tokens the operator fills locally
	Why     string
}

// defaultProbes is the curated probe library (state the server can't self-report).
func defaultProbes() []Probe {
	return []Probe{
		{
			ID:      "served-widget-version",
			Title:   "Version of the widget the page actually loads",
			Where:   ProbeConsole,
			Command: `[...document.scripts].map(s=>s.src).filter(s=>/widgets\/.+\.js/.test(s))`,
			Why:     "The ?v= on each system widget URL = the version the loader serves. If it lags Admin▸Packages, the in-memory registry never picked up the install.",
		},
		{
			ID:      "served-widget-content",
			Title:   "Fetch a served widget and check for an expected marker",
			Where:   ProbeConsole,
			Command: `fetch(document.querySelector('script[src*="character-sheet"]').src).then(r=>r.text()).then(t=>console.log('bytes',t.length,'hasPlayEntrance',t.includes('playEntrance')))`,
			Why:     "Confirms whether the bytes the browser receives are the new build (marker present) or a stale/cached copy.",
		},
		{
			ID:      "package-version-dirs",
			Title:   "On-disk installed version folders for a system",
			Where:   ProbeDocker,
			Command: `docker exec <chronicle> ls -la <media>/packages/systems/`,
			Why:     "Shows every installed version folder. Multiple folders → a stale one may shadow the newest.",
		},
		{
			ID:      "package-file-marker",
			Title:   "Which installed copy has the new code",
			Where:   ProbeDocker,
			Command: `docker exec <chronicle> sh -lc 'grep -rl playEntrance <media>/packages/systems/ 2>/dev/null'`,
			Why:     "Pinpoints which on-disk version folder actually contains the new build, vs the served dir in system.versions.",
		},
		{
			ID:      "chronicle-logs",
			Title:   "Recent Chronicle logs (install / rescan lines)",
			Where:   ProbeDocker,
			Command: `docker logs --tail 200 <chronicle>`,
			Why:     "Shows package install, 'replacing system with preferred copy', 'ignoring duplicate system', and boot rescan lines — what the loader did with the new version.",
		},
		{
			ID:      "image-digest",
			Title:   "Which Chronicle image the container runs",
			Where:   ProbeDocker,
			Command: `docker inspect --format '{{.Image}} {{.Config.Image}}' <chronicle>`,
			Why:     "Confirms the backend is on the expected image — a stale image explains merged backend changes not being live.",
		},
	}
}

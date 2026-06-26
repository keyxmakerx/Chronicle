// Package systems — operator_diag.go assembles a read-only "operator
// diagnostics" report: a single copy-paste markdown document the admin hands to
// the AI assistant. It is the operator-facing analogue of the campaign AI-Export
// (internal/plugins/ai_workspace) — but instead of campaign content it carries
// DEPLOYMENT/SERVING state plus a modular library of probes (docker / browser
// console / SQL commands) the operator runs and pastes back, using the human as
// the assistant's hands for things the server can't self-report.
//
// Design goals: MODULAR (new probes are one struct appended to defaultProbes;
// new self-report sections are one render call) and SECRET-FREE BY CONSTRUCTION
// (the report only contains versions, on-disk paths, content hashes, and the
// probe text — never env vars, tokens, or credentials; probes that need secrets
// use <placeholder> tokens the operator fills locally and never pastes back).
package systems

import (
	"fmt"
	"strings"
)

// ProbeWhere names where a probe command is run, so the operator (and the
// assistant reading the report) know which surface produced an output.
type ProbeWhere string

const (
	ProbeDocker  ProbeWhere = "docker"          // host shell, against the running containers
	ProbeConsole ProbeWhere = "browser-console" // DevTools console on the relevant page
	ProbeSQL     ProbeWhere = "sql"             // a DB query (run via the DB container)
	ProbeURL     ProbeWhere = "url"             // open an admin URL and copy the response
)

// Probe is one diagnostic the assistant may ask the operator to run. Adding a
// new diagnostic is literally appending one of these to defaultProbes — that's
// the "modular and templated" property: the library grows without touching the
// renderer or the route.
type Probe struct {
	ID      string     // stable short id (e.g. "served-widget-version")
	Title   string     // human title
	Where   ProbeWhere // which surface to run it on
	Command string     // the exact command/snippet to run (may carry <placeholder> tokens)
	Why     string     // what the assistant learns from the output
}

// defaultProbes is the curated probe library. Ordered roughly by how often
// they're the first thing to check for a "what's actually deployed?" question.
// PLACEHOLDERS the operator substitutes locally: <chronicle> = the Chronicle
// container name; <db> = the MariaDB container name; <campaignId> = the campaign
// UUID; <media> = the in-container media path (shown by the systems table above).
func defaultProbes() []Probe {
	return []Probe{
		{
			ID:      "served-widget-version",
			Title:   "Version of the widget the page actually loads",
			Where:   ProbeConsole,
			Command: `[...document.scripts].map(s=>s.src).filter(s=>/widgets\/.+\.js/.test(s))`,
			Why:     "The ?v= on each system widget URL = the version the loader is serving. If it lags Admin▸Packages, the in-memory registry never picked up the install.",
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
			Why:     "Shows every installed version folder. Multiple folders → a stale one may be shadowing the newest.",
		},
		{
			ID:      "package-file-marker",
			Title:   "Which installed copy has the new code",
			Where:   ProbeDocker,
			Command: `docker exec <chronicle> sh -lc 'grep -rl playEntrance <media>/packages/systems/ 2>/dev/null'`,
			Why:     "Pinpoints which on-disk version folder actually contains the new build, vs which one the loader serves (the systems table above shows the served dir).",
		},
		{
			ID:      "chronicle-logs",
			Title:   "Recent Chronicle logs (look for install/rescan lines)",
			Where:   ProbeDocker,
			Command: `docker logs --tail 200 <chronicle>`,
			Why:     "Shows package install, 'replacing system with preferred copy', 'ignoring duplicate system', and boot rescan lines — i.e. what the loader did with the new version.",
		},
		{
			ID:      "extensions-health",
			Title:   "Extensions health JSON (served version + dir + file hashes)",
			Where:   ProbeURL,
			Command: `/admin/extensions/health`,
			Why:     "The authoritative server-side view of what each loader serves. (Also embedded as the systems table above when generated server-side.)",
		},
		{
			ID:      "image-digest",
			Title:   "Which Chronicle image the container is running",
			Where:   ProbeDocker,
			Command: `docker inspect --format '{{.Image}} {{.Config.Image}}' <chronicle>`,
			Why:     "Confirms the backend container is on the expected image — a stale image explains why merged backend changes aren't live.",
		},
	}
}

// renderSystemsSection renders the served-reality table for each loaded system.
func renderSystemsSection(b *strings.Builder, systems []SystemHealth) {
	b.WriteString("## Loaded systems — what the server is ACTUALLY serving\n\n")
	if len(systems) == 0 {
		b.WriteString("_No systems loaded (registry empty, or this report was built without server state)._\n\n")
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

// BuildOperatorReport renders the full markdown report. Pure — takes the
// server-collected systems health + the probe library and produces the
// copy-paste document. Unit-tested directly.
func BuildOperatorReport(systems []SystemHealth, probes []Probe) string {
	var b strings.Builder
	b.WriteString("# Chronicle Operator Diagnostics\n\n")
	b.WriteString("Read-only snapshot — no secrets included. Paste this WHOLE document to the AI assistant, then run the probes below and paste each output where indicated.\n\n")
	renderSystemsSection(&b, systems)
	renderProbesSection(&b, probes)
	return b.String()
}

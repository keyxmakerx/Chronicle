// Package systems — operator_batch.go implements the BATCH half of the operator
// AI workspace: the request/approve/execute protocol the user described, modeled
// on the campaign ai_workspace plugin's export→prompt→parse→commit loop.
//
// Flow (all read-only):
//  1. The operator downloads the FUNCTIONS SPEC (FunctionsSpecJSON) — a compact,
//     machine-readable catalog the external AI consumes.
//  2. The AI composes ONE batch-request object naming the diagnostics it wants.
//  3. The operator pastes it into Admin ▸ Diagnostics ▸ AI Workspace.
//  4. ParseBatch validates it against the catalog and produces a PLAN the operator
//     reviews (prompt-injection containment: the human sees exactly what will run
//     before approving; unknown names and full-dump requests are flagged).
//  5. On approval, RunBatch executes only the read-only diagnostics, redacts the
//     output, and returns ONE compact document the operator pastes back to the AI.
//
// "Quantized / less context heavy": the batch runs ONLY the named diagnostics
// (each already targeted), prefixed by a one-line manifest and a byte-count
// footer so the AI sees the payload size. The heavy full dump (system.health) is
// gated behind an explicit `full_dump: true` in the request — the security gate
// the user asked for, so a stray full dump can't leak a wall of context.
package systems

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Catalog returns the named read-only diagnostic catalog (exported so the admin
// AI-workspace handler can render the functions spec and execute batches without
// reaching into unexported state).
func Catalog() []Diagnostic { return diagnosticCatalog() }

// batchSpecVersion is the schema version the AI targets. Bumped only on a
// breaking change to the request shape; ParseBatch accepts a missing/zero v as 1.
const batchSpecVersion = 1

const (
	// maxBatchCalls caps how many calls one request may name (bounded toolset).
	maxBatchCalls = 50
	// maxBatchBytes caps the pasted request — a batch object is tiny, so a
	// multi-MB paste is hostile; reject before decoding (cheap DoS guard,
	// mirrors ai_workspace's import body cap).
	maxBatchBytes = 64 * 1024
	// maxBatchOutputBytes caps the assembled result document so "compact by
	// default" can't silently degrade into a multi-MB dump (e.g. an authorized
	// full dump on a large install). Past the cap, output is truncated with a
	// notice rather than streamed unbounded.
	maxBatchOutputBytes = 256 * 1024
)

// FunctionSpec is one entry in the machine-readable functions list the AI reads.
// JSON tags are terse on purpose — this is the "less context heavy" payload.
type FunctionSpec struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Arg      string `json:"arg,omitempty"`       // arg hint, omitted when the function takes none
	FullDump bool   `json:"full_dump,omitempty"` // true = requires full_dump:true in the request to run
}

// FunctionsSpec is the top-level functions list: a self-describing contract the
// external AI consumes, including the exact request shape and an example.
type FunctionsSpec struct {
	V         int            `json:"v"`
	Purpose   string         `json:"purpose"`
	HowToUse  string         `json:"how_to_use"`
	Request   BatchRequest   `json:"request_format"` // canonical example request
	Functions []FunctionSpec `json:"functions"`
}

// BatchCall is one diagnostic invocation in a batch request.
type BatchCall struct {
	Name string `json:"name"`
	Arg  string `json:"arg,omitempty"`
}

// BatchRequest is the single pastable object the AI composes. `FullDump` is the
// explicit authorization gate for heavy diagnostics; `Note` is free text the AI
// uses to record what it is investigating (echoed into the result for audit).
type BatchRequest struct {
	V        int         `json:"v"`
	Note     string      `json:"note,omitempty"`
	FullDump bool        `json:"full_dump,omitempty"`
	Calls    []BatchCall `json:"calls"`
}

// PlanStatus classifies one planned call after validation against the catalog.
type PlanStatus string

const (
	PlanOK          PlanStatus = "ok"               // known, runnable as requested
	PlanUnknown     PlanStatus = "unknown"          // no such diagnostic name
	PlanNeedsFull   PlanStatus = "blocked-fulldump" // full-dump diagnostic but full_dump:false
	PlanMissingName PlanStatus = "missing-name"     // empty name in the request
	PlanDuplicate   PlanStatus = "duplicate"        // identical (name,arg) already runs earlier
)

// callKey identifies a call for deduplication: two calls with the same name and
// arg do identical work, so the batch runs them once.
func callKey(name, arg string) string { return name + "\x00" + arg }

// sanitizeInline collapses a value to a single safe line for the result
// markdown's manifest: newlines and backticks (which would split the bullet or
// close the inline-code span the AI reads) become spaces, and it is
// length-capped. Defends the integrity of the document the operator pastes back
// (LOW-severity finding) — purely cosmetic, but keeps a crafted note/name from
// corrupting the manifest.
func sanitizeInline(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '`' {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// PlannedCall is one row the operator reviews before approving. It carries the
// resolved title/desc so the review screen is self-explanatory.
type PlannedCall struct {
	Name     string
	Arg      string
	Title    string
	Desc     string
	FullDump bool
	Status   PlanStatus
	Note     string // human reason for a non-OK status
}

// Runnable reports whether this planned call will actually execute on approval.
func (p PlannedCall) Runnable() bool { return p.Status == PlanOK }

// BatchPlan is the validated, reviewable result of parsing a pasted request.
type BatchPlan struct {
	Request   BatchRequest
	Calls     []PlannedCall
	RunnableN int // count of Status==PlanOK (how many will actually run)
}

// buildFunctionsSpec assembles the functions list from the live catalog.
func buildFunctionsSpec() FunctionsSpec {
	cat := diagnosticCatalog()
	fns := make([]FunctionSpec, 0, len(cat))
	example := make([]BatchCall, 0, 2)
	for _, d := range cat {
		fns = append(fns, FunctionSpec{
			Name:     d.Name,
			Title:    d.Title,
			Desc:     d.Desc,
			Arg:      d.ArgHint,
			FullDump: d.FullDump,
		})
	}
	// A small, concrete example so the AI emits a valid object first try.
	example = append(example, BatchCall{Name: "system.versions"})
	if hasDiagnostic(cat, "packages.installed-vs-loaded") {
		example = append(example, BatchCall{Name: "packages.installed-vs-loaded"})
	}
	return FunctionsSpec{
		V:        batchSpecVersion,
		Purpose:  "Read-only Chronicle operator diagnostics. Each function fingerprints what the server is ACTUALLY serving (versions, file hashes, install-vs-loaded state) — for tracking down deploy/serve mismatches and sync issues.",
		HowToUse: "Compose ONE request_format object naming the functions you want, then tell the operator to paste it into Admin ▸ Diagnostics ▸ AI Workspace. It is reviewed and approved by a human, executed read-only and secret-redacted, and the compact result pasted back to you. Heavy/full-dump functions require \"full_dump\": true — request that only when a targeted function won't do.",
		Request: BatchRequest{
			V:        batchSpecVersion,
			Note:     "what you're investigating (optional)",
			FullDump: false,
			Calls:    example,
		},
		Functions: fns,
	}
}

// FunctionsSpecJSON returns the indented functions list the operator copies and
// feeds to the AI. Stable, pretty-printed (it's read by a human and a model).
func FunctionsSpecJSON() string {
	b, err := json.MarshalIndent(buildFunctionsSpec(), "", "  ")
	if err != nil {
		// MarshalIndent of a static struct can't realistically fail; degrade safely.
		return "{\"error\":\"failed to render functions spec\"}"
	}
	return string(b)
}

// stripCodeFence removes a leading ```json / ``` fence and trailing ``` that
// chat models routinely wrap JSON in, plus surrounding whitespace, so a pasted
// fenced block parses without the operator hand-editing it.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line (``` or ```json).
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	} else {
		return "" // a lone "```" with no body
	}
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ParseBatch parses a pasted batch-request string and validates every call
// against the catalog, returning a reviewable plan. It returns an error only for
// a structurally invalid request (bad JSON, no calls, too many calls) — unknown
// or gated call NAMES are not errors; they surface as non-OK plan rows so the
// operator sees the whole picture before approving.
func ParseBatch(raw string) (*BatchPlan, error) {
	if len(raw) > maxBatchBytes {
		return nil, fmt.Errorf("request too large (%d bytes) — cap is %d; a batch object should be small", len(raw), maxBatchBytes)
	}
	body := stripCodeFence(raw)
	if body == "" {
		return nil, fmt.Errorf("empty request — paste the AI's batch object")
	}
	var req BatchRequest
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields() // reject typo'd top-level keys so silent omissions don't hide
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("not a valid batch object: %w", err)
	}
	if req.V != 0 && req.V != batchSpecVersion {
		return nil, fmt.Errorf("unsupported request version %d (this server speaks v%d)", req.V, batchSpecVersion)
	}
	if len(req.Calls) == 0 {
		return nil, fmt.Errorf("request has no calls")
	}
	if len(req.Calls) > maxBatchCalls {
		return nil, fmt.Errorf("too many calls (%d) — cap is %d", len(req.Calls), maxBatchCalls)
	}
	req.Note = sanitizeInline(req.Note) // clean once so review + result agree

	cat := diagnosticCatalog()
	plan := &BatchPlan{Request: req}
	seen := make(map[string]bool, len(req.Calls)) // dedup identical (name,arg) work
	for _, c := range req.Calls {
		pc := PlannedCall{Name: strings.TrimSpace(c.Name), Arg: strings.TrimSpace(c.Arg)}
		switch {
		case pc.Name == "":
			pc.Status = PlanMissingName
			pc.Note = "every call needs a \"name\""
		default:
			if d := findDiagnostic(cat, pc.Name); d != nil {
				pc.Title, pc.Desc, pc.FullDump = d.Title, d.Desc, d.FullDump
				switch {
				case d.FullDump && !req.FullDump:
					pc.Status = PlanNeedsFull
					pc.Note = "heavy full-dump — add \"full_dump\": true to authorize"
				case seen[callKey(pc.Name, pc.Arg)]:
					pc.Status = PlanDuplicate
					pc.Note = "identical to an earlier call — runs once"
				default:
					seen[callKey(pc.Name, pc.Arg)] = true
					pc.Status = PlanOK
					plan.RunnableN++
				}
			} else {
				pc.Status = PlanUnknown
				pc.Note = "no such function — check the functions list"
			}
		}
		plan.Calls = append(plan.Calls, pc)
	}
	return plan, nil
}

// RunBatch executes the runnable calls in a (re-validated) plan and returns ONE
// compact, secret-redacted document: a manifest line, each result, a footer with
// the runnable count and byte size. Non-runnable rows are listed in the manifest
// (so the AI learns what was skipped and why) but contribute no payload.
//
// It re-derives runnability from the live catalog rather than trusting the plan's
// flags — defense against a stale/forged plan slipping a gated call through.
func RunBatch(plan *BatchPlan) string {
	if plan == nil {
		return "_no plan_"
	}
	cat := diagnosticCatalog()

	// One authoritative classification pass, independent of the plan's stored
	// flags: re-derive runnability from the live catalog AND dedup identical
	// (name,arg) work, so the manifest and the payload can never disagree and a
	// duplicate call can't re-run an expensive sweep (the MED finding).
	type item struct {
		name, arg string
		run       bool
		reason    string
	}
	seen := make(map[string]bool, len(plan.Calls))
	items := make([]item, 0, len(plan.Calls))
	for _, c := range plan.Calls {
		it := item{name: c.Name, arg: c.Arg}
		d := findDiagnostic(cat, c.Name)
		switch {
		case c.Name == "":
			it.reason = "missing name"
		case d == nil:
			it.reason = "unknown function"
		case d.FullDump && !plan.Request.FullDump:
			it.reason = "full dump not authorized"
		case seen[callKey(c.Name, c.Arg)]:
			it.reason = "duplicate"
		default:
			seen[callKey(c.Name, c.Arg)] = true
			it.run = true
		}
		items = append(items, it)
	}

	var b strings.Builder
	b.WriteString("# Chronicle diagnostics — batch result\n\n")
	if note := plan.Request.Note; note != "" {
		fmt.Fprintf(&b, "_Investigating:_ %s\n\n", note) // already sanitized in ParseBatch
	}

	// Manifest: one line per requested call so skips are visible. name/arg are
	// sanitized so a crafted value can't corrupt the manifest the AI reads.
	b.WriteString("**Manifest:**\n")
	var skipped []string
	for _, it := range items {
		label := sanitizeInline(it.name)
		if label == "" {
			label = "(unnamed)"
		}
		if it.arg != "" {
			label += " " + sanitizeInline(it.arg)
		}
		if it.run {
			fmt.Fprintf(&b, "- ✓ `%s`\n", label)
		} else {
			fmt.Fprintf(&b, "- ✗ `%s` — %s\n", label, it.reason)
			skipped = append(skipped, label)
		}
	}
	b.WriteString("\n---\n\n")

	// Payload: only the runnable calls, each already redacted by RunDiagnostic.
	// Capped so an authorized full dump on a large install can't run away.
	ran, truncated := 0, false
	for _, it := range items {
		if !it.run {
			continue
		}
		if b.Len() >= maxBatchOutputBytes {
			truncated = true
			break
		}
		out, ok := RunDiagnostic(cat, it.name, it.arg)
		if !ok {
			continue
		}
		b.WriteString(strings.TrimRight(out, "\n"))
		b.WriteString("\n\n---\n\n")
		ran++
	}
	if truncated {
		fmt.Fprintf(&b, "_…output truncated at ~%d KB — narrow the batch and re-run for the rest._\n\n---\n\n", maxBatchOutputBytes/1024)
	}

	// Footer: size + counts so the AI can reason about context budget.
	doc := redactSecrets(b.String()) // belt-and-suspenders: RunDiagnostic already redacts each part
	var f strings.Builder
	fmt.Fprintf(&f, "_ran %d function(s)", ran)
	if len(skipped) > 0 {
		sort.Strings(skipped)
		fmt.Fprintf(&f, ", skipped %d (%s)", len(skipped), strings.Join(skipped, ", "))
	}
	fmt.Fprintf(&f, " · %d bytes · ~%d tokens_\n", len(doc), len(doc)/4)
	return doc + f.String()
}

// findDiagnostic returns the catalog entry with the given name, or nil.
func findDiagnostic(cat []Diagnostic, name string) *Diagnostic {
	for i := range cat {
		if cat[i].Name == name {
			return &cat[i]
		}
	}
	return nil
}

// hasDiagnostic reports whether a named diagnostic exists (used to keep the
// example request valid even if the catalog is trimmed).
func hasDiagnostic(cat []Diagnostic, name string) bool { return findDiagnostic(cat, name) != nil }

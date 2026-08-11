package systems

import (
	"strings"
	"testing"
)

// The host.* family was built in five separate stages, each adding its own
// diagnostics and its own tests. The per-stage tests are thorough about their
// own renderers, but nothing checked the family AS A CATALOG — and the catalog
// is the artefact the operator and the AI assistant actually meet first. These
// tests cover the properties that only exist across stage boundaries, so a
// sixth stage adding a diagnostic inherits them for free.

// TestEveryHostDiagnosticRunsWithEmptyArg executes every registered host.*
// diagnostic through the real dispatch path with NO argument.
//
// Why the empty argument specifically: the route passes `?arg=` through
// verbatim, so a bare click on the admin page's Run button is exactly this
// call. Several of these diagnostics take an argument, and the no-argument
// branch is the one an operator hits by accident — it must print usage or a
// default listing, never panic and never come back blank. A blank result is
// the specific failure this whole workstream exists to prevent: it reads as
// "nothing to report" when it actually means "this never ran".
//
// The per-stage suites each cover their own diagnostics; the value here is that
// the loop is over the CATALOG, so a diagnostic added later is covered whether
// or not its author writes this test again.
func TestEveryHostDiagnosticRunsWithEmptyArg(t *testing.T) {
	cat := diagnosticCatalog()

	var hostCount int
	for _, d := range cat {
		if !strings.HasPrefix(d.Name, "host.") {
			continue
		}
		hostCount++

		t.Run(d.Name, func(t *testing.T) {
			// A panic in a diagnostic would otherwise take down the admin
			// request; recovering here names the culprit instead of failing
			// the whole package with a stack trace.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked with empty arg: %v", r)
				}
			}()

			got, ok := RunDiagnostic(cat, d.Name, "")
			if !ok {
				t.Fatalf("RunDiagnostic reported no such diagnostic")
			}
			if strings.TrimSpace(got) == "" {
				t.Fatal("returned empty output — an empty result reads as 'nothing to report' when it means 'this produced nothing'")
			}
			// Every diagnostic opens with its own H2 so a pasted result is
			// self-identifying: the assistant receives these out of order and
			// out of context.
			if want := "## " + d.Name; !strings.HasPrefix(got, want) {
				t.Errorf("output must start with %q so a pasted result names itself; got:\n%s", want, first(got, 160))
			}
			// A Printf verb that did not get its argument renders as `%!s(MISSING)`
			// and looks like data. Cheap to catch, impossible to spot in review.
			if strings.Contains(got, "%!") {
				t.Errorf("output contains a formatting error (%%!): \n%s", first(got, 400))
			}
		})
	}

	// Guard the loop itself: if a refactor renamed the family, the range above
	// would silently test nothing and still pass.
	if hostCount == 0 {
		t.Fatal("no host.* diagnostics found in the catalog — this test would pass vacuously")
	}
}

// TestCatalogDescriptionsStayScannable caps Desc length.
//
// Desc is documented on the struct as a "one-line 'what you get / when to use'"
// and it is rendered IN FULL for every entry by renderCatalog (the menu) and by
// FunctionsSpecJSON (what the assistant reads to choose). It is a menu, not the
// documentation: whatever a reader needs once they have chosen belongs in the
// diagnostic's own output, where the person acting on it is actually looking.
//
// The host.* entries arrived carrying their incident narrative in the Desc and
// ran to 588/539/492/470/463 bytes against a pre-existing catalog mean of 177,
// which pushed the menu to 8.9 KB and the functions spec to 13 KB. The cap is
// set above today's longest with real headroom, so ordinary editing is
// unaffected — it exists to stop a future stage reintroducing a paragraph.
func TestCatalogDescriptionsStayScannable(t *testing.T) {
	// Chosen to sit clearly above the longest current Desc and clearly below
	// the essay-length ones that were removed. Raising this is a decision about
	// the menu, not a formality — move the prose into the Run output instead.
	const maxDescBytes = 450

	for _, d := range diagnosticCatalog() {
		if n := len(d.Desc); n > maxDescBytes {
			t.Errorf("%s: Desc is %d bytes (max %d). The catalog is a menu — move the explanation into the diagnostic's own output.\n%s",
				d.Name, n, maxDescBytes, d.Desc)
		}
		// A Desc that is only a title is no use for choosing between 28
		// entries; the pre-existing catalog's shortest is ~98 bytes.
		if n := len(d.Desc); n < 40 {
			t.Errorf("%s: Desc is only %d bytes — too short to choose on", d.Name, n)
		}
	}
}

// TestCatalogNamesAreUniqueAndWellFormed re-checks uniqueness across the whole
// catalog. TestDiagnosticCatalog_WellFormed already asserts this; the reason to
// state it again here is the ArgHint half, which nothing else covers: a
// diagnostic whose Run parses an argument but whose ArgHint is empty is
// invisible on the admin page, because RenderDiagnosticsHTML only draws an
// input box when ArgHint is set. That combination is silently unusable rather
// than broken, which is the hardest kind of defect to notice.
func TestCatalogNamesAreUniqueAndWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range diagnosticCatalog() {
		if seen[d.Name] {
			t.Errorf("duplicate diagnostic name %q", d.Name)
		}
		seen[d.Name] = true

		if strings.TrimSpace(d.Name) != d.Name {
			t.Errorf("%q has surrounding whitespace", d.Name)
		}
		// ArgHint is free text, but every current one names its shape with
		// either <> (required) or [] (optional). An ArgHint that does neither
		// is probably prose that will not fit the input box's placeholder.
		if d.ArgHint != "" && !strings.ContainsAny(d.ArgHint, "<[") {
			t.Errorf("%s: ArgHint %q should show the argument shape, e.g. <system-id> or [<count>]", d.Name, d.ArgHint)
		}
	}
}

// TestHostFamilyPrecedesTheRest pins the ordering claim the catalog comment
// makes: every host.* diagnostic sorts ahead of every system.*/packages.*/
// entity.* one.
//
// This is not cosmetic. Each stage asserted its own diagnostics sat near the
// top, but "near the top" is not composable — five stages each inserting at
// their own preferred position is how a group becomes interleaved. The reason
// the group leads is that every other diagnostic describes what is being
// served, and all of them are worthless if the binary is not the one the
// operator thinks it is.
func TestHostFamilyPrecedesTheRest(t *testing.T) {
	cat := diagnosticCatalog()

	lastHost, firstOther := -1, -1
	for i, d := range cat {
		switch {
		case strings.HasPrefix(d.Name, "host."):
			lastHost = i
		// probes is the run-and-paste-back library and deliberately sits last,
		// so it is not part of the "everything else" that host.* precedes.
		case d.Name == "probes":
		default:
			if firstOther == -1 {
				firstOther = i
			}
		}
	}

	if lastHost == -1 || firstOther == -1 {
		t.Fatal("expected both host.* and non-host diagnostics in the catalog")
	}
	if lastHost > firstOther {
		t.Errorf("host.* diagnostics must form one uninterrupted block ahead of the rest: %q (index %d) sits after %q (index %d)",
			cat[lastHost].Name, lastHost, cat[firstOther].Name, firstOther)
	}
}

// TestOnlyGenuinelyHeavyDiagnosticsRequireFullDump pins the FullDump flag.
//
// FullDump means a batch must pass full_dump:true to run the diagnostic, so it
// is friction deliberately applied to expensive or verbose checks. Applied to a
// cheap one it just makes the tool annoying; omitted from an expensive one it
// removes the guard. Both directions are asserted by name, because the flag's
// correctness is a judgement about each diagnostic that should be restated
// explicitly when a new one is added rather than inferred.
func TestOnlyGenuinelyHeavyDiagnosticsRequireFullDump(t *testing.T) {
	// The heavy ones: system.health dumps served reality for every loaded
	// system, and host.assets hashes every file under the static root by
	// reading it whole.
	wantFullDump := map[string]bool{
		"system.health": true,
		"host.assets":   true,
	}

	for _, d := range diagnosticCatalog() {
		switch {
		case wantFullDump[d.Name] && !d.FullDump:
			t.Errorf("%s walks/reads enough to need FullDump, but the flag is unset", d.Name)
		case !wantFullDump[d.Name] && d.FullDump:
			t.Errorf("%s requires full_dump:true but is not in the known-heavy set. If it really is heavy, add it to wantFullDump with the reason; if not, clear the flag.", d.Name)
		}
	}
}

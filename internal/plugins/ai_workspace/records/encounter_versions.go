package records

import (
	"context"
	"fmt"
	"strings"
)

// maxEncounterVersions matches the encounters plugin's own cap, so a block
// over it is refused at review rather than at Apply.
const maxEncounterVersions = 6

// TestsAPI is the Tests block as the AI needs it, adapted in app wiring:
// every test on a page the operator may see, DM only.
type TestsAPI interface {
	Tests(ctx context.Context, campaignID string, a Actor) ([]TestRef, error)
}

// TestRef is one test: a montage or other long test on a page.
type TestRef struct {
	ID, Name, PageName string
	Results            []TestResult
	// Recorded is the label of the result the table settled on; empty
	// while the test is still open.
	Recorded string
}

// TestResult is one way a test can end.
type TestResult struct{ ID, Label string }

// EncounterVersion is a version as read back: the creatures it plays in
// full, and the test result that picks it, by name.
type EncounterVersion struct {
	Name         string
	Test, Result string
	Creatures    []EncounterCreature
}

// EncounterVersionInput is a version to save: the whole creature list it
// plays, and the test and result that pick it (both empty when only the DM
// picks). The adapter turns the list into changes from the base rows.
type EncounterVersionInput struct {
	Name             string
	TestID, ResultID string
	Creatures        []EncounterCreature
}

// parsedVersion is one `versions` item before its test is found.
type parsedVersion struct {
	name, test, result string
	creatures          []EncounterCreature
}

// parseVersions reads the `versions` list. Each item is a map with `name`,
// optional `test` and `result`, and `creatures` written like the base list.
func parseVersions(items []any) ([]parsedVersion, error) {
	if len(items) > maxEncounterVersions {
		return nil, badRequestf("an encounter has at most %d versions", maxEncounterVersions)
	}
	out := make([]parsedVersion, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, badRequestf("each version is a set of keys: `name`, `creatures`, and `test` with `result` when a test picks it")
		}
		str := func(k string) string {
			if v, ok := m[k]; ok && v != nil {
				return strings.TrimSpace(fmt.Sprint(v))
			}
			return ""
		}
		v := parsedVersion{name: str("name"), test: str("test"), result: str("result")}
		if v.name == "" {
			return nil, badRequestf("every version needs a `name`")
		}
		if seen[strings.ToLower(v.name)] {
			return nil, badRequestf("two versions are called %s", quote(v.name))
		}
		seen[strings.ToLower(v.name)] = true
		if (v.test == "") != (v.result == "") {
			return nil, badRequestf("%s: give both `test` and `result`, or neither", v.name)
		}
		var list []any
		switch c := m["creatures"].(type) {
		case []any:
			list = c
		case nil:
		default:
			list = []any{c}
		}
		if len(list) == 0 {
			return nil, badRequestf("%s: a version lists every creature it plays under `creatures`", v.name)
		}
		for _, line := range list {
			c, err := parseCreature(line)
			if err != nil {
				return nil, badRequestf("%s: %s", v.name, planError(err))
			}
			v.creatures = append(v.creatures, c)
		}
		out = append(out, v)
	}
	return out, nil
}

// resolveVersions finds each version's test by name, preferring a test on
// the encounter's own page when two share a name, and its result by label.
// Two versions picked by the same result are refused, as the plugin does.
func resolveVersions(ctx context.Context, svc TestsAPI, campaignID, pageName string, a Actor, in []parsedVersion) ([]EncounterVersionInput, error) {
	var tests []TestRef
	out := make([]EncounterVersionInput, 0, len(in))
	picks := map[string]bool{}
	for _, v := range in {
		ev := EncounterVersionInput{Name: v.name, Creatures: v.creatures}
		if v.test != "" {
			if tests == nil {
				if svc == nil {
					return nil, badRequestf("tests can't be read on this server, so a version can't name one; leave out `test` and `result`")
				}
				var err error
				if tests, err = svc.Tests(ctx, campaignID, a); err != nil {
					return nil, err
				}
				if tests == nil {
					tests = []TestRef{}
				}
			}
			t, err := findTest(tests, v.test, pageName)
			if err != nil {
				return nil, badRequestf("%s: %s", v.name, planError(err))
			}
			for _, r := range t.Results {
				if sameName(r.Label, v.result) {
					ev.TestID, ev.ResultID = t.ID, r.ID
					break
				}
			}
			if ev.ResultID == "" {
				labels := make([]string, len(t.Results))
				for i, r := range t.Results {
					labels[i] = quote(r.Label)
				}
				return nil, badRequestf("%s: the test %s has no result %s; its results are %s", v.name, quote(t.Name), quote(v.result), strings.Join(labels, ", "))
			}
			key := ev.TestID + "/" + ev.ResultID
			if picks[key] {
				return nil, badRequestf("two versions are picked by %s: %s", t.Name, v.result)
			}
			picks[key] = true
		}
		out = append(out, ev)
	}
	return out, nil
}

func findTest(tests []TestRef, name, pageName string) (TestRef, error) {
	var hits []TestRef
	for _, t := range tests {
		if sameName(t.Name, name) {
			hits = append(hits, t)
		}
	}
	switch {
	case len(hits) == 1:
		return hits[0], nil
	case len(hits) > 1:
		for _, t := range hits {
			if sameName(t.PageName, pageName) {
				return t, nil
			}
		}
		return TestRef{}, badRequestf("more than one test is called %s and none is on this encounter's page; rename one", quote(name))
	}
	if len(tests) == 0 {
		return TestRef{}, badRequestf("no test called %s, and the campaign has no tests yet; add one with the Tests block on a page", quote(name))
	}
	return TestRef{}, badRequestf("no test called %s; ask `what: tests` for the campaign's tests", quote(name))
}

// creatureLine writes one creature the way the block reads it.
func creatureLine(c EncounterCreature) string {
	start := map[string]string{"seen": "starts seen", "hidden": "starts hidden", "later": "arrives later"}[c.Start]
	if start == "" {
		start = "starts seen"
	}
	return fmt.Sprintf("%d %s, group %s, %s", c.Count, c.Name, c.Group, start)
}

// writeEncounterBlock writes one encounter in this kind's own format, so an
// AI can read it and send it back changed.
func writeEncounterBlock(b *strings.Builder, e EncounterRef) {
	b.WriteString("---\nkind: encounter\npage: " + quote(e.PageName) + "\nname: " + quote(e.Name) + "\n")
	if e.Ready {
		b.WriteString("ready: true\n")
	}
	if len(e.Heroes) > 0 {
		hs := make([]string, len(e.Heroes))
		for i, h := range e.Heroes {
			hs[i] = quote(h)
		}
		b.WriteString("heroes: [" + strings.Join(hs, ", ") + "]\n")
	}
	if len(e.Creatures) > 0 {
		b.WriteString("creatures:\n")
		for _, c := range e.Creatures {
			b.WriteString("  - " + quote(creatureLine(c)) + "\n")
		}
	}
	if len(e.Versions) > 0 {
		b.WriteString("versions:\n")
		for _, v := range e.Versions {
			b.WriteString("  - name: " + quote(v.Name) + "\n")
			if v.Test != "" {
				b.WriteString("    test: " + quote(v.Test) + "\n    result: " + quote(v.Result) + "\n")
			}
			b.WriteString("    creatures:\n")
			for _, c := range v.Creatures {
				b.WriteString("      - " + quote(creatureLine(c)) + "\n")
			}
		}
	}
	b.WriteString("---\n")
}

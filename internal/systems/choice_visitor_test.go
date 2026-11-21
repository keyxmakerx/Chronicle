package systems

import (
	"context"
	"strings"
	"testing"
)

// vcamp is the campaign id the stand-in characters belong to.
const vcamp = "c1"

// visitorEnv is the real pick-list stack for one campaign: the package's file,
// the campaign's own entries through the real overlay, and the real rules, so
// the visitor's plan is judged on the code that serves it.
type visitorEnv struct {
	choices *characterChoiceService
	rules   ChoiceRuleService
	entries SystemEntryService
	erepo   *fakeEntryRepo
	rrepo   *fakeRuleRepo
}

func newVisitorEnv(t *testing.T, manifest func(*SystemManifest)) *visitorEnv {
	t.Helper()
	choiceSourcesMu.Lock()
	savedOverlay, savedSources := choiceOverlay, choiceSources
	choiceSources = nil
	choiceSourcesMu.Unlock()
	t.Cleanup(func() {
		choiceSourcesMu.Lock()
		choiceOverlay, choiceSources = savedOverlay, savedSources
		choiceSourcesMu.Unlock()
	})

	m := entryTestManifest()
	m.Name = "Test System"
	if manifest != nil {
		manifest(m)
	}
	files := map[string]string{"sys/data/ancestries.json": `[` +
		`{"slug":"fury","name":"Fury"},{"slug":"devil","name":"Devil"},{"slug":"human","name":"Human"}]`}
	choices := newTestChoiceService(t, files, []*SystemManifest{m}, fakeAddons{"sys": true})
	erepo, rrepo := &fakeEntryRepo{}, &fakeRuleRepo{}
	resolver := EntryManifestResolver(func(context.Context, string) *SystemManifest { return m })
	rules := NewChoiceRuleService(rrepo, erepo, choices, resolver)
	entries := NewSystemEntryService(erepo, resolver, WithEntryCharacters(testCharacters), WithEntryRules(rules.(EntryRuleForgetter)))
	RegisterSystemEntryChoices(entries)
	choices.SetChoiceLimiter(rules)
	return &visitorEnv{choices: choices, rules: rules, entries: entries, erepo: erepo, rrepo: rrepo}
}

func (e *visitorEnv) homebrew(t *testing.T, in CreateSystemEntryInput) *SystemEntry {
	t.Helper()
	in.FieldKey = "ancestry"
	en, err := e.entries.Create(context.Background(), vcamp, dirActor, in)
	if err != nil {
		t.Fatal(err)
	}
	return en
}

func (e *visitorEnv) mark(t *testing.T, source, key, target, state string) {
	t.Helper()
	if _, err := e.rules.SetMarks(context.Background(), vcamp, dirActor, ChoiceMarksInput{FieldKey: "ancestry",
		Marks: []ChoiceMark{{Source: source, Key: key, Target: target, State: state}}}); err != nil {
		t.Fatalf("mark %s/%s for %q = %s: %v", source, key, target, state, err)
	}
}

func planNames(p *CreatorPlan) string {
	var out []string
	for _, st := range p.Steps {
		for _, c := range st.Choices {
			out = append(out, st.FieldKey+":"+c.Name)
		}
	}
	return strings.Join(out, ",")
}

// A visitor's plan is the list a brand-new player is offered, built from what
// the public reads: nothing kept to a player or to the Directors, no draft,
// and no rule that names a player, whatever the Directors have set.
func TestChoiceVisitor_PlanHoldsOnlyWhatThePublicReads(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *visitorEnv)
		want  string
	}{
		{"package entries only", func(*testing.T, *visitorEnv) {}, "ancestry:Devil,ancestry:Fury,ancestry:Human"},
		{"homebrew for everyone is offered",
			func(t *testing.T, e *visitorEnv) { e.homebrew(t, CreateSystemEntryInput{Name: "Mothfolk"}) },
			"ancestry:Devil,ancestry:Fury,ancestry:Human,ancestry:Mothfolk"},
		{"a Directors-only entry never appears",
			func(t *testing.T, e *visitorEnv) {
				e.homebrew(t, CreateSystemEntryInput{Name: "Secret", Visibility: EntryVisibilityDirectors})
			},
			"ancestry:Devil,ancestry:Fury,ancestry:Human"},
		{"a one-player grant never appears, though that player is named by a rule",
			func(t *testing.T, e *visitorEnv) {
				fairy := e.homebrew(t, CreateSystemEntryInput{Name: "Fairy", Visibility: EntryVisibilityCharacter, ForEntityID: "wren"})
				e.mark(t, "campaign", itoa64(fairy.ID), "tyne", RuleOpen)
			},
			"ancestry:Devil,ancestry:Fury,ancestry:Human"},
		{"a player's draft never appears",
			func(t *testing.T, e *visitorEnv) {
				e.erepo.rows = append(e.erepo.rows, SystemEntry{ID: 900, CampaignID: vcamp, SystemID: "sys", FieldKey: "ancestry",
					Slug: "pats-draft", Name: "Pat's Draft", Visibility: EntryVisibilityEveryone, Status: EntryStatusDraft, CreatedBy: "pat"})
			},
			"ancestry:Devil,ancestry:Fury,ancestry:Human"},
		{"a rule for everyone narrows the list",
			func(t *testing.T, e *visitorEnv) { e.mark(t, "package", "fury", "", RuleNotOffered) },
			"ancestry:Devil,ancestry:Human"},
		{"a rule about one player does not reach it, and a player's exception does not widen it",
			func(t *testing.T, e *visitorEnv) {
				e.mark(t, "package", "fury", "", RuleNotOffered)
				e.mark(t, "package", "fury", "pat", RuleOpen)
				e.mark(t, "package", "devil", "pat", RuleNotOffered)
			},
			"ancestry:Devil,ancestry:Human"},
		{"an ask-first entry is still listed (the page sends it as a plain choice)",
			func(t *testing.T, e *visitorEnv) {
				m := e.homebrew(t, CreateSystemEntryInput{Name: "Mothfolk"})
				e.mark(t, "campaign", itoa64(m.ID), "", RuleAskFirst)
			},
			"ancestry:Devil,ancestry:Fury,ancestry:Human,ancestry:Mothfolk"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newVisitorEnv(t, nil)
			tc.setup(t, env)
			got, err := env.choices.CreatorPlan(WithChoiceVisitor(context.Background()), vcamp)
			if err != nil {
				t.Fatal(err)
			}
			if names := planNames(got); names != tc.want {
				t.Errorf("visitor plan = %s\nwant          %s", names, tc.want)
			}
			// The same list a player with no rules of their own is offered.
			fresh, err := env.choices.CreatorPlan(WithChoiceViewerUser(WithChoiceViewer(context.Background(), false), "newcomer"), vcamp)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name != "a player's draft never appears" && planNames(fresh) != planNames(got) {
				t.Errorf("a fresh player is offered %s, the visitor %s", planNames(fresh), planNames(got))
			}
		})
	}
}

// Nothing the caller puts on the context can widen what a visitor reads: not a
// Director, not an account with rules or grants, not "unlimited", and not the
// registered extra sources.
func TestChoiceVisitor_ContextCannotWidenIt(t *testing.T) {
	env := newVisitorEnv(t, nil)
	env.homebrew(t, CreateSystemEntryInput{Name: "Secret", Visibility: EntryVisibilityDirectors})
	fairy := env.homebrew(t, CreateSystemEntryInput{Name: "Fairy", Visibility: EntryVisibilityCharacter, ForEntityID: "wren"})
	env.mark(t, "package", "fury", "", RuleNotOffered)
	env.mark(t, "campaign", itoa64(fairy.ID), "tyne", RuleOpen)
	RegisterChoiceSource(func(context.Context, string, string) ([]Choice, error) {
		return []Choice{{Name: "Source Leak", Source: "campaign"}}, nil
	})

	const want = "ancestry:Devil,ancestry:Human"
	hostile := WithChoiceUnlimited(WithChoiceViewerUser(WithChoiceFor(WithChoiceViewer(context.Background(), true), "tyne"), "tyne"))
	wrappers := map[string]func(context.Context) context.Context{
		"as given":                    func(c context.Context) context.Context { return WithChoiceVisitor(c) },
		"after a Director's context":  func(c context.Context) context.Context { return WithChoiceVisitor(hostile) },
		"with unlimited added after":  func(c context.Context) context.Context { return WithChoiceUnlimited(WithChoiceVisitor(c)) },
		"with a Director added after": func(c context.Context) context.Context { return WithChoiceViewer(WithChoiceVisitor(c), true) },
	}
	for name, wrap := range wrappers {
		t.Run(name, func(t *testing.T) {
			got, err := env.choices.CreatorPlan(wrap(context.Background()), vcamp)
			if err != nil {
				t.Fatal(err)
			}
			if names := planNames(got); names != want {
				t.Errorf("plan = %s, want %s", names, want)
			}
			for _, st := range got.Steps {
				if st.CanMake {
					t.Errorf("step %s offers to make your own entry to a visitor", st.FieldKey)
				}
			}
		})
	}
}

// A category the manifest keeps to Directors is not the public's to read, so a
// visitor's list never draws on its file.
func TestChoiceVisitor_DirectorOnlyFileIsSkipped(t *testing.T) {
	env := newVisitorEnv(t, func(m *SystemManifest) { m.DirectorData = []string{"ancestries.json"} })
	player, err := env.choices.CreatorPlan(WithChoiceViewerUser(WithChoiceViewer(context.Background(), false), "newcomer"), vcamp)
	if err != nil {
		t.Fatal(err)
	}
	if planNames(player) == "" {
		t.Fatal("setup: a player should read the file")
	}
	got, err := env.choices.CreatorPlan(WithChoiceVisitor(context.Background()), vcamp)
	if err != nil {
		t.Fatal(err)
	}
	if names := planNames(got); names != "" {
		t.Errorf("a visitor was offered a Director-only file: %s", names)
	}
}

package systems

import (
	"strings"
	"testing"
)

// Every catalog check must be pickable by hand, once, with a plain title, or
// the page silently hides a check the functions list still offers.
func TestDiagnosticGroupsCoverCatalogOnce(t *testing.T) {
	seen := map[string]string{}
	for _, g := range DiagnosticGroups() {
		if len(g.Checks) == 0 {
			t.Errorf("group %q is empty", g.ID)
		}
		for _, c := range g.Checks {
			if prev, ok := seen[c.Name]; ok {
				t.Errorf("%s is in both %q and %q", c.Name, prev, g.ID)
			}
			seen[c.Name] = g.ID
			if pickerTitles[c.Name] == "" {
				t.Errorf("%s has no plain title", c.Name)
			}
		}
	}
	for _, d := range diagnosticCatalog() {
		if _, ok := seen[d.Name]; !ok {
			t.Errorf("catalog check %s is in no group", d.Name)
		}
	}
	for _, g := range pickerGroupOrder {
		for _, n := range g.names {
			if !hasDiagnostic(diagnosticCatalog(), n) {
				t.Errorf("group %q names %s, which is not in the catalog", g.id, n)
			}
		}
	}
	for n := range pickerKinds {
		if !hasDiagnostic(diagnosticCatalog(), n) {
			t.Errorf("pickerKinds names %s, which is not in the catalog", n)
		}
	}
}

// A check whose catalog hint takes an argument needs a blank kind, or the page
// would send it with no argument at all.
func TestPickerKindsMatchArgHints(t *testing.T) {
	for _, d := range diagnosticCatalog() {
		k := pickerKinds[d.Name]
		switch {
		case d.ArgHint == "" && k != ArgNone:
			t.Errorf("%s takes no argument but has blank kind %q", d.Name, k)
		case d.ArgHint != "" && k == ArgNone:
			t.Errorf("%s takes %q but has no blank", d.Name, d.ArgHint)
		case strings.HasPrefix(d.ArgHint, "<campaignId>") && k == ArgText:
			t.Errorf("%s is campaign-scoped but is a free-text blank", d.Name)
		}
	}
}

func TestDiagnosticQuestions(t *testing.T) {
	qs := DiagnosticQuestions()
	if len(qs) != 6 {
		t.Fatalf("want the six signed questions, got %d", len(qs))
	}
	for _, q := range qs {
		if len(q.Calls) == 0 {
			t.Errorf("question %q runs nothing", q.ID)
		}
		needsCamp, needsPage := false, false
		for _, c := range q.Calls {
			if !hasDiagnostic(diagnosticCatalog(), c) {
				t.Errorf("question %q names %s, which is not in the catalog", q.ID, c)
			}
			switch pickerKinds[c] {
			case ArgCampaign:
				needsCamp = true
			case ArgCampaignPage:
				needsCamp, needsPage = true, true
			case ArgCampaignText, ArgText:
				if !strings.HasPrefix(diagnosticHint(c), "[") {
					t.Errorf("question %q uses %s, whose text blank the sentence can't fill", q.ID, c)
				}
			}
		}
		if needsCamp != q.NeedsCampaign() || needsPage != q.NeedsPage() {
			t.Errorf("question %q: sentence blanks (campaign %v, page %v) don't match its checks (campaign %v, page %v)",
				q.ID, q.NeedsCampaign(), q.NeedsPage(), needsCamp, needsPage)
		}
	}
}

func diagnosticHint(name string) string {
	for _, d := range diagnosticCatalog() {
		if d.Name == name {
			return d.ArgHint
		}
	}
	return ""
}

func TestComposeBatch(t *testing.T) {
	tests := []struct {
		name     string
		in       ComposeInput
		wantArgs map[string]string
		wantFull bool
		needCamp bool
		needPage bool
	}{
		{
			name:     "blank page question, both blanks filled",
			in:       ComposeInput{Note: "A sheet or page shows blank", Calls: []string{"entity.fields", "entity.sync-mappings", "sync.inbound"}, Campaign: "c1", Page: "Tyne"},
			wantArgs: map[string]string{"entity.fields": "c1:Tyne", "entity.sync-mappings": "c1:Tyne", "sync.inbound": "c1:Tyne"},
		},
		{
			name:     "blanks left empty become review-step placeholders",
			in:       ComposeInput{Calls: []string{"campaign.config", "entity.fields"}},
			wantArgs: map[string]string{"campaign.config": "<campaignId>", "entity.fields": "<campaignId>:<entity>"},
			needCamp: true, needPage: true,
		},
		{
			name:     "free text and campaign text",
			in:       ComposeInput{Calls: []string{"host.errors", "entity.find", "system.versions"}, Campaign: "c1", Args: map[string]string{"host.errors": " 5 ", "entity.find": "orrin"}},
			wantArgs: map[string]string{"host.errors": "5", "entity.find": "c1:orrin", "system.versions": ""},
		},
		{
			name:     "a large check is the full-dump opt-in",
			in:       ComposeInput{Calls: []string{"system.health"}},
			wantArgs: map[string]string{"system.health": ""},
			wantFull: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ParseBatch(ComposeBatch(tt.in))
			if err != nil {
				t.Fatalf("composed request does not parse: %v", err)
			}
			if plan.Request.FullDump != tt.wantFull {
				t.Errorf("full_dump = %v, want %v", plan.Request.FullDump, tt.wantFull)
			}
			if plan.Request.Note != tt.in.Note {
				t.Errorf("note = %q, want %q", plan.Request.Note, tt.in.Note)
			}
			if len(plan.Calls) != len(tt.wantArgs) {
				t.Fatalf("got %d calls, want %d", len(plan.Calls), len(tt.wantArgs))
			}
			for _, c := range plan.Calls {
				if want, ok := tt.wantArgs[c.Name]; !ok || c.Arg != want {
					t.Errorf("%s arg = %q, want %q", c.Name, c.Arg, want)
				}
				if !c.Runnable() {
					t.Errorf("%s is not runnable: %s", c.Name, c.Note)
				}
			}
			if PlanNeedsCampaign(plan) != tt.needCamp || PlanNeedsEntity(plan) != tt.needPage {
				t.Errorf("needs campaign %v page %v, want %v %v", PlanNeedsCampaign(plan), PlanNeedsEntity(plan), tt.needCamp, tt.needPage)
			}
		})
	}
}

// An unknown name from a tampered form is flagged by ParseBatch, never run.
func TestComposeBatchUnknownNameIsFlagged(t *testing.T) {
	plan, err := ParseBatch(ComposeBatch(ComposeInput{Calls: []string{"host.build", "db.drop"}}))
	if err != nil {
		t.Fatal(err)
	}
	if plan.RunnableN != 1 {
		t.Errorf("runnable = %d, want 1", plan.RunnableN)
	}
}

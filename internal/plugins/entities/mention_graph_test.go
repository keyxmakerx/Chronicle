package entities

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The ids are shaped like the uuids a mention carries.
const (
	idWatch  = "aaaaaaaa-0000-0000-0000-000000000001"
	idLedger = "aaaaaaaa-0000-0000-0000-000000000002"
	idKeep   = "aaaaaaaa-0000-0000-0000-000000000003"
	idGone   = "aaaaaaaa-0000-0000-0000-000000000004"
)

func mentionOf(id, name string) string {
	return `<a data-mention-id="` + id + `" href="/e/` + id + `">@` + name + `</a>`
}

func dmOnlyText(inner string) string {
	return `<span data-secret="true">` + inner + `</span>`
}

func TestMentionTargets(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		seesSecrets bool
		want        []string
	}{
		{"a name in plain text", `<p>See ` + mentionOf(idLedger, "Ledger") + `.</p>`, false, []string{idLedger}},
		{"each page once, in the order written", `<p>` + mentionOf(idKeep, "Keep") + mentionOf(idLedger, "Ledger") + mentionOf(idKeep, "Keep") + `</p>`, false, []string{idKeep, idLedger}},
		{"a page naming itself", `<p>` + mentionOf(idWatch, "Watch") + `</p>`, false, nil},
		{"no names", `<p>Nothing here.</p>`, false, nil},
		{"a name only in DM-only text, for a player", `<p>Rumour: ` + dmOnlyText(`it is in the `+mentionOf(idLedger, "Ledger")) + `</p>`, false, nil},
		{"a name only in DM-only text, for the DM", `<p>Rumour: ` + dmOnlyText(`it is in the `+mentionOf(idLedger, "Ledger")) + `</p>`, true, []string{idLedger}},
		{"a name both in DM-only text and outside it", `<p>` + dmOnlyText(mentionOf(idLedger, "Ledger")) + ` and ` + mentionOf(idLedger, "Ledger") + `</p>`, false, []string{idLedger}},
		{"DM-only text beside a plain name", `<p>` + dmOnlyText(mentionOf(idLedger, "Ledger")) + mentionOf(idKeep, "Keep") + `</p>`, false, []string{idKeep}},
		{"a name inside a span inside DM-only text", `<p>` + dmOnlyText(`<span class="x">`+mentionOf(idLedger, "Ledger")+`</span> more`) + `</p>`, false, nil},
		{"DM-only text that was never closed", `<p><span data-secret="true">` + mentionOf(idLedger, "Ledger") + `</p>`, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mentionTargets(idWatch, tt.html, tt.seesSecrets)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mentionTargets = %v, want %v", got, tt.want)
			}
		})
	}
}

// mentionGraphRepo answers the two lookups GetMentionGraph makes and records
// which pages it was asked to name.
func mentionGraphRepo(sources []MentionSource, visible map[string]MentionPage, asked *[]string) *mockEntityRepo {
	return &mockEntityRepo{
		findMentionSourcesFn: func(context.Context, string, int, string) ([]MentionSource, error) {
			return sources, nil
		},
		findMentionPagesFn: func(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]MentionPage, error) {
			*asked = append(*asked, ids...)
			out := map[string]MentionPage{}
			for _, id := range ids {
				if p, ok := visible[id]; ok {
					out[id] = p
				}
			}
			return out, nil
		},
	}
}

func linkPairs(g *MentionGraph) []string {
	var out []string
	for _, l := range g.Links {
		out = append(out, l.SourceEntityID+">"+l.TargetEntityID)
	}
	sort.Strings(out)
	return out
}

func pageIDs(g *MentionGraph) []string {
	var out []string
	for id := range g.Pages {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func TestGetMentionGraph(t *testing.T) {
	visible := map[string]MentionPage{
		idWatch:  {ID: idWatch, Name: "Harbour Watch", Slug: "harbour-watch", TypeName: "Organization", TypeIcon: "fa-flag", TypeColor: "#112233"},
		idLedger: {ID: idLedger, Name: "Smuggler's Ledger", Slug: "ledger", TypeName: "Item", TypeIcon: "fa-box", TypeColor: "#445566"},
		idKeep:   {ID: idKeep, Name: "Tidewatch Keep", Slug: "keep", TypeName: "Location", TypeIcon: "fa-map", TypeColor: "#778899"},
	}
	// The Watch names the Keep in plain text, the Ledger only in DM-only
	// text, and a page that is gone (in the trash, hidden or another
	// campaign's: the lookup does not return it).
	sources := []MentionSource{{ID: idWatch, EntryHTML: `<p>` + mentionOf(idKeep, "Keep") + ` ` +
		dmOnlyText(mentionOf(idLedger, "Ledger")) + ` ` + mentionOf(idGone, "Gone") + `</p>`}}

	t.Run("a player gets the plain link, named at both ends", func(t *testing.T) {
		var asked []string
		svc := NewEntityService(mentionGraphRepo(sources, visible, &asked), nil, nil)
		g, err := svc.GetMentionGraph(context.Background(), "camp", permissions.RolePlayer, "u1", false)
		if err != nil {
			t.Fatalf("GetMentionGraph: %v", err)
		}
		if got, want := linkPairs(g), []string{idWatch + ">" + idKeep}; !reflect.DeepEqual(got, want) {
			t.Errorf("links = %v, want %v", got, want)
		}
		if got, want := pageIDs(g), []string{idWatch, idKeep}; !reflect.DeepEqual(got, want) {
			t.Errorf("pages = %v, want %v", got, want)
		}
		if g.Pages[idKeep] != visible[idKeep] {
			t.Errorf("the Keep is described as %+v", g.Pages[idKeep])
		}
		for _, id := range asked {
			if id == idLedger {
				t.Error("the page named only in DM-only text was looked up for a player")
			}
		}
	})

	t.Run("the DM also gets the link that sits in DM-only text", func(t *testing.T) {
		var asked []string
		svc := NewEntityService(mentionGraphRepo(sources, visible, &asked), nil, nil)
		g, err := svc.GetMentionGraph(context.Background(), "camp", permissions.RoleOwner, "dm", true)
		if err != nil {
			t.Fatalf("GetMentionGraph: %v", err)
		}
		if got, want := linkPairs(g), []string{idWatch + ">" + idLedger, idWatch + ">" + idKeep}; !reflect.DeepEqual(got, want) {
			t.Errorf("links = %v, want %v", got, want)
		}
		if _, ok := g.Pages[idGone]; ok {
			t.Error("a page the lookup did not return is in the graph")
		}
	})

	t.Run("a link from a page that cannot be named is left out", func(t *testing.T) {
		var asked []string
		onlyKeep := map[string]MentionPage{idKeep: visible[idKeep]}
		svc := NewEntityService(mentionGraphRepo(sources, onlyKeep, &asked), nil, nil)
		g, err := svc.GetMentionGraph(context.Background(), "camp", permissions.RolePlayer, "u1", false)
		if err != nil {
			t.Fatalf("GetMentionGraph: %v", err)
		}
		if len(g.Links) != 0 || len(g.Pages) != 0 {
			t.Errorf("got links %v and pages %v", linkPairs(g), pageIDs(g))
		}
	})

	t.Run("a campaign with no names in its text asks for no pages", func(t *testing.T) {
		var asked []string
		svc := NewEntityService(mentionGraphRepo(nil, visible, &asked), nil, nil)
		g, err := svc.GetMentionGraph(context.Background(), "camp", permissions.RolePlayer, "u1", false)
		if err != nil {
			t.Fatalf("GetMentionGraph: %v", err)
		}
		if g == nil || g.Pages == nil || len(g.Links) != 0 || len(asked) != 0 {
			t.Errorf("got %+v after asking for %v", g, asked)
		}
	})
}

package campaigns

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func tabBody(text string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, text)
		return err
	})
}

// A plugin's Customize tab is merged in SortOrder (then registration order),
// and a factory that returns no ID or no body is skipped rather than rendered
// as an unreachable tab that would collide with the page's own keys.
func TestCustomizeTabs_RegisterSortsAndSkipsInvalid(t *testing.T) {
	h := &Handler{}
	cc := ctxWithRole(RoleOwner)
	h.RegisterCustomizeTab(nil) // tolerated
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "late", Label: "Late", SortOrder: 50, Content: tabBody("late")}
	})
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "maps", Label: "Maps", SortOrder: 10, Content: tabBody("maps")}
	})
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "", Label: "Nameless", Content: tabBody("x")}
	})
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "empty", Label: "Empty", SortOrder: 1}
	})
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "tie", Label: "Tie", SortOrder: 10, Content: tabBody("tie")}
	})

	got := h.customizeTabs(cc)
	var ids []string
	for _, tab := range got {
		ids = append(ids, tab.ID)
	}
	if want := "maps,tie,late"; strings.Join(ids, ",") != want {
		t.Errorf("tab order = %v, want %s", ids, want)
	}
}

// With nothing registered the Customize page renders exactly as before.
func TestCustomizeTabs_NoneRegistered(t *testing.T) {
	if got := (&Handler{}).customizeTabs(ctxWithRole(RoleOwner)); len(got) != 0 {
		t.Errorf("want no tabs, got %d", len(got))
	}
}

// The page shows a registered tab's button and body, wired to the Alpine tab
// key, after the three built-in tabs.
func TestCustomizePage_RendersContributedTab(t *testing.T) {
	cc := ctxWithRole(RoleOwner)
	extra := []CustomizeTab{{ID: "maps", Label: "Maps", Icon: "fa-solid fa-map", Content: tabBody("MAPS-BODY")}}
	var sb strings.Builder
	if err := CustomizePage(cc, nil, "tok", extra).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"MAPS-BODY", `tab === &#39;maps&#39;`, "Appearance"} {
		if !strings.Contains(out, want) {
			t.Errorf("customize page missing %q", want)
		}
	}
	if strings.Index(out, "Appearance") > strings.Index(out, "MAPS-BODY") {
		t.Error("contributed tabs come after the built-in ones")
	}
}

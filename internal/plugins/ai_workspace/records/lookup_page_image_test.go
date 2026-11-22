package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeAliases map[string][]string

func (f fakeAliases) GetAliases(_ context.Context, id string) ([]entities.EntityAlias, error) {
	var out []entities.EntityAlias
	for _, a := range f[id] {
		out = append(out, entities.EntityAlias{EntityID: id, Alias: a})
	}
	return out, nil
}

// The page answer says whether a picture and a cover exist and lists the
// other names, but never a file path, and a hidden page gives neither.
func TestLookups_PagePictureAndAliases(t *testing.T) {
	pic, cover := "/media/secret/aldric.png", " "
	aldric := page("aldric", "Ser Aldric", "Character", false)
	aldric.ImagePath, aldric.CoverImagePath = &pic, &cover
	ghost := page("ghost", "The Ghost", "Character", true)
	ghost.ImagePath = &pic
	rope := page("rope", "Rope", "Item", false)

	l := world()
	l.Pages = &fakePages{all: []entities.Entity{aldric, ghost, rope}}
	l.Aliases = fakeAliases{"aldric": {"The Old Knight", "Al"}, "ghost": {"Hidden Alias"}}

	tests := []struct {
		name, page string
		a          Actor
		want, not  []string
		wantErr    bool
	}{
		{"picture without a cover, with aliases", "Ser Aldric", player,
			[]string{"Picture: yes", "Cover: no", "Other names: The Old Knight, Al"}, []string{"/media", "aldric.png"}, false},
		{"no picture and no aliases", "Rope", player,
			[]string{"Picture: no", "Cover: no"}, []string{"Other names"}, false},
		{"a Director sees the private page's", "The Ghost", owner,
			[]string{"Picture: yes", "Other names: Hidden Alias"}, []string{"/media"}, false},
		{"a player never reaches a private page's", "The Ghost", player, nil, []string{"Hidden Alias", "Picture"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans := l.Answer(context.Background(), camp, tt.a, []Record{lookup("page", map[string]any{"name": tt.page})})
			if (ans[0].Error != "") != tt.wantErr {
				t.Fatalf("error = %q", ans[0].Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(ans[0].Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans[0].Text)
				}
			}
			for _, w := range tt.not {
				if strings.Contains(ans[0].Text, w) {
					t.Errorf("answer leaks %q:\n%s", w, ans[0].Text)
				}
			}
		})
	}
}

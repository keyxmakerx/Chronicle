package decks

import "testing"

// Shown must equal the number of cards the strip draws, or the page tab's
// count and its offer to open disagree with what the viewer sees.
func TestHeroHandViewShown(t *testing.T) {
	card := HandCard{DeckName: "Omens"}
	tests := []struct {
		name string
		view *HeroHandView
		want int
	}{
		{"nil view", nil, 0},
		{"nothing", &HeroHandView{}, 0},
		{"own hand only", &HeroHandView{Own: &HandSpot{Hand: []HandCard{card, card}}}, 2},
		{"own play only", &HeroHandView{Own: &HandSpot{Play: []HandCard{card}}}, 1},
		{"own hand and play", &HeroHandView{Own: &HandSpot{Hand: []HandCard{card, card}, Play: []HandCard{card}}}, 3},
		{"own but empty", &HeroHandView{Own: &HandSpot{}}, 0},
		{"others: faces and backs", &HeroHandView{Others: []HandOther{{Count: 3, Faces: []Face{{}}}}}, 3},
		{"others: backs only", &HeroHandView{Others: []HandOther{{Count: 2}}}, 2},
		{"others: two decks", &HeroHandView{Others: []HandOther{{Count: 2, Faces: []Face{{}, {}}}, {Count: 1}}}, 3},
		{"others: count below faces still draws the faces", &HeroHandView{Others: []HandOther{{Count: 0, Faces: []Face{{}}}}}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.view.Shown(); got != tc.want {
				t.Errorf("Shown = %d, want %d", got, tc.want)
			}
			if tc.view != nil && (tc.view.Shown() == 0) != tc.view.Empty() {
				t.Errorf("Shown and Empty disagree for %s", tc.name)
			}
		})
	}
}

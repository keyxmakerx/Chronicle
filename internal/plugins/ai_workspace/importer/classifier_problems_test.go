package importer

import "testing"

func TestPageProblem_Did(t *testing.T) {
	tests := []struct {
		p    PageProblem
		want string
	}{
		{PageProblem{Page: "Nobody Here", Action: ActionUpdate, Reason: "Cannot update: no existing entity named \"Nobody Here\" in this campaign."}, "Asked to change Nobody Here, refused: no such page"},
		{PageProblem{Page: "Nobody Here", Action: ActionDelete, Reason: "Cannot delete: no existing entity named \"Nobody Here\" in this campaign."}, "Asked to remove Nobody Here, refused: no such page"},
		{PageProblem{Page: "Khepri", Action: ActionCreate, Reason: "These sheet fields don't fit the Race category: x"}, "Asked to add Khepri, refused: its sheet fields don't fit the category"},
	}
	for _, tt := range tests {
		if got := tt.p.Did(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

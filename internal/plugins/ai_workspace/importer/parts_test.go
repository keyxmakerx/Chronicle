package importer

import (
	"reflect"
	"strings"
	"testing"
)

func TestTakeParts_ReadsLabels(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		want    Part
		wantErr string
	}{
		{"numbers", "name: Lastwell fixes\npart: 3\nof: 3\nneeds: [2, 1, 2]",
			Part{Name: "Lastwell fixes", Key: "lastwell-fixes", Number: 3, Of: 3, Needs: []int{1, 2}, Block: 1}, ""},
		{"written loosely", "name: Lastwell fixes\npart: 2 of 4\nneeds: part 1",
			Part{Name: "Lastwell fixes", Key: "lastwell-fixes", Number: 2, Of: 4, Needs: []int{1}, Block: 1}, ""},
		{"no needs", "name: Lastwell fixes\npart: 1",
			Part{Name: "Lastwell fixes", Key: "lastwell-fixes", Number: 1, Block: 1}, ""},
		{"no name", "part: 2", Part{}, "needs `name:`"},
		{"no number", "name: Lastwell fixes", Part{}, "needs `part:`"},
		{"needs a later part", "name: Lastwell fixes\npart: 2\nneeds: [2]", Part{}, "can only need earlier parts"},
		{"count below number", "name: Lastwell fixes\npart: 3\nof: 2", Part{}, "part 3 of 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lab := TakeParts(Parse("---\nkind: part\n" + tt.label + "\n---\n---\nname: Moon\n---\nBody."))
			if tt.wantErr != "" {
				if len(lab.Parts) != 0 || lab.Blocks[0].Status != StatusParseError || !strings.Contains(lab.Blocks[0].ParseError, tt.wantErr) {
					t.Fatalf("want a parse error with %q, got parts %+v, first block %+v", tt.wantErr, lab.Parts, lab.Blocks[0])
				}
				if _, ok := lab.PartOf(1); ok {
					t.Errorf("a block under a bad label belongs to no part")
				}
				return
			}
			if len(lab.Parts) != 1 || !reflect.DeepEqual(lab.Parts[0], tt.want) {
				t.Fatalf("part = %+v, want %+v", lab.Parts, tt.want)
			}
			if len(lab.Blocks) != 1 || lab.Blocks[0].Name != "Moon" {
				t.Fatalf("blocks = %+v", lab.Blocks)
			}
			if pi, ok := lab.PartOf(lab.Blocks[0].SourceIndex); !ok || pi != 0 {
				t.Errorf("the page should belong to the label above it")
			}
		})
	}
}

func TestOrderParts(t *testing.T) {
	label := func(name string, n int) string {
		return "---\nkind: part\nname: " + name + "\npart: " + string(rune('0'+n)) + "\n---"
	}
	page := func(name string) string { return "---\nname: " + name + "\n---\nBody." }
	tests := []struct {
		name  string
		paste []string
		want  []string // block names in the order wanted, "" when unchanged
	}{
		{"in order", []string{label("A", 1), page("One"), label("A", 2), page("Two")}, nil},
		{"one label", []string{page("Loose"), label("A", 2), page("Two")}, nil},
		{"reversed", []string{page("Loose"), label("A", 2), page("Two"), label("A", 1), page("One")},
			[]string{"Loose", "A", "One", "A", "Two"}},
		{"two answers keep their own order", []string{label("B", 2), page("B2"), label("A", 1), page("A1"), label("B", 1), page("B1")},
			[]string{"B", "B1", "B", "B2", "A", "A1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, moved := OrderParts(Parse(strings.Join(tt.paste, "\n")))
			if (tt.want != nil) != moved {
				t.Fatalf("moved = %v", moved)
			}
			if !moved {
				return
			}
			var names []string
			for _, p := range Parse(got) {
				names = append(names, p.Name)
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("order = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestMissingNeedsAndPartList(t *testing.T) {
	p := Part{Number: 4, Needs: []int{1, 2, 3}}
	got := MissingNeeds(p, map[int]bool{1: true}, map[int]bool{3: true})
	if !reflect.DeepEqual(got, []int{2}) {
		t.Errorf("missing = %v, want [2]", got)
	}
	for nums, want := range map[string]string{"2": "part 2", "1,2": "parts 1 and 2", "1,2,3": "parts 1, 2 and 3"} {
		var in []int
		for _, f := range strings.Split(nums, ",") {
			in = append(in, int(f[0]-'0'))
		}
		if got := PartList(in); got != want {
			t.Errorf("PartList(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestAnswerMissing(t *testing.T) {
	tests := []struct {
		name    string
		present []Part
		applied map[int]bool
		want    []int
	}{
		{"all in", []Part{{Number: 1, Of: 2}, {Number: 2, Of: 2}}, nil, nil},
		{"earlier parts missing", []Part{{Number: 3, Of: 3, Needs: []int{2}}}, nil, []int{1, 2}},
		{"applied counts", []Part{{Number: 3, Of: 3}}, map[int]bool{1: true, 2: true}, nil},
		{"no count, only needs", []Part{{Number: 3, Needs: []int{1}}}, nil, []int{1}},
		{"a fix raises the count", []Part{{Number: 4, Of: 4}}, map[int]bool{1: true, 2: true, 3: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnswerMissing(tt.present, tt.applied); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("missing = %v, want %v", got, tt.want)
			}
		})
	}
}

// Each part keeps its own blocks; a block above the first label rides with
// the first part.
func TestPartTexts(t *testing.T) {
	paste := strings.Join([]string{
		"---\nname: Loose\n---\nBody.",
		"---\nkind: part\nname: A\npart: 2\n---", "---\nname: Two\n---\nBody.",
		"---\nkind: part\nname: A\npart: 1\n---", "---\nname: One\n---\nBody.",
	}, "\n")
	texts := PartTexts(Parse(paste))
	if len(texts) != 2 {
		t.Fatalf("texts = %q", texts)
	}
	var names [][]string
	for _, tx := range texts {
		var n []string
		for _, p := range Parse(tx) {
			n = append(n, p.Name)
		}
		names = append(names, n)
	}
	want := [][]string{{"A", "Loose", "Two"}, {"A", "One"}}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("parts = %v, want %v", names, want)
	}
}

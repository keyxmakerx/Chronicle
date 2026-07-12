package entities

import "testing"

func TestReindexForReorder(t *testing.T) {
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	tests := []struct {
		name    string
		ordered []string
		moved   string
		idx     int
		want    []string
	}{
		{"move last to front", []string{"a", "b", "c"}, "c", 0, []string{"c", "a", "b"}},
		{"move first to end", []string{"a", "b", "c"}, "a", 2, []string{"b", "c", "a"}},
		{"idempotent in place", []string{"a", "b", "c"}, "b", 1, []string{"a", "b", "c"}},
		{"oversized index clamps to append", []string{"a", "b", "c"}, "a", 99, []string{"b", "c", "a"}},
		{"negative index clamps to front", []string{"a", "b", "c"}, "b", -5, []string{"b", "a", "c"}},
		{"absent id is inserted at index", []string{"a", "b"}, "z", 1, []string{"a", "z", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reindexForReorder(tt.ordered, tt.moved, tt.idx)
			if !eq(got, tt.want) {
				t.Errorf("reindexForReorder(%v, %q, %d) = %v, want %v", tt.ordered, tt.moved, tt.idx, got, tt.want)
			}
		})
	}

	// Result is always dense 0..N-1 by construction (positions are the indices),
	// which is what the callers persist as sort_order — verify the int form too.
	gotInts := reindexForReorder([]int{10, 20, 30}, 30, 0)
	wantInts := []int{30, 10, 20}
	for i := range wantInts {
		if gotInts[i] != wantInts[i] {
			t.Fatalf("int reindex = %v, want %v", gotInts, wantInts)
		}
	}
}

func TestSameParent(t *testing.T) {
	a, b := "n1", "n2"
	cases := []struct {
		x, y *string
		want bool
	}{
		{nil, nil, true},
		{&a, nil, false},
		{nil, &a, false},
		{&a, &a, true},
		{&a, &b, false},
	}
	for _, c := range cases {
		if got := sameParent(c.x, c.y); got != c.want {
			t.Errorf("sameParent(%v, %v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

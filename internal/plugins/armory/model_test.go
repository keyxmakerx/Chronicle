package armory

import "testing"

func TestItemListOptions_Offset(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		perPage  int
		expected int
	}{
		{"page 1", 1, 24, 0},
		{"page 2", 2, 24, 24},
		{"page 3 small", 3, 10, 20},
		{"page 0 clamped", 0, 24, 0},
		{"negative page", -1, 24, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ItemListOptions{Page: tt.page, PerPage: tt.perPage}
			if got := opts.Offset(); got != tt.expected {
				t.Errorf("Offset() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestItemListOptions_OrderByClause(t *testing.T) {
	tests := []struct {
		sort     string
		expected string
	}{
		{"name", "ORDER BY e.name ASC"},
		{"updated", "ORDER BY e.updated_at DESC"},
		{"created", "ORDER BY e.created_at DESC"},
		{"", "ORDER BY e.name ASC"},
		{"unknown", "ORDER BY e.name ASC"},
	}

	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			opts := ItemListOptions{Sort: tt.sort}
			if got := opts.OrderByClause(); got != tt.expected {
				t.Errorf("OrderByClause() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDefaultItemListOptions(t *testing.T) {
	opts := DefaultItemListOptions()
	if opts.Page != 1 {
		t.Errorf("Page = %d, want 1", opts.Page)
	}
	if opts.PerPage != 24 {
		t.Errorf("PerPage = %d, want 24", opts.PerPage)
	}
	if opts.Sort != "name" {
		t.Errorf("Sort = %q, want %q", opts.Sort, "name")
	}
}

func TestTransactionListOptions_Offset(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		perPage  int
		expected int
	}{
		{"page 1", 1, 20, 0},
		{"page 2", 2, 20, 20},
		{"page 0 clamped", 0, 20, 0},
		{"negative page", -1, 20, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := TransactionListOptions{Page: tt.page, PerPage: tt.perPage}
			if got := opts.Offset(); got != tt.expected {
				t.Errorf("Offset() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestItemCard_FieldString(t *testing.T) {
	tests := []struct {
		name   string
		card   ItemCard
		key    string
		expect string
	}{
		{"existing string", ItemCard{Fields: map[string]any{"level": "5"}}, "level", "5"},
		{"missing key", ItemCard{Fields: map[string]any{"level": "5"}}, "hp", ""},
		{"nil fields", ItemCard{}, "level", ""},
		{"non-string value", ItemCard{Fields: map[string]any{"level": 5}}, "level", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.card.FieldString(tt.key); got != tt.expect {
				t.Errorf("FieldString(%q) = %q, want %q", tt.key, got, tt.expect)
			}
		})
	}
}

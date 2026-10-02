package admin

import (
	"strings"
	"testing"
)

func TestParseListQuery(t *testing.T) {
	long := strings.Repeat("é", 150)
	tests := []struct {
		name            string
		q, f, page      string
		wantQ, wantF    string
		wantPage        int
		wantQRuneLength int // 0 = skip
	}{
		{"defaults", "", "", "", "", "", 1, 0},
		{"trims q", "  ann  ", "", "", "ann", "", 1, 0},
		{"page zero", "", "", "0", "", "", 1, 0},
		{"page negative", "", "", "-4", "", "", 1, 0},
		{"page junk", "", "", "abc", "", "", 1, 0},
		{"page ok", "", "", "7", "", "", 7, 0},
		{"page huge", "", "", "99999999999999999999", "", "", 1, 0},
		{"page capped", "", "", "5000000", "", "", maxListPage, 0},
		{"filter trimmed", "", " admins ", "", "", "admins", 1, 0},
		{"caps long q by runes", long, "", "", "", "", 1, maxListSearchLen},
		{"strips control chars", "a\x00b\nc", "", "", "abc", "", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListQuery(tt.q, tt.f, tt.page)
			if tt.wantQRuneLength > 0 {
				if n := len([]rune(got.Q)); n != tt.wantQRuneLength {
					t.Errorf("q rune length = %d, want %d", n, tt.wantQRuneLength)
				}
			} else if got.Q != tt.wantQ {
				t.Errorf("Q = %q, want %q", got.Q, tt.wantQ)
			}
			if got.Filter != tt.wantF {
				t.Errorf("Filter = %q, want %q", got.Filter, tt.wantF)
			}
			if got.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", got.Page, tt.wantPage)
			}
		})
	}
}

func TestClampPage(t *testing.T) {
	tests := []struct {
		name                     string
		page, total, perPage, want int
	}{
		{"within", 2, 60, 25, 2},
		{"past end", 9, 60, 25, 3},
		{"empty list", 5, 0, 25, 1},
		{"exact multiple", 4, 100, 25, 4},
		{"one past exact multiple", 5, 100, 25, 4},
		{"bad per page", 3, 10, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampPage(tt.page, tt.total, tt.perPage); got != tt.want {
				t.Errorf("clampPage = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestListViewURLs(t *testing.T) {
	v := listView{BaseURL: "/admin/users", Query: "a&b c", Filter: "admins"}
	tests := []struct{ name, got, want string }{
		{"chip all keeps search", v.ChipURL(""), "/admin/users?q=a%26b+c"},
		{"chip filter", v.ChipURL("disabled"), "/admin/users?f=disabled&q=a%26b+c"},
		{"clear search keeps chip", v.ClearSearchURL(), "/admin/users?f=admins"},
		{"pager params", v.PagerParams(), "f=admins&q=a%26b+c"},
		{"bare", listView{BaseURL: "/admin/users"}.ChipURL(""), "/admin/users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

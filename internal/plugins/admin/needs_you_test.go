package admin

import "testing"

func TestBuildNeedsYou(t *testing.T) {
	tests := []struct {
		name     string
		in       needsInput
		wantHref []string
	}{
		{"nothing", needsInput{SMTPKnown: true, SMTPConfigured: true}, nil},
		{"smtp service not wired", needsInput{}, nil},
		{"smtp missing", needsInput{SMTPKnown: true}, []string{"/admin/smtp"}},
		{
			"everything, most urgent first",
			needsInput{UnhealthyPlugins: 1, PendingMigrations: 2, APIAlerts: 3, PendingSubmissions: 4, SMTPKnown: true},
			[]string{"/admin/systems", "/admin/database", "/admin/api", "/admin/packages/pending", "/admin/smtp"},
		},
		{
			"only some",
			needsInput{PendingSubmissions: 2, APIAlerts: 1, SMTPKnown: true, SMTPConfigured: true},
			[]string{"/admin/api", "/admin/packages/pending"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildNeedsYou(tc.in)
			if len(got) != len(tc.wantHref) {
				t.Fatalf("got %d items, want %d: %+v", len(got), len(tc.wantHref), got)
			}
			for i, h := range tc.wantHref {
				if got[i].Href != h {
					t.Errorf("item %d href = %q, want %q", i, got[i].Href, h)
				}
			}
		})
	}
}

func TestBuildNeedsYou_Wording(t *testing.T) {
	one := buildNeedsYou(needsInput{PendingSubmissions: 1})
	many := buildNeedsYou(needsInput{PendingSubmissions: 3})
	if one[0].Text == many[0].Text {
		t.Errorf("singular and plural sentences should differ, both %q", one[0].Text)
	}
}

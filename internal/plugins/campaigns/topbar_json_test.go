// topbar_json_test.go covers topbarStyleJSON/topbarContentJSON: the nil
// guards, a fully-populated round trip, and the control-character case a
// hand-rolled quote/backslash-only escaper gets wrong — a literal newline in
// a JSON string value is invalid JSON and must become the two-character
// escape \n.

package campaigns

import (
	"encoding/json"
	"testing"
)

func TestTopbarStyleJSON(t *testing.T) {
	cases := []struct {
		name  string
		style *TopbarStyle
		want  string
	}{
		{
			name:  "nil style",
			style: nil,
			want:  "{}",
		},
		{
			name:  "zero-value style: mode has no omitempty, everything else does",
			style: &TopbarStyle{},
			want:  `{"mode":""}`,
		},
		{
			name: "fully populated",
			style: &TopbarStyle{
				Mode:         "gradient",
				Color:        "#ff0000",
				GradientFrom: "#111111",
				GradientTo:   "#222222",
				GradientDir:  "to-r",
				ImagePath:    "2026/01/x.png",
			},
			want: `{"mode":"gradient","color":"#ff0000","gradient_from":"#111111","gradient_to":"#222222","gradient_dir":"to-r","image_path":"2026/01/x.png"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := topbarStyleJSON(tc.style)
			if got != tc.want {
				t.Errorf("topbarStyleJSON() = %s, want %s", got, tc.want)
			}
			if !json.Valid([]byte(got)) {
				t.Errorf("topbarStyleJSON() produced invalid JSON: %s", got)
			}
		})
	}
}

func TestTopbarContentJSON(t *testing.T) {
	cases := []struct {
		name    string
		content *TopbarContent
		want    string
	}{
		{
			name:    "nil content",
			content: nil,
			want:    `{"mode":"none","links":[],"quote":""}`,
		},
		{
			name:    "zero-value content: mode has no omitempty, quote/links do",
			content: &TopbarContent{},
			want:    `{"mode":""}`,
		},
		{
			// The case a hand-rolled escaper (quotes + backslashes only, no
			// control characters) gets wrong: a raw newline in the quote is
			// invalid inside a JSON string and must become \n.
			name:    "quote with an embedded newline",
			content: &TopbarContent{Mode: "quote", Quote: "line one\nline two"},
			want:    `{"mode":"quote","quote":"line one\nline two"}`,
		},
		{
			name: "links round-trip, including an icon-less link",
			content: &TopbarContent{
				Mode: "links",
				Links: []TopbarLink{
					{Label: "Wiki", URL: "/wiki", Icon: "fa-book"},
					{Label: "Forum", URL: "https://example.com/forum"},
				},
			},
			want: `{"mode":"links","links":[{"label":"Wiki","url":"/wiki","icon":"fa-book"},{"label":"Forum","url":"https://example.com/forum"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := topbarContentJSON(tc.content)
			if got != tc.want {
				t.Errorf("topbarContentJSON() = %s, want %s", got, tc.want)
			}
			if !json.Valid([]byte(got)) {
				t.Errorf("topbarContentJSON() produced invalid JSON: %s", got)
			}
		})
	}
}

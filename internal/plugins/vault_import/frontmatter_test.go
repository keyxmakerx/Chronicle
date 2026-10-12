package vault_import

import (
	"reflect"
	"testing"
)

func TestSplitFrontMatter(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		want     FrontMatter
		body     string
		hadBlock bool
		wantErr  bool
	}{
		{"none", "# Hi\ntext", FrontMatter{}, "# Hi\ntext", false, false},
		{"all keys", "---\nname: Bob\ntype: npc\nsubcategory: Baker\nvisibility: public\ntags: [a, b]\ndescription: A baker\n---\nBody",
			FrontMatter{Name: "Bob", Type: "npc", Subcategory: "Baker", Visibility: "public", Tags: []string{"a", "b"}, Description: "A baker"}, "Body", true, false},
		{"title is a name", "---\ntitle: Bob\n---\nx", FrontMatter{Name: "Bob"}, "x", true, false},
		{"name wins over title", "---\nname: A\ntitle: B\n---\nx", FrontMatter{Name: "A"}, "x", true, false},
		{"tags as string", "---\ntags: '#a, b'\n---\nx", FrontMatter{Tags: []string{"a", "b"}}, "x", true, false},
		{"tags as list with hash", "---\ntags:\n  - '#a'\n  - b\n---\nx", FrontMatter{Tags: []string{"a", "b"}}, "x", true, false},
		{"unknown keys ignored", "---\ncssclasses: [wide]\ndate: 2024-01-02\naliases: [Bobby]\n---\nx", FrontMatter{}, "x", true, false},
		{"numeric name does not reject the note", "---\nname: 42\n---\nx", FrontMatter{}, "x", true, false},
		{"type is lowered", "---\ntype: NPC\n---\nx", FrontMatter{Type: "npc"}, "x", true, false},
		{"crlf", "---\r\nname: Bob\r\n---\r\nBody", FrontMatter{Name: "Bob"}, "Body", true, false},
		{"bom", "\xef\xbb\xbf---\nname: Bob\n---\nBody", FrontMatter{Name: "Bob"}, "Body", true, false},
		{"dots close the block", "---\nname: Bob\n...\nBody", FrontMatter{Name: "Bob"}, "Body", true, false},
		{"empty block", "---\n---\nBody", FrontMatter{}, "Body", true, false},
		{"block not on first line is body", "\n---\nname: Bob\n---\nBody", FrontMatter{}, "\n---\nname: Bob\n---\nBody", false, false},
		{"unclosed block is body", "---\nname: Bob\nBody", FrontMatter{}, "---\nname: Bob\nBody", false, false},
		{"rule inside body is not a block", "Intro\n---\nmore", FrontMatter{}, "Intro\n---\nmore", false, false},
		{"invalid yaml keeps the text", "---\nname: [oops\n---\nBody", FrontMatter{}, "Body", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, had, err := SplitFrontMatter(tc.src)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if had != tc.hadBlock {
				t.Errorf("hadBlock = %v, want %v", had, tc.hadBlock)
			}
			if !reflect.DeepEqual(fm, tc.want) {
				t.Errorf("front matter = %+v, want %+v", fm, tc.want)
			}
			if body != tc.body {
				t.Errorf("body = %q, want %q", body, tc.body)
			}
		})
	}
}

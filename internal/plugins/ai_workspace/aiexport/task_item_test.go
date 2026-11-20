package aiexport

import "testing"

// A checklist is exported as "- [ ]" lines, so an AI that rewrites the page
// keeps its boxes; a plain list is exported as before.
func TestHTMLToMarkdownChecklist(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			"states are kept",
			`<ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>Hero built</p></li><li data-type="taskItem" data-checked="false"><p>Lines ready</p></li></ul>`,
			"- [x] Hero built\n\n- [ ] Lines ready",
		},
		{
			"marks inside an item survive",
			`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><p>Ask the <strong>DM</strong></p></li></ul>`,
			"- [ ] Ask the **DM**",
		},
		{
			"a plain list is untouched",
			`<ul><li><p>one</p></li><li><p>two</p></li></ul>`,
			"- one\n\n- two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			got, err := htmlToMarkdown(&in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

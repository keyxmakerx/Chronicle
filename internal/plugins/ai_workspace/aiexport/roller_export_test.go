package aiexport

import "testing"

func TestRollerExport(t *testing.T) {
	names := map[string]string{"tavern-names": "Tavern names", "odd": "Odd {name}", "a": "Twin", "b": "twin"}
	div := func(id, n string) string {
		return `<div class="ce-roll ce-roll--t-` + id + ` ce-roll--n-` + n + `"></div>`
	}
	tests := []struct {
		name    string
		in      string
		secrets bool
		want    string
	}{
		{"named", "<p>Hi</p>" + div("tavern-names", "5") + "<p>Bye</p>", true, "Hi\n\n{roll{Tavern names|5}}\n\nBye"},
		{"a built-in table by id", div("rumours", "1"), true, "{roll{rumours|1}}"},
		{"an odd count reads as 3", div("rumours", "7"), true, "{roll{rumours|3}}"},
		{"a name import can't read back", div("odd", "3"), true, "{roll{odd|3}}"},
		{"a name two tables share", div("a", "3"), true, "{roll{a|3}}"},
		{"dropped without secrets", "<p>Hi</p>" + div("tavern-names", "3") + "<p>Bye</p>", false, "Hi\n\nBye"},
		{"literal roller text is broken", "<p>write {roll{x}} here</p>", true, "write {\u200broll{x}} here"},
		{"literal roller text is broken without secrets", "<p>write {roll{x}} here</p>", false, "write {\u200broll{x}} here"},
		{"any other div is its text", `<div class="note">Plain</div>`, true, "Plain"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			var got string
			var err error
			if tc.secrets {
				got, err = htmlToMarkdownWithSecrets(&in, names)
			} else {
				got, err = htmlToMarkdown(&in)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

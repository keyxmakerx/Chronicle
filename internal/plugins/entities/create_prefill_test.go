package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestCreateFormPrefillName pins quick search's "Create a page named …":
// the name arrives as ?name=, is trimmed and cut to the field's limit, and
// the form shows it escaped in the name field.
func TestCreateFormPrefillName(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "Ferryman", "Ferryman"},
		{"trimmed", "  Port Rell ", "Port Rell"},
		{"cut to 200", strings.Repeat("é", 250), strings.Repeat("é", 200)},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := prefillName(tc.in); got != tc.want {
				t.Errorf("prefillName = %q, want %q", got, tc.want)
			}
		})
	}

	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}}
	var buf bytes.Buffer
	err := EntityCreateFormComponent(cc, []EntityType{{ID: 1, Name: "Character", Enabled: true}}, 1, nil, nil, `The "Bell"<b>`, "csrf", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `value="The &#34;Bell&#34;&lt;b&gt;"`) {
		t.Errorf("name field does not carry the escaped name")
	}
}

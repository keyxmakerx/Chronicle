package systems

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateManifest_Terms(t *testing.T) {
	tests := []struct {
		name    string
		terms   *ManifestTerms
		wantErr bool
	}{
		{"none", nil, false},
		{"empty gm", &ManifestTerms{}, false},
		{"one word", &ManifestTerms{GM: "Director"}, false},
		{"abbreviation", &ManifestTerms{GM: "DM"}, false},
		{"two words", &ManifestTerms{GM: "Dungeon Master"}, false},
		{"accented", &ManifestTerms{GM: "Maître"}, false},
		{"markup", &ManifestTerms{GM: "<b>GM</b>"}, true},
		{"quote", &ManifestTerms{GM: `GM"`}, true},
		{"apostrophe", &ManifestTerms{GM: "Keeper's"}, true},
		{"padded", &ManifestTerms{GM: " Director"}, true},
		{"leading digit", &ManifestTerms{GM: "1GM"}, true},
		{"too long", &ManifestTerms{GM: strings.Repeat("a", maxTermLen+1)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := panelManifest()
			m.Terms = tt.terms
			err := ValidateManifest(&m)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "terms:") {
				t.Fatalf("error = %v, want it to name terms", err)
			}
		})
	}
}

func TestManifestGMTerm(t *testing.T) {
	var m SystemManifest
	if err := json.Unmarshal([]byte(`{"terms":{"gm":"Director"}}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := m.GMTerm(); got != "Director" {
		t.Fatalf("GMTerm = %q, want Director", got)
	}
	if got := (&SystemManifest{}).GMTerm(); got != "" {
		t.Fatalf("no terms: GMTerm = %q, want empty", got)
	}
	var none *SystemManifest
	if got := none.GMTerm(); got != "" {
		t.Fatalf("nil manifest: GMTerm = %q, want empty", got)
	}
}

package systems

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// zipOf builds an in-memory ZIP from name → content.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const limitsData = `[{"slug":"goblin","name":"Goblin"}]`

// An owner's custom system is data only and bounded in size; the upload and
// its preview refuse the same things.
func TestCustomSystemUploadRules(t *testing.T) {
	withWidgets := strings.Replace(testManifestJSON, `"categories"`,
		`"widgets": [{"slug":"w","name":"W","script_file":"widgets/w.js"}], "categories"`, 1)
	many := map[string]string{"manifest.json": testManifestJSON, "data/creatures.json": limitsData}
	for i := 0; i < maxCustomSystemFiles; i++ {
		many[fmt.Sprintf("extra/%03d.txt", i)] = "x"
	}
	// Five files just under the per-file cap add up past the total.
	big := map[string]string{"manifest.json": testManifestJSON}
	pad := strings.Repeat(" ", maxDataFileSize-len(limitsData)-16)
	for i := 0; i < 5; i++ {
		big[fmt.Sprintf("data/c%d.json", i)] = limitsData + pad
	}

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{"data only installs", map[string]string{"manifest.json": testManifestJSON, "data/creatures.json": limitsData}, ""},
		{"a script file is refused", map[string]string{"manifest.json": testManifestJSON, "data/creatures.json": limitsData, "widgets/w.js": "alert(1)"}, "reference data only"},
		{"a script anywhere is refused", map[string]string{"manifest.json": testManifestJSON, "data/creatures.json": limitsData, "x/Y.JS": "alert(1)"}, "reference data only"},
		{"a manifest declaring widgets is refused", map[string]string{"manifest.json": withWidgets, "data/creatures.json": limitsData}, "reference data only"},
		{"too many files", many, "more than"},
		{"too much data in total", big, "in total"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := zipOf(t, tt.files)
			mgr := NewCampaignSystemManager(t.TempDir())
			_, err := mgr.Install("camp-1", bytes.NewReader(data), int64(len(data)))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("install: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("install err = %v, want %q", err, tt.wantErr)
			}
			if tt.name == "too much data in total" {
				return // the preview reads only declared categories
			}
			preview, err := PreviewFromZIP(data)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Valid {
				t.Fatalf("preview accepted what install refuses")
			}
		})
	}
}

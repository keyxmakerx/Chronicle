// Tests for the export-handler helpers added by the media-bundle work
// (Track F). Focus on the pure helpers — isZip and
// extractCampaignJSONFromZip — because they encode the security and
// format-detection contract that the rest of the handler depends on.
package campaigns

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// TestIsZip pins the magic-byte check used to route imports between
// the JSON and zip code paths.
func TestIsZip(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
		want  bool
	}{
		{"valid zip magic", []byte{0x50, 0x4B, 0x03, 0x04, 'x'}, true},
		{"json blob", []byte("{\"format\":\"chronicle\"}"), false},
		{"empty", nil, false},
		{"too short", []byte{0x50, 0x4B}, false},
		{"wrong magic", []byte{0xFF, 0xFF, 0xFF, 0xFF}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isZip(c.input); got != c.want {
				t.Errorf("isZip(%v) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// buildZip is a test helper that writes a zip with the given entries to
// an in-memory buffer. Returns the bytes for use in extract tests.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// TestExtractCampaignJSONFromZip_HappyPath confirms a well-formed bundle
// yields the JSON bytes verbatim plus a count of media entries.
func TestExtractCampaignJSONFromZip_HappyPath(t *testing.T) {
	bundle := buildZip(t, map[string]string{
		"campaign.json":     `{"format":"chronicle-campaign-v1","version":1}`,
		"media/abc.jpg":     "FAKE-IMAGE-BYTES",
		"media/def-thumb.png": "FAKE-THUMB-BYTES",
	})
	jsonBytes, mediaCount, err := extractCampaignJSONFromZip(bundle)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "chronicle-campaign-v1") {
		t.Errorf("JSON not extracted correctly: %s", jsonBytes)
	}
	if mediaCount != 2 {
		t.Errorf("mediaCount = %d, want 2", mediaCount)
	}
}

// TestExtractCampaignJSONFromZip_MissingCampaignJSON pins the safety
// check: a zip from somewhere else must not be silently accepted.
func TestExtractCampaignJSONFromZip_MissingCampaignJSON(t *testing.T) {
	bundle := buildZip(t, map[string]string{
		"some-other-thing.json": `{}`,
	})
	if _, _, err := extractCampaignJSONFromZip(bundle); err == nil {
		t.Fatal("expected error for zip missing campaign.json")
	}
}

// TestExtractCampaignJSONFromZip_InvalidZip refuses garbage with a clean
// error rather than panicking.
func TestExtractCampaignJSONFromZip_InvalidZip(t *testing.T) {
	if _, _, err := extractCampaignJSONFromZip([]byte("not a zip")); err == nil {
		t.Fatal("expected error for non-zip input")
	}
}

// TestExtractCampaignJSONFromZip_OversizedJSON refuses a zip whose
// embedded campaign.json exceeds the JSON-only cap. Without this an
// adversary could ship a multi-GB JSON inside a small zip and force
// the import service to allocate it.
func TestExtractCampaignJSONFromZip_OversizedJSON(t *testing.T) {
	huge := strings.Repeat("a", maxImportSize+10)
	bundle := buildZip(t, map[string]string{
		"campaign.json": huge,
	})
	if _, _, err := extractCampaignJSONFromZip(bundle); err == nil {
		t.Fatal("expected error for oversized campaign.json inside zip")
	}
}

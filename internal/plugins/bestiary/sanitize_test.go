package bestiary

import (
	"encoding/json"
	"strings"
	"testing"
)

// These tests pin the DS-SEC-AUDIT-R1 CRITICAL fix: statblock strings are
// stripped of HTML metacharacters on write and on read, so widget innerHTML
// interpolation of organization/role/size/etc. cannot execute
// attacker-authored markup.

func TestSanitizeStatblockJSON_StripsAnglesEverywhere(t *testing.T) {
	raw := json.RawMessage(`{
		"name": "Goblin <img src=x onerror=alert(1)>",
		"organization": "<script>steal()</script>horde",
		"role": "ambusher",
		"level": 2,
		"size": "1M <b>huge</b>",
		"keywords": ["goblin", "<svg onload=1>humanoid"],
		"abilities": {"Bite <i>": {"effect": "2 <img> damage", "cost": 3}},
		"traits": [{"name": "Sneak<>", "tiers": [1, "t2 <a>", true]}]
	}`)

	clean := sanitizeStatblockJSON(raw)

	if strings.ContainsAny(string(clean), "<>") {
		t.Fatalf("sanitized statblock still contains angle brackets:\n%s", clean)
	}

	// Structure and non-string values survive.
	var v struct {
		Name      string           `json:"name"`
		Org       string           `json:"organization"`
		Level     int              `json:"level"`
		Keywords  []string         `json:"keywords"`
		Abilities map[string]any   `json:"abilities"`
		Traits    []map[string]any `json:"traits"`
	}
	if err := json.Unmarshal(clean, &v); err != nil {
		t.Fatalf("sanitized statblock is not valid JSON: %v", err)
	}
	if v.Level != 2 {
		t.Errorf("level mutated: got %d", v.Level)
	}
	if v.Org != "scriptsteal()/scripthorde" {
		t.Errorf("organization not stripped as expected: %q", v.Org)
	}
	if len(v.Keywords) != 2 || v.Keywords[1] != "svg onload=1humanoid" {
		t.Errorf("array strings not stripped: %#v", v.Keywords)
	}
	if _, ok := v.Abilities["Bite i"]; !ok {
		t.Errorf("map keys not stripped: %#v", v.Abilities)
	}
}

func TestSanitizeStatblockJSON_FastPathReturnsInputUnchanged(t *testing.T) {
	raw := json.RawMessage(`{"name":"Clean Goblin","level":1}`)
	clean := sanitizeStatblockJSON(raw)
	if string(clean) != string(raw) {
		t.Errorf("clean input should be returned byte-identical, got %s", clean)
	}
}

func TestSanitizeStatblockJSON_Idempotent(t *testing.T) {
	raw := json.RawMessage(`{"name":"G","organization":"<b>horde&quot;</b>"}`)
	once := sanitizeStatblockJSON(raw)
	twice := sanitizeStatblockJSON(once)
	if string(once) != string(twice) {
		t.Errorf("sanitize not idempotent:\nonce:  %s\ntwice: %s", once, twice)
	}
}

func TestSanitizePublicationInPlace_ScrubsDenormalizedColumns(t *testing.T) {
	org := `<img src=x onerror=alert(1)>horde`
	role := "ambusher"
	p := &Publication{
		StatblockJSON: json.RawMessage(`{"name":"G <b>","organization":"<x>h"}`),
		Organization:  &org,
		Role:          &role,
	}

	sanitizePublicationInPlace(p)

	if strings.ContainsAny(string(p.StatblockJSON), "<>") {
		t.Errorf("statblock still has angles: %s", p.StatblockJSON)
	}
	if *p.Organization != "img src=x onerror=alert(1)horde" {
		t.Errorf("organization column not stripped: %q", *p.Organization)
	}
	if *p.Role != "ambusher" {
		t.Errorf("clean role mutated: %q", *p.Role)
	}
	// Nil-safe.
	sanitizePublicationInPlace(nil)
	sanitizePublicationInPlace(&Publication{})
}

func TestSanitizeStatblockJSON_MalformedReturnsInput(t *testing.T) {
	raw := json.RawMessage(`{"name": <not json`)
	if got := sanitizeStatblockJSON(raw); string(got) != string(raw) {
		t.Errorf("malformed input should pass through for the read path, got %s", got)
	}
}

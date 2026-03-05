package campaigns

import (
	"encoding/json"
	"testing"
)

func TestDetectCampaignExport_ValidFormat(t *testing.T) {
	export := CampaignExport{
		Format:  ExportFormat,
		Version: ExportVersion,
		Campaign: ExportCampaignMeta{
			Name: "Test Campaign",
		},
	}
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}

	result, err := DetectCampaignExport(data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Campaign.Name != "Test Campaign" {
		t.Errorf("expected name 'Test Campaign', got %q", result.Campaign.Name)
	}
}

func TestDetectCampaignExport_InvalidJSON(t *testing.T) {
	_, err := DetectCampaignExport([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDetectCampaignExport_WrongFormat(t *testing.T) {
	data := []byte(`{"format": "unknown", "version": 1}`)
	_, err := DetectCampaignExport(data)
	if err == nil {
		t.Fatal("expected error for wrong format")
	}
}

func TestDetectCampaignExport_UnsupportedVersion(t *testing.T) {
	data := []byte(`{"format": "chronicle-campaign-v1", "version": 999}`)
	_, err := DetectCampaignExport(data)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestDetectCampaignExport_MissingName(t *testing.T) {
	data := []byte(`{"format": "chronicle-campaign-v1", "version": 1, "campaign": {}}`)
	_, err := DetectCampaignExport(data)
	if err == nil {
		t.Fatal("expected error for missing campaign name")
	}
}

func TestNewIDMap(t *testing.T) {
	idMap := NewIDMap("camp-1")
	if idMap.CampaignID != "camp-1" {
		t.Errorf("expected campaign ID 'camp-1', got %q", idMap.CampaignID)
	}
	if idMap.EntityTypeIDs == nil {
		t.Error("EntityTypeIDs should be initialized")
	}
	if idMap.EntityIDs == nil {
		t.Error("EntityIDs should be initialized")
	}
}

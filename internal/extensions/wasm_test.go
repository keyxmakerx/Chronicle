package extensions

import (
	"encoding/json"
	"testing"
)

// TestWASMManifestValidation tests that WASM plugin declarations in manifests
// are validated correctly.
func TestWASMManifestValidation(t *testing.T) {
	tests := []struct {
		name    string
		contrib WASMContribution
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid WASM plugin",
			contrib: WASMContribution{
				Slug:         "dice-calc",
				Name:         "Dice Calculator",
				File:         "plugins/dice-calc.wasm",
				Capabilities: []string{"log", "entity_read"},
				Hooks:        []string{"entity.created"},
			},
			wantErr: false,
		},
		{
			name: "missing slug",
			contrib: WASMContribution{
				Name:         "Test",
				File:         "test.wasm",
				Capabilities: []string{"log"},
			},
			wantErr: true,
			errMsg:  "slug is required",
		},
		{
			name: "missing name",
			contrib: WASMContribution{
				Slug:         "test",
				File:         "test.wasm",
				Capabilities: []string{"log"},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "missing file",
			contrib: WASMContribution{
				Slug:         "test",
				Name:         "Test",
				Capabilities: []string{"log"},
			},
			wantErr: true,
			errMsg:  "file is required",
		},
		{
			name: "non-wasm file extension",
			contrib: WASMContribution{
				Slug:         "test",
				Name:         "Test",
				File:         "plugins/test.js",
				Capabilities: []string{"log"},
			},
			wantErr: true,
			errMsg:  "must be a .wasm file",
		},
		{
			name: "path traversal in file",
			contrib: WASMContribution{
				Slug:         "test",
				Name:         "Test",
				File:         "../../../etc/passwd.wasm",
				Capabilities: []string{"log"},
			},
			wantErr: true,
			errMsg:  "path traversal",
		},
		{
			name: "no capabilities",
			contrib: WASMContribution{
				Slug: "test",
				Name: "Test",
				File: "test.wasm",
			},
			wantErr: true,
			errMsg:  "at least one capability is required",
		},
		{
			name: "unknown capability",
			contrib: WASMContribution{
				Slug:         "test",
				Name:         "Test",
				File:         "test.wasm",
				Capabilities: []string{"log", "hack_the_planet"},
			},
			wantErr: true,
			errMsg:  "unknown capability",
		},
		{
			name: "unknown hook type",
			contrib: WASMContribution{
				Slug:         "test",
				Name:         "Test",
				File:         "test.wasm",
				Capabilities: []string{"log"},
				Hooks:        []string{"entity.exploded"},
			},
			wantErr: true,
			errMsg:  "unknown hook type",
		},
		{
			name: "memory limit too high",
			contrib: WASMContribution{
				Slug:          "test",
				Name:          "Test",
				File:          "test.wasm",
				Capabilities:  []string{"log"},
				MemoryLimitMB: 512,
			},
			wantErr: true,
			errMsg:  "memory_limit_mb cannot exceed 256",
		},
		{
			name: "timeout too high",
			contrib: WASMContribution{
				Slug:         "test",
				Name:         "Test",
				File:         "test.wasm",
				Capabilities: []string{"log"},
				TimeoutSecs:  600,
			},
			wantErr: true,
			errMsg:  "timeout_secs cannot exceed 300",
		},
		{
			name: "all capabilities valid",
			contrib: WASMContribution{
				Slug:         "full-access",
				Name:         "Full Access Plugin",
				File:         "full.wasm",
				Capabilities: []string{"log", "entity_read", "calendar_read", "tag_read", "kv_store"},
			},
			wantErr: false,
		},
		{
			name: "all hooks valid",
			contrib: WASMContribution{
				Slug:         "all-hooks",
				Name:         "All Hooks Plugin",
				File:         "hooks.wasm",
				Capabilities: []string{"log"},
				Hooks: []string{
					"entity.created", "entity.updated", "entity.deleted",
					"calendar.event_created", "calendar.event_updated", "calendar.event_deleted",
					"tag.added", "tag.removed",
				},
			},
			wantErr: false,
		},
		{
			name: "valid with config and limits",
			contrib: WASMContribution{
				Slug:          "custom-limits",
				Name:          "Custom Limits Plugin",
				File:          "custom.wasm",
				Capabilities:  []string{"log", "kv_store"},
				MemoryLimitMB: 64,
				TimeoutSecs:   60,
				Config: []WASMConfigField{
					{Key: "threshold", Label: "Threshold", Type: "number", Default: "10"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := &ExtensionManifest{
				ManifestVersion: 1,
				ID:              "test-ext",
				Name:            "Test Extension",
				Version:         "1.0.0",
				Description:     "Test extension for WASM validation",
				Contributes: &ManifestContributes{
					WASMPlugins: []WASMContribution{tt.contrib},
				},
			}

			err := ValidateManifest(manifest)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestWASMModelDefaults tests the default WASM limits.
func TestWASMModelDefaults(t *testing.T) {
	limits := DefaultWASMLimits()

	if limits.MemoryLimitBytes != 16*1024*1024 {
		t.Errorf("expected default memory limit 16 MB, got %d", limits.MemoryLimitBytes)
	}
	if limits.TimeoutMS != 30000 {
		t.Errorf("expected default timeout 30000ms, got %d", limits.TimeoutMS)
	}
	if limits.FuelLimit != 0 {
		t.Errorf("expected default fuel limit 0, got %d", limits.FuelLimit)
	}
}

// TestWASMCapabilities tests the capability constants.
func TestWASMCapabilities(t *testing.T) {
	expected := map[string]bool{
		"log":            true,
		"entity_read":    true,
		"entity_write":   true,
		"calendar_read":  true,
		"calendar_write": true,
		"tag_read":       true,
		"tag_write":      true,
		"relation_write": true,
		"kv_store":       true,
		"message":        true,
	}

	if len(AllCapabilities) != len(expected) {
		t.Errorf("expected %d capabilities, got %d", len(expected), len(AllCapabilities))
	}

	for cap := range expected {
		if !AllCapabilities[cap] {
			t.Errorf("capability %q missing from AllCapabilities", cap)
		}
	}
}

// TestWASMHookTypes tests the hook type constants.
func TestWASMHookTypes(t *testing.T) {
	expected := []string{
		"entity.created", "entity.updated", "entity.deleted",
		"calendar.event_created", "calendar.event_updated", "calendar.event_deleted",
		"tag.added", "tag.removed",
		"campaign.deleted",
	}

	if len(ValidHookTypes) != len(expected) {
		t.Errorf("expected %d hook types, got %d", len(expected), len(ValidHookTypes))
	}

	for _, hook := range expected {
		if !ValidHookTypes[hook] {
			t.Errorf("hook type %q missing from ValidHookTypes", hook)
		}
	}
}

// TestPluginKey tests the plugin key generation.
func TestPluginKey(t *testing.T) {
	key := pluginKey("my-ext", "dice-roller")
	if key != "my-ext:dice-roller" {
		t.Errorf("expected key 'my-ext:dice-roller', got %q", key)
	}
}

// TestWASMCallResponse tests serialization of call responses.
func TestWASMCallResponse(t *testing.T) {
	resp := WASMCallResponse{
		Output: json.RawMessage(`{"result": 42}`),
		Logs:   []string{"computed value", "returning"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var decoded WASMCallResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if string(decoded.Output) != `{"result":42}` {
		t.Errorf("unexpected output: %s", decoded.Output)
	}
	if len(decoded.Logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(decoded.Logs))
	}
}

// TestWASMCallResponseError tests error response serialization.
func TestWASMCallResponseError(t *testing.T) {
	resp := WASMCallResponse{
		Error: "function not found",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal error response: %v", err)
	}

	var decoded WASMCallResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	if decoded.Error != "function not found" {
		t.Errorf("unexpected error: %s", decoded.Error)
	}
	if decoded.Output != nil {
		t.Errorf("expected nil output on error, got %s", decoded.Output)
	}
}

// TestHookEvent tests hook event serialization.
func TestHookEvent(t *testing.T) {
	event := HookEvent{
		Type:       HookEntityCreated,
		CampaignID: "campaign-123",
		Payload:    json.RawMessage(`{"entity_id": "ent-456", "name": "Gandalf"}`),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal hook event: %v", err)
	}

	var decoded HookEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal hook event: %v", err)
	}

	if decoded.Type != HookEntityCreated {
		t.Errorf("expected type %q, got %q", HookEntityCreated, decoded.Type)
	}
	if decoded.CampaignID != "campaign-123" {
		t.Errorf("expected campaign_id 'campaign-123', got %q", decoded.CampaignID)
	}
}

// TestWASMPluginManagerListEmpty tests listing when no plugins are loaded.
func TestWASMPluginManagerListEmpty(t *testing.T) {
	pm := NewPluginManager("/tmp/nonexistent", nil)
	plugins := pm.ListPlugins()

	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

// TestWASMPluginManagerGetNotFound tests getting a non-existent plugin.
func TestWASMPluginManagerGetNotFound(t *testing.T) {
	pm := NewPluginManager("/tmp/nonexistent", nil)
	_, found := pm.GetPlugin("nonexistent", "nope")

	if found {
		t.Error("expected plugin not found")
	}
}

// TestWASMPluginManagerPluginsForHookEmpty tests hook dispatch with no plugins.
func TestWASMPluginManagerPluginsForHookEmpty(t *testing.T) {
	pm := NewPluginManager("/tmp/nonexistent", nil)
	targets := pm.PluginsForHook(HookEntityCreated)

	if len(targets) != 0 {
		t.Errorf("expected 0 hook targets, got %d", len(targets))
	}
}

// TestWASMManifestWithPlugins tests full manifest parsing with WASM plugins.
func TestWASMManifestWithPlugins(t *testing.T) {
	manifestJSON := `{
		"manifest_version": 1,
		"id": "wasm-test-ext",
		"name": "WASM Test Extension",
		"version": "1.0.0",
		"description": "Extension with WASM plugins for testing",
		"contributes": {
			"wasm_plugins": [
				{
					"slug": "dice-roller",
					"name": "Dice Roller",
					"description": "Roll dice with custom formulas",
					"file": "plugins/dice-roller.wasm",
					"capabilities": ["log", "entity_read", "kv_store"],
					"hooks": ["entity.created"],
					"memory_limit_mb": 32,
					"timeout_secs": 10,
					"config": [
						{
							"key": "default_sides",
							"label": "Default Dice Sides",
							"type": "number",
							"default": "20"
						}
					]
				}
			]
		}
	}`

	manifest, err := ParseManifest([]byte(manifestJSON))
	if err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if manifest.Contributes == nil {
		t.Fatal("expected contributes to be non-nil")
	}

	if len(manifest.Contributes.WASMPlugins) != 1 {
		t.Fatalf("expected 1 WASM plugin, got %d", len(manifest.Contributes.WASMPlugins))
	}

	wp := manifest.Contributes.WASMPlugins[0]
	if wp.Slug != "dice-roller" {
		t.Errorf("expected slug 'dice-roller', got %q", wp.Slug)
	}
	if wp.Name != "Dice Roller" {
		t.Errorf("expected name 'Dice Roller', got %q", wp.Name)
	}
	if wp.File != "plugins/dice-roller.wasm" {
		t.Errorf("expected file 'plugins/dice-roller.wasm', got %q", wp.File)
	}
	if len(wp.Capabilities) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(wp.Capabilities))
	}
	if len(wp.Hooks) != 1 {
		t.Errorf("expected 1 hook, got %d", len(wp.Hooks))
	}
	if wp.MemoryLimitMB != 32 {
		t.Errorf("expected memory_limit_mb 32, got %d", wp.MemoryLimitMB)
	}
	if wp.TimeoutSecs != 10 {
		t.Errorf("expected timeout_secs 10, got %d", wp.TimeoutSecs)
	}
	if len(wp.Config) != 1 {
		t.Errorf("expected 1 config field, got %d", len(wp.Config))
	}
}

// TestWASMSecurityAllowlist tests that .wasm files are in the allowlist.
func TestWASMSecurityAllowlist(t *testing.T) {
	if !allowedFileExts[".wasm"] {
		t.Error("expected .wasm to be in the allowedFileExts allowlist")
	}
}

// TestWASMZipEntryValidation tests that .wasm zip entries pass validation.
func TestWASMZipEntryValidation(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid wasm file", "plugins/dice-roller.wasm", false},
		{"wasm in root", "main.wasm", false},
		{"wasm in nested dir", "plugins/v2/calculator.wasm", false},
		{"path traversal with wasm", "../evil.wasm", true},
		{"absolute path wasm", "/etc/evil.wasm", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateZipEntry(tt.path)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestWithCampaignID tests the context helper.
func TestWithCampaignID(t *testing.T) {
	ctx := WithCampaignID(t.Context(), "camp-123")
	val, ok := ctx.Value(contextKeyCampaignID).(string)

	if !ok {
		t.Fatal("expected campaign ID in context")
	}
	if val != "camp-123" {
		t.Errorf("expected 'camp-123', got %q", val)
	}
}

// TestWithExtensionID tests the extension ID context helper.
func TestWithExtensionID(t *testing.T) {
	ctx := WithExtensionID(t.Context(), "my-ext")
	val, ok := ctx.Value(contextKeyExtensionID).(string)

	if !ok {
		t.Fatal("expected extension ID in context")
	}
	if val != "my-ext" {
		t.Errorf("expected 'my-ext', got %q", val)
	}
}

// TestHostEnvironmentLogDrain tests the log accumulation and drain.
func TestHostEnvironmentLogDrain(t *testing.T) {
	env := NewHostEnvironment(nil, nil, nil, nil)
	env.SetCallContext("ext-1", "slug-1", "camp-1")

	env.appendLog("ext-1", "slug-1", "first log")
	env.appendLog("ext-1", "slug-1", "second log")

	logs := env.DrainLogs("ext-1", "slug-1")
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0] != "first log" {
		t.Errorf("expected 'first log', got %q", logs[0])
	}

	// Drain again should be empty.
	logs2 := env.DrainLogs("ext-1", "slug-1")
	if len(logs2) != 0 {
		t.Errorf("expected 0 logs after drain, got %d", len(logs2))
	}

	env.ClearCallContext("ext-1", "slug-1")
}

// TestHostEnvironmentLogLimit tests that log accumulation is capped at 100.
func TestHostEnvironmentLogLimit(t *testing.T) {
	env := NewHostEnvironment(nil, nil, nil, nil)
	env.SetCallContext("ext-1", "slug-1", "camp-1")

	for i := 0; i < 150; i++ {
		env.appendLog("ext-1", "slug-1", "log message")
	}

	logs := env.DrainLogs("ext-1", "slug-1")
	if len(logs) != 100 {
		t.Errorf("expected 100 logs (capped), got %d", len(logs))
	}

	env.ClearCallContext("ext-1", "slug-1")
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

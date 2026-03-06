package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/tetratelabs/wazero"
)

// PluginManager manages the lifecycle of WASM logic extensions.
// It loads, unloads, and calls WASM plugins via the Extism SDK with
// capability-based security: each plugin only receives host functions
// matching its declared capabilities.
type PluginManager struct {
	mu       sync.RWMutex
	plugins  map[string]*loadedPlugin // keyed by "extID:slug".
	extDir   string                   // Root dir for extension files.
	hostEnv  *HostEnvironment         // Provides host functions to plugins.
	limits   WASMLimits               // Default resource limits.
}

// loadedPlugin wraps an Extism plugin instance with metadata.
type loadedPlugin struct {
	plugin       *extism.Plugin
	info         WASMPluginInfo
	contribution WASMContribution
	cancel       context.CancelFunc // For timeout enforcement.
}

// NewPluginManager creates a new WASM plugin manager.
func NewPluginManager(extDir string, hostEnv *HostEnvironment) *PluginManager {
	return &PluginManager{
		plugins: make(map[string]*loadedPlugin),
		extDir:  extDir,
		hostEnv: hostEnv,
		limits:  DefaultWASMLimits(),
	}
}

// pluginKey creates the unique key for a plugin instance.
func pluginKey(extID, slug string) string {
	return extID + ":" + slug
}

// Load loads a WASM plugin from its extension directory. The plugin is
// instantiated with only the host functions matching its declared capabilities.
func (pm *PluginManager) Load(ctx context.Context, extID string, contrib WASMContribution) error {
	key := pluginKey(extID, contrib.Slug)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Unload existing instance if present.
	if existing, ok := pm.plugins[key]; ok {
		existing.plugin.Close(ctx)
		if existing.cancel != nil {
			existing.cancel()
		}
		delete(pm.plugins, key)
	}

	// Read the WASM binary.
	wasmPath := filepath.Join(pm.extDir, extID, contrib.File)
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("reading WASM file %s: %w", wasmPath, err)
	}

	// Determine resource limits.
	limits := pm.limits
	if contrib.MemoryLimitMB > 0 {
		limits.MemoryLimitBytes = uint64(contrib.MemoryLimitMB) * 1024 * 1024
	}
	if contrib.TimeoutSecs > 0 {
		limits.TimeoutMS = int64(contrib.TimeoutSecs) * 1000
	}

	// Build the set of allowed capabilities.
	capSet := make(map[string]bool, len(contrib.Capabilities))
	for _, c := range contrib.Capabilities {
		capSet[c] = true
	}

	// Build host functions based on declared capabilities.
	hostFunctions := pm.hostEnv.BuildHostFunctions(capSet)

	// Create Extism manifest for the plugin.
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		Memory: &extism.ManifestMemory{
			MaxPages: uint32(limits.MemoryLimitBytes / 65536), // WASM page = 64KB.
		},
		Config:       map[string]string{},
		AllowedHosts: []string{}, // No network access.
	}

	// Create the Extism plugin with host functions.
	pluginConfig := extism.PluginConfig{
		ModuleConfig:  wazero.NewModuleConfig().WithSysNanosleep().WithSysWalltime(),
		EnableWasi:    true,
		RuntimeConfig: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
	}

	p, err := extism.NewPlugin(ctx, manifest, pluginConfig, hostFunctions)
	if err != nil {
		slog.Error("failed to load WASM plugin",
			slog.String("ext_id", extID),
			slog.String("slug", contrib.Slug),
			slog.Any("error", err),
		)
		// Record the error state.
		pm.plugins[key] = &loadedPlugin{
			info: WASMPluginInfo{
				ExtID:        extID,
				Slug:         contrib.Slug,
				Name:         contrib.Name,
				Version:      "",
				Status:       WASMStatusError,
				Capabilities: contrib.Capabilities,
				Hooks:        contrib.Hooks,
				LoadedAt:     time.Now().UTC(),
				ErrorMsg:     err.Error(),
			},
			contribution: contrib,
		}
		return fmt.Errorf("creating WASM plugin: %w", err)
	}

	pm.plugins[key] = &loadedPlugin{
		plugin: p,
		info: WASMPluginInfo{
			ExtID:        extID,
			Slug:         contrib.Slug,
			Name:         contrib.Name,
			Status:       WASMStatusLoaded,
			Capabilities: contrib.Capabilities,
			Hooks:        contrib.Hooks,
			LoadedAt:     time.Now().UTC(),
		},
		contribution: contrib,
	}

	slog.Info("WASM plugin loaded",
		slog.String("ext_id", extID),
		slog.String("slug", contrib.Slug),
		slog.String("name", contrib.Name),
		slog.Int("capabilities", len(contrib.Capabilities)),
		slog.Int("hooks", len(contrib.Hooks)),
	)

	return nil
}

// Unload stops and removes a WASM plugin.
func (pm *PluginManager) Unload(ctx context.Context, extID, slug string) error {
	key := pluginKey(extID, slug)

	pm.mu.Lock()
	defer pm.mu.Unlock()

	lp, ok := pm.plugins[key]
	if !ok {
		return fmt.Errorf("plugin %s not loaded", key)
	}

	if lp.plugin != nil {
		lp.plugin.Close(ctx)
	}
	if lp.cancel != nil {
		lp.cancel()
	}
	delete(pm.plugins, key)

	slog.Info("WASM plugin unloaded",
		slog.String("ext_id", extID),
		slog.String("slug", slug),
	)

	return nil
}

// UnloadAll stops all loaded WASM plugins. Called during graceful shutdown.
func (pm *PluginManager) UnloadAll(ctx context.Context) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for key, lp := range pm.plugins {
		if lp.plugin != nil {
			lp.plugin.Close(ctx)
		}
		if lp.cancel != nil {
			lp.cancel()
		}
		delete(pm.plugins, key)
	}

	slog.Info("all WASM plugins unloaded")
}

// Reload unloads and reloads a plugin from disk.
func (pm *PluginManager) Reload(ctx context.Context, extID, slug string) error {
	key := pluginKey(extID, slug)

	pm.mu.RLock()
	lp, ok := pm.plugins[key]
	pm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin %s not loaded", key)
	}

	return pm.Load(ctx, extID, lp.contribution)
}

// Call invokes a named function on a WASM plugin with the given input.
// The call is wrapped with a timeout derived from the plugin's limits.
func (pm *PluginManager) Call(ctx context.Context, extID, slug, function string, input json.RawMessage) (*WASMCallResponse, error) {
	key := pluginKey(extID, slug)

	pm.mu.RLock()
	lp, ok := pm.plugins[key]
	pm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plugin %s not loaded", key)
	}
	if lp.plugin == nil {
		return nil, fmt.Errorf("plugin %s is in error state: %s", key, lp.info.ErrorMsg)
	}

	// Determine timeout.
	timeoutMS := pm.limits.TimeoutMS
	if lp.contribution.TimeoutSecs > 0 {
		timeoutMS = int64(lp.contribution.TimeoutSecs) * 1000
	}

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	// Set campaign context for host functions.
	if campaignID, ok := ctx.Value(contextKeyCampaignID).(string); ok {
		pm.hostEnv.SetCallContext(extID, slug, campaignID)
		defer pm.hostEnv.ClearCallContext(extID, slug)
	}

	// Prepare input bytes.
	var inputBytes []byte
	if input != nil {
		inputBytes = []byte(input)
	}

	// Check if function exists before calling.
	if !lp.plugin.FunctionExists(function) {
		return nil, fmt.Errorf("function %q not exported by plugin %s", function, key)
	}

	// Call the WASM function.
	_, output, err := lp.plugin.CallWithContext(callCtx, function, inputBytes)
	if err != nil {
		slog.Warn("WASM plugin call failed",
			slog.String("plugin", key),
			slog.String("function", function),
			slog.Any("error", err),
		)
		return &WASMCallResponse{
			Error: err.Error(),
		}, nil
	}

	// Collect logs from host environment.
	logs := pm.hostEnv.DrainLogs(extID, slug)

	return &WASMCallResponse{
		Output: json.RawMessage(output),
		Logs:   logs,
	}, nil
}

// ListPlugins returns info about all loaded plugins.
func (pm *PluginManager) ListPlugins() []WASMPluginInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]WASMPluginInfo, 0, len(pm.plugins))
	for _, lp := range pm.plugins {
		result = append(result, lp.info)
	}
	return result
}

// GetPlugin returns info about a specific plugin.
func (pm *PluginManager) GetPlugin(extID, slug string) (*WASMPluginInfo, bool) {
	key := pluginKey(extID, slug)

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	lp, ok := pm.plugins[key]
	if !ok {
		return nil, false
	}
	return &lp.info, true
}

// PluginsForHook returns all loaded plugins that listen for a given hook event type.
func (pm *PluginManager) PluginsForHook(hookType string) []pluginHookTarget {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var targets []pluginHookTarget
	for _, lp := range pm.plugins {
		if lp.plugin == nil || lp.info.Status != WASMStatusLoaded {
			continue
		}
		for _, h := range lp.contribution.Hooks {
			if h == hookType {
				targets = append(targets, pluginHookTarget{
					extID: lp.info.ExtID,
					slug:  lp.info.Slug,
				})
				break
			}
		}
	}
	return targets
}

// pluginHookTarget identifies a plugin that should receive a hook event.
type pluginHookTarget struct {
	extID string
	slug  string
}

// contextKey is used to pass campaign ID through context.
type contextKey string

const contextKeyCampaignID contextKey = "wasm_campaign_id"

// WithCampaignID returns a context with the campaign ID set for WASM calls.
func WithCampaignID(ctx context.Context, campaignID string) context.Context {
	return context.WithValue(ctx, contextKeyCampaignID, campaignID)
}

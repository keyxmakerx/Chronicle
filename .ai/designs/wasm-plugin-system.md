# WASM Plugin System Design: Extism + wazero Integration

**Status:** Design Document (not yet implemented)
**Date:** 2026-03-06
**Scope:** Adding sandboxed WebAssembly plugin execution to Chronicle via Extism Go SDK

---

## Table of Contents

1. [Extism Go SDK Integration](#1-extism-go-sdk-integration)
2. [Plugin Lifecycle](#2-plugin-lifecycle)
3. [Host Function Design for Chronicle](#3-host-function-design-for-chronicle)
4. [Security Model](#4-security-model)
5. [Plugin Development Experience](#5-plugin-development-experience)
6. [Performance Characteristics](#6-performance-characteristics)
7. [Real-World Examples & Lessons](#7-real-world-examples--lessons)

---

## 1. Extism Go SDK Integration

### Dependencies

```go
// go.mod
require (
    github.com/extism/go-sdk v1.x.x
    github.com/tetratelabs/wazero v1.x.x // transitive, via go-sdk
)
```

The Extism Go SDK is a pure-Go library built on top of wazero -- no CGO, no
shared libraries, no cross-compilation headaches. It adds roughly 5-10 MB to the
binary.

### Loading a .wasm Plugin from Disk

```go
package wasmrt

import (
    "context"
    "fmt"

    extism "github.com/extism/go-sdk"
    "github.com/tetratelabs/wazero"
)

// LoadPlugin loads a compiled .wasm file from disk and returns a ready plugin.
func LoadPlugin(ctx context.Context, path string, hostFns []extism.HostFunction) (*extism.Plugin, error) {
    manifest := extism.Manifest{
        Wasm: []extism.Wasm{
            extism.WasmFile{Path: path},
        },
        // 5-second timeout prevents infinite loops.
        Timeout: 5000,
        // Cap linear memory at 16 MB (256 pages * 64 KB).
        Memory: struct {
            MaxPages uint32 `json:"max_pages,omitempty"`
        }{MaxPages: 256},
        // No network access by default.
        AllowedHosts: []string{},
        // No filesystem access by default.
        AllowedPaths: map[string]string{},
    }

    config := extism.PluginConfig{
        EnableWasi:   true,
        ModuleConfig: wazero.NewModuleConfig().WithSysWalltime(),
    }

    plugin, err := extism.NewPlugin(ctx, manifest, config, hostFns)
    if err != nil {
        return nil, fmt.Errorf("wasmrt: failed to load plugin %s: %w", path, err)
    }
    return plugin, nil
}
```

### Calling a Function in the Plugin

```go
// CallPlugin invokes an exported function on the plugin with JSON input/output.
func CallPlugin(plugin *extism.Plugin, funcName string, input []byte) ([]byte, error) {
    exitCode, output, err := plugin.Call(funcName, input)
    if err != nil {
        return nil, fmt.Errorf("wasmrt: call %s failed (exit %d): %w", funcName, exitCode, err)
    }
    return output, nil
}
```

### Passing Data In/Out (JSON Serialization)

All data exchange uses JSON-encoded byte buffers. This is the Extism convention:
plugins receive `[]byte` input and return `[]byte` output.

```go
// Host side -- calling a plugin function with structured data.
type EntityQuery struct {
    CampaignID string   `json:"campaign_id"`
    TypeSlug   string   `json:"type_slug,omitempty"`
    Tags       []string `json:"tags,omitempty"`
    Limit      int      `json:"limit,omitempty"`
}

type EntityResult struct {
    ID        string            `json:"id"`
    Name      string            `json:"name"`
    TypeSlug  string            `json:"type_slug"`
    Fields    map[string]any    `json:"fields"`
}

func QueryEntitiesViaPlugin(plugin *extism.Plugin, query EntityQuery) ([]EntityResult, error) {
    input, err := json.Marshal(query)
    if err != nil {
        return nil, err
    }

    output, err := CallPlugin(plugin, "on_entity_query", input)
    if err != nil {
        return nil, err
    }

    var results []EntityResult
    if err := json.Unmarshal(output, &results); err != nil {
        return nil, fmt.Errorf("wasmrt: invalid plugin response: %w", err)
    }
    return results, nil
}
```

### Defining and Exposing Host Functions

Host functions let plugins call back into Chronicle. They use wazero's stack-based
calling convention where data is passed as memory offsets (pointers).

```go
package wasmrt

import (
    "context"
    "encoding/json"
    "log/slog"

    extism "github.com/extism/go-sdk"
)

// ChronicleHostAPI holds the service interfaces that host functions can call.
// This is the bridge between WASM plugins and Chronicle's service layer.
type ChronicleHostAPI struct {
    EntityService  EntityReader      // read-only entity access
    TagService     TagReader         // read-only tag access
    CalendarService CalendarWriter   // calendar event creation
    Logger         *slog.Logger
}

// EntityReader is the subset of the entity service exposed to WASM plugins.
type EntityReader interface {
    GetByID(ctx context.Context, campaignID, entityID string) (*Entity, error)
    Search(ctx context.Context, campaignID, query string, limit int) ([]Entity, error)
    ListByType(ctx context.Context, campaignID, typeSlug string, limit, offset int) ([]Entity, error)
}

// BuildHostFunctions creates all host functions that Chronicle exposes to plugins.
func (api *ChronicleHostAPI) BuildHostFunctions() []extism.HostFunction {
    return []extism.HostFunction{
        api.hostGetEntity(),
        api.hostSearchEntities(),
        api.hostLog(),
    }
}

// hostGetEntity exposes chronicle.entity_get(entity_id) -> Entity JSON.
// Plugin calls this to read a single entity by ID.
func (api *ChronicleHostAPI) hostGetEntity() extism.HostFunction {
    return extism.NewHostFunctionWithStack(
        "chronicle_entity_get",
        func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
            // Read the request JSON from plugin memory.
            reqBytes, err := p.ReadBytes(stack[0])
            if err != nil {
                api.Logger.Error("host: failed to read entity_get input", "error", err)
                stack[0] = 0
                return
            }

            var req struct {
                CampaignID string `json:"campaign_id"`
                EntityID   string `json:"entity_id"`
            }
            if err := json.Unmarshal(reqBytes, &req); err != nil {
                api.Logger.Error("host: invalid entity_get request", "error", err)
                stack[0] = 0
                return
            }

            // Call Chronicle's entity service.
            entity, err := api.EntityService.GetByID(ctx, req.CampaignID, req.EntityID)
            if err != nil {
                api.Logger.Error("host: entity_get failed", "error", err)
                stack[0] = 0
                return
            }

            // Marshal result and write back to plugin memory.
            result, _ := json.Marshal(entity)
            offset, err := p.WriteBytes(result)
            if err != nil {
                api.Logger.Error("host: failed to write entity_get result", "error", err)
                stack[0] = 0
                return
            }
            stack[0] = offset
        },
        // Input: PTR to request JSON. Output: PTR to response JSON.
        []extism.ValueType{extism.ValueTypePTR},
        []extism.ValueType{extism.ValueTypePTR},
    )
}

// hostSearchEntities exposes chronicle.entity_search(query) -> []Entity JSON.
func (api *ChronicleHostAPI) hostSearchEntities() extism.HostFunction {
    return extism.NewHostFunctionWithStack(
        "chronicle_entity_search",
        func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
            reqBytes, err := p.ReadBytes(stack[0])
            if err != nil {
                stack[0] = 0
                return
            }

            var req struct {
                CampaignID string `json:"campaign_id"`
                Query      string `json:"query"`
                Limit      int    `json:"limit"`
            }
            if err := json.Unmarshal(reqBytes, &req); err != nil {
                stack[0] = 0
                return
            }
            if req.Limit <= 0 || req.Limit > 100 {
                req.Limit = 25
            }

            entities, err := api.EntityService.Search(ctx, req.CampaignID, req.Query, req.Limit)
            if err != nil {
                stack[0] = 0
                return
            }

            result, _ := json.Marshal(entities)
            offset, _ := p.WriteBytes(result)
            stack[0] = offset
        },
        []extism.ValueType{extism.ValueTypePTR},
        []extism.ValueType{extism.ValueTypePTR},
    )
}

// hostLog exposes chronicle.log(message) for plugin debugging.
func (api *ChronicleHostAPI) hostLog() extism.HostFunction {
    return extism.NewHostFunctionWithStack(
        "chronicle_log",
        func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
            msg, err := p.ReadString(stack[0])
            if err != nil {
                return
            }
            // Level is encoded as the second parameter: 0=debug, 1=info, 2=warn, 3=error
            level := int(stack[1])
            switch level {
            case 0:
                api.Logger.Debug("wasm-plugin", "msg", msg)
            case 1:
                api.Logger.Info("wasm-plugin", "msg", msg)
            case 2:
                api.Logger.Warn("wasm-plugin", "msg", msg)
            default:
                api.Logger.Error("wasm-plugin", "msg", msg)
            }
        },
        []extism.ValueType{extism.ValueTypePTR, extism.ValueTypeI32},
        []extism.ValueType{},
    )
}
```

### Error Handling and Timeouts

```go
// Timeout is set on the Manifest (in milliseconds).
manifest := extism.Manifest{
    Timeout: 5000, // 5 seconds -- kills plugin if it exceeds this
}

// When a plugin times out, plugin.Call returns an error.
// The plugin instance remains usable for subsequent calls.
_, _, err := plugin.Call("expensive_function", input)
if err != nil {
    // err message will indicate timeout if that was the cause.
    // Log it, return a graceful error to the user.
    slog.Error("plugin timed out", "plugin", pluginName, "error", err)
}
```

### Memory Limits and Resource Constraints

```go
manifest := extism.Manifest{
    Memory: struct {
        MaxPages uint32 `json:"max_pages,omitempty"`
    }{
        // Each page is 64 KB. 256 pages = 16 MB max linear memory.
        // This prevents memory bomb attacks from plugins.
        MaxPages: 256,
    },
    // Timeout in milliseconds. Prevents infinite loops.
    Timeout: 5000,
}
```

**Summary of configurable limits:**

| Limit | Location | Default Recommendation |
|-------|----------|----------------------|
| Memory max pages | `Manifest.Memory.MaxPages` | 256 (16 MB) |
| Execution timeout | `Manifest.Timeout` | 5000 ms |
| Allowed network hosts | `Manifest.AllowedHosts` | `[]` (none) |
| Allowed filesystem paths | `Manifest.AllowedPaths` | `{}` (none) |
| Plugin config (key-value) | `Manifest.Config` | Per-plugin settings |

---

## 2. Plugin Lifecycle

### Plugin Discovery and Registration

Chronicle should store WASM plugins on disk under a dedicated directory and track
their metadata in the database.

```
data/
  plugins/
    random-encounters/
      plugin.wasm          # Compiled WASM binary
      manifest.json        # Plugin metadata, permissions, hooks
    weather-generator/
      plugin.wasm
      manifest.json
```

**manifest.json format:**

```json
{
  "name": "Random Encounters",
  "slug": "random-encounters",
  "version": "1.0.0",
  "description": "Generates random encounters based on entity tags and location type",
  "author": "community",
  "hooks": ["on_entity_view", "on_calendar_advance"],
  "permissions": {
    "entity_read": true,
    "entity_write": false,
    "calendar_read": true,
    "calendar_write": true,
    "tag_read": true,
    "http": []
  },
  "config_schema": {
    "difficulty": {"type": "string", "default": "medium", "enum": ["easy", "medium", "hard"]},
    "encounter_table": {"type": "string", "default": ""}
  }
}
```

### Plugin Manager Service

```go
package wasmrt

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "sync"

    extism "github.com/extism/go-sdk"
    "github.com/tetratelabs/wazero"
)

// PluginManager handles loading, pooling, and lifecycle of WASM plugins.
type PluginManager struct {
    mu             sync.RWMutex
    pluginDir      string
    hostAPI        *ChronicleHostAPI
    compiledCache  map[string]*extism.CompiledPlugin  // slug -> compiled
    instancePools  map[string]chan *extism.Plugin       // slug -> pool of instances
    manifests      map[string]*PluginManifest          // slug -> manifest
    poolSize       int
    compilationCache wazero.CompilationCache
}

// NewPluginManager creates a manager that discovers and loads plugins.
func NewPluginManager(pluginDir string, hostAPI *ChronicleHostAPI, poolSize int) (*PluginManager, error) {
    cache := wazero.NewCompilationCache()
    return &PluginManager{
        pluginDir:        pluginDir,
        hostAPI:          hostAPI,
        compiledCache:    make(map[string]*extism.CompiledPlugin),
        instancePools:    make(map[string]chan *extism.Plugin),
        manifests:        make(map[string]*PluginManifest),
        poolSize:         poolSize,
        compilationCache: cache,
    }, nil
}

// DiscoverAndLoad scans the plugin directory and loads all valid plugins.
// Called at server startup.
func (pm *PluginManager) DiscoverAndLoad(ctx context.Context) error {
    entries, err := os.ReadDir(pm.pluginDir)
    if err != nil {
        return fmt.Errorf("wasmrt: failed to read plugin dir: %w", err)
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        slug := entry.Name()
        if err := pm.LoadPlugin(ctx, slug); err != nil {
            // Log but don't fail startup -- one bad plugin shouldn't break everything.
            slog.Error("failed to load plugin", "slug", slug, "error", err)
            continue
        }
        slog.Info("loaded WASM plugin", "slug", slug)
    }
    return nil
}

// LoadPlugin compiles a single plugin and pre-creates a pool of instances.
func (pm *PluginManager) LoadPlugin(ctx context.Context, slug string) error {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    wasmPath := filepath.Join(pm.pluginDir, slug, "plugin.wasm")
    manifestPath := filepath.Join(pm.pluginDir, slug, "manifest.json")

    // Parse manifest.
    manifest, err := parseManifest(manifestPath)
    if err != nil {
        return fmt.Errorf("invalid manifest for %s: %w", slug, err)
    }

    // Build host functions scoped to this plugin's permissions.
    hostFns := pm.hostAPI.BuildHostFunctionsForPermissions(manifest.Permissions)

    // Create Extism manifest with resource limits.
    extManifest := extism.Manifest{
        Wasm:         []extism.Wasm{extism.WasmFile{Path: wasmPath}},
        Timeout:      5000,
        Memory:       struct{ MaxPages uint32 `json:"max_pages,omitempty"` }{MaxPages: 256},
        AllowedHosts: manifest.Permissions.HTTP,
        Config:       manifest.DefaultConfig(),
    }

    config := extism.PluginConfig{
        EnableWasi:    true,
        RuntimeConfig: wazero.NewRuntimeConfig().WithCompilationCache(pm.compilationCache),
    }

    // Compile once -- this is the expensive step (AOT compilation).
    compiled, err := extism.NewCompiledPlugin(ctx, extManifest, config, hostFns)
    if err != nil {
        return fmt.Errorf("compilation failed for %s: %w", slug, err)
    }

    // Pre-create instance pool.
    pool := make(chan *extism.Plugin, pm.poolSize)
    for i := 0; i < pm.poolSize; i++ {
        instance, err := compiled.Instance(ctx, extism.PluginInstanceConfig{})
        if err != nil {
            return fmt.Errorf("failed to create instance %d for %s: %w", i, slug, err)
        }
        pool <- instance
    }

    pm.compiledCache[slug] = compiled
    pm.instancePools[slug] = pool
    pm.manifests[slug] = manifest
    return nil
}

// Acquire borrows a plugin instance from the pool. Returns it via Release.
func (pm *PluginManager) Acquire(ctx context.Context, slug string) (*extism.Plugin, error) {
    pm.mu.RLock()
    pool, ok := pm.instancePools[slug]
    compiled, hasCompiled := pm.compiledCache[slug]
    pm.mu.RUnlock()

    if !ok {
        return nil, fmt.Errorf("wasmrt: plugin %q not loaded", slug)
    }

    select {
    case instance := <-pool:
        return instance, nil
    default:
        // Pool exhausted -- create a new instance on the fly if we have
        // the compiled module. This handles burst traffic.
        if hasCompiled {
            return compiled.Instance(ctx, extism.PluginInstanceConfig{})
        }
        return nil, fmt.Errorf("wasmrt: no instances available for %q", slug)
    }
}

// Release returns a plugin instance to the pool.
func (pm *PluginManager) Release(slug string, instance *extism.Plugin) {
    pm.mu.RLock()
    pool, ok := pm.instancePools[slug]
    pm.mu.RUnlock()

    if !ok {
        return
    }

    select {
    case pool <- instance:
        // Returned to pool.
    default:
        // Pool is full (burst instance) -- close it.
        instance.Close(context.Background())
    }
}

// UnloadPlugin removes a plugin and closes all its instances.
func (pm *PluginManager) UnloadPlugin(ctx context.Context, slug string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    if pool, ok := pm.instancePools[slug]; ok {
        close(pool)
        for instance := range pool {
            instance.Close(ctx)
        }
        delete(pm.instancePools, slug)
    }
    delete(pm.compiledCache, slug)
    delete(pm.manifests, slug)
}

// ReloadPlugin performs a hot-reload: unload then load.
func (pm *PluginManager) ReloadPlugin(ctx context.Context, slug string) error {
    pm.UnloadPlugin(ctx, slug)
    return pm.LoadPlugin(ctx, slug)
}
```

### Hot-Reloading Without Server Restart

Yes, plugins can be hot-reloaded. The pattern:

1. **Unload** the old plugin (drain the pool, close instances).
2. **Load** the new .wasm from disk (re-compile, create new pool).
3. In-flight requests using the old instance finish naturally -- they hold a
   reference and will release to a now-closed pool (the release silently closes
   the instance).

This can be triggered by:
- An admin API endpoint: `POST /admin/plugins/{slug}/reload`
- A filesystem watcher (optional, for development)

### Plugin Crash and Timeout Handling

| Scenario | What Happens | Recovery |
|----------|-------------|----------|
| **Timeout** | `plugin.Call` returns error after `Timeout` ms | Instance remains usable; log and return error to caller |
| **Panic/trap** | WASM trap surfaces as a Go error from `plugin.Call` | Instance may be corrupted; discard it and create a new one from `CompiledPlugin` |
| **OOM** | Memory allocation beyond `MaxPages` fails inside WASM | Returns error; instance is still usable |
| **Host function panic** | Go panic in host function | Recovered by Echo's recovery middleware; instance is discarded |

```go
// Safe call wrapper with automatic instance recycling on crash.
func (pm *PluginManager) SafeCall(ctx context.Context, slug, funcName string, input []byte) ([]byte, error) {
    instance, err := pm.Acquire(ctx, slug)
    if err != nil {
        return nil, err
    }

    exitCode, output, err := instance.Call(funcName, input)
    if err != nil {
        // Don't return this instance to the pool -- it may be corrupted.
        instance.Close(ctx)
        return nil, fmt.Errorf("wasmrt: %s.%s failed (exit %d): %w", slug, funcName, exitCode, err)
    }

    // Instance is healthy, return to pool.
    pm.Release(slug, instance)
    return output, nil
}
```

---

## 3. Host Function Design for Chronicle

### Recommended Host Function API

Design principle: expose **narrow, read-heavy interfaces** behind service
abstractions. Plugins should not have direct access to repositories or SQL.

#### Entity Operations

| Host Function | Signature (JSON in/out) | Permission |
|---------------|------------------------|------------|
| `chronicle_entity_get` | `{campaign_id, entity_id}` -> `Entity` | `entity_read` |
| `chronicle_entity_search` | `{campaign_id, query, limit}` -> `[]Entity` | `entity_read` |
| `chronicle_entity_list_by_type` | `{campaign_id, type_slug, limit, offset}` -> `[]Entity` | `entity_read` |
| `chronicle_entity_get_fields` | `{campaign_id, entity_id}` -> `{fields_data}` | `entity_read` |
| `chronicle_entity_set_field` | `{campaign_id, entity_id, field_key, value}` -> `{ok}` | `entity_write` |

#### Tag Operations

| Host Function | Signature | Permission |
|---------------|-----------|------------|
| `chronicle_tag_list` | `{campaign_id}` -> `[]Tag` | `tag_read` |
| `chronicle_tag_get_entities` | `{campaign_id, tag_id, limit}` -> `[]Entity` | `tag_read` |

#### Relation Operations

| Host Function | Signature | Permission |
|---------------|-----------|------------|
| `chronicle_relation_list` | `{campaign_id, entity_id}` -> `[]Relation` | `entity_read` |

#### Calendar Operations

| Host Function | Signature | Permission |
|---------------|-----------|------------|
| `chronicle_calendar_get` | `{campaign_id}` -> `Calendar` | `calendar_read` |
| `chronicle_calendar_get_events` | `{campaign_id, year, month}` -> `[]Event` | `calendar_read` |
| `chronicle_calendar_create_event` | `{campaign_id, name, year, month, day, ...}` -> `{id}` | `calendar_write` |

#### Utility Operations

| Host Function | Signature | Permission |
|---------------|-----------|------------|
| `chronicle_log` | `{message, level}` -> void | always |
| `chronicle_config_get` | `{key}` -> `{value}` | always |

### Stable API Contract Design

To prevent breaking plugins when Chronicle internals change:

1. **Version the host API.** Each host function set gets a version number
   (`chronicle_v1_entity_get`). When breaking changes are needed, introduce `v2`
   functions and keep `v1` available for a deprecation cycle.

2. **Use DTOs, not domain models.** Host functions serialize purpose-built DTOs,
   not internal model structs. The DTO layer absorbs internal refactors.

```go
// wasmrt/dto.go -- stable DTOs for the WASM API boundary.

// WasmEntity is the stable representation of an entity for WASM plugins.
// This struct is versioned separately from the internal entity model.
type WasmEntity struct {
    ID       string            `json:"id"`
    Name     string            `json:"name"`
    TypeSlug string            `json:"type_slug"`
    TypeName string            `json:"type_name"`
    Fields   map[string]any    `json:"fields"`
    Tags     []WasmTag         `json:"tags"`
    ImageURL string            `json:"image_url,omitempty"`
}

type WasmTag struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Slug string `json:"slug"`
}
```

3. **Service interfaces, not concrete types.** Host functions depend on
   interfaces (`EntityReader`, `TagReader`). The concrete service implementations
   can change freely.

4. **Strict input validation.** Every host function validates its JSON input
   before calling any service. Malformed input returns a zero-value response, never
   a panic.

---

## 4. Security Model

### What WASM Plugins Cannot Do by Default

WebAssembly provides hardware-level sandboxing. By default, a Chronicle WASM
plugin:

- **Cannot access the filesystem** -- no file reads or writes unless `AllowedPaths` is explicitly set.
- **Cannot make network requests** -- no HTTP, DNS, or socket access unless `AllowedHosts` lists specific domains.
- **Cannot access host memory** -- WASM operates in its own linear memory space. It can only read/write data that the host explicitly copies in/out.
- **Cannot call arbitrary Go functions** -- only explicitly registered host functions are visible.
- **Cannot access environment variables, stdin/stdout, or system calls** beyond what WASI provides (and WASI is itself limited).
- **Cannot exceed memory limits** -- bounded by `MaxPages`.
- **Cannot run indefinitely** -- killed after `Timeout` ms.

### Capability-Based Access

Permissions are declared in the plugin's `manifest.json` and enforced at load time:

```go
// Only build host functions that this plugin is allowed to use.
func (api *ChronicleHostAPI) BuildHostFunctionsForPermissions(perms Permissions) []extism.HostFunction {
    var fns []extism.HostFunction

    // Always available.
    fns = append(fns, api.hostLog())
    fns = append(fns, api.hostConfigGet())

    if perms.EntityRead {
        fns = append(fns, api.hostGetEntity())
        fns = append(fns, api.hostSearchEntities())
        fns = append(fns, api.hostListEntitiesByType())
        fns = append(fns, api.hostGetEntityFields())
    }

    if perms.EntityWrite {
        fns = append(fns, api.hostSetEntityField())
    }

    if perms.TagRead {
        fns = append(fns, api.hostListTags())
        fns = append(fns, api.hostGetTagEntities())
    }

    if perms.CalendarRead {
        fns = append(fns, api.hostGetCalendar())
        fns = append(fns, api.hostGetCalendarEvents())
    }

    if perms.CalendarWrite {
        fns = append(fns, api.hostCreateCalendarEvent())
    }

    return fns
}
```

If a plugin tries to call a host function it wasn't granted, the WASM runtime
will return a link-time error (the import won't be satisfied). This is enforced
at the WebAssembly level -- there's no way to bypass it from inside the sandbox.

### Permission Levels

| Level | Entity Read | Entity Write | Calendar Read | Calendar Write | HTTP | Use Case |
|-------|------------|-------------|---------------|----------------|------|----------|
| **read-only** | Yes | No | Yes | No | No | Data viewers, exporters |
| **standard** | Yes | Yes | Yes | Yes | No | Content generators, automation |
| **network** | Yes | Yes | Yes | Yes | Allowed hosts | External API integrations |

### Denial-of-Service Prevention

| Attack Vector | Mitigation |
|---------------|-----------|
| Infinite loop | `Manifest.Timeout` (5000 ms default) kills execution |
| Memory bomb | `Manifest.Memory.MaxPages` (256 pages = 16 MB default) |
| CPU exhaustion | Timeout + pool sizing limits concurrent plugin executions |
| Host function abuse | Rate limiting inside host functions; result set caps (`Limit` capped at 100) |
| Stack overflow | WASM has a fixed call stack; wazero enforces it |

### Campaign-Scoped Isolation

Every host function receives a `campaign_id` in its request. The host function
implementation MUST verify that the calling context (the campaign that activated
this plugin) matches the requested `campaign_id`. A plugin activated for campaign
A should never be able to read data from campaign B.

```go
// Enforced inside every host function.
if req.CampaignID != pluginContext.CampaignID {
    // Return empty result, log the violation.
    stack[0] = 0
    return
}
```

---

## 5. Plugin Development Experience

### Supported Languages

Extism provides PDKs (Plugin Development Kits) for:

| Language | PDK | Binary Size | Build Target | Notes |
|----------|-----|-------------|-------------|-------|
| **Rust** | [extism/rust-pdk](https://github.com/extism/rust-pdk) | ~100 KB | `wasm32-unknown-unknown` | Best performance, smallest binaries |
| **Go (TinyGo)** | [extism/go-pdk](https://github.com/extism/go-pdk) | ~500 KB | `wasip1` via TinyGo | Familiar to Chronicle contributors |
| **Go (std)** | [extism/go-pdk](https://github.com/extism/go-pdk) | ~2-5 MB | `GOOS=wasip1 GOARCH=wasm` | Larger but no TinyGo dependency |
| **JavaScript** | [extism/js-pdk](https://github.com/extism/js-pdk) | ~1-3 MB | QuickJS-ng embedded | Lowest barrier to entry |
| **TypeScript** | [extism/js-pdk](https://github.com/extism/js-pdk) | ~1-3 MB | Via extism-js compiler | Type safety for JS devs |
| **C#** | [extism/dotnet-pdk](https://github.com/extism/dotnet-pdk) | ~2 MB | NativeAOT-LLVM | |
| **Zig** | [extism/zig-pdk](https://github.com/extism/zig-pdk) | ~50 KB | Native WASM target | Very small binaries |

### Minimal Plugin Examples

#### Rust Plugin

```rust
// Cargo.toml
// [lib]
// crate-type = ["cdylib"]
// [dependencies]
// extism-pdk = "1.3"
// serde = { version = "1", features = ["derive"] }
// serde_json = "1"

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
struct EntityViewEvent {
    entity_id: String,
    entity_name: String,
    type_slug: String,
    campaign_id: String,
}

#[derive(Serialize)]
struct PluginResponse {
    sidebar_html: String,
    notifications: Vec<String>,
}

// Host function imports -- these are provided by Chronicle.
#[host_fn]
extern "ExtismHost" {
    fn chronicle_entity_search(req: Json<SearchReq>) -> Json<Vec<Entity>>;
    fn chronicle_log(msg: String, level: i32);
}

#[plugin_fn]
pub fn on_entity_view(input: String) -> FnResult<String> {
    let event: EntityViewEvent = serde_json::from_str(&input)?;

    // Call back into Chronicle to find related entities.
    let search = SearchReq {
        campaign_id: event.campaign_id.clone(),
        query: event.entity_name.clone(),
        limit: 5,
    };
    let Json(related) = unsafe { chronicle_entity_search(Json(search))? };

    let html = format!(
        "<div class='plugin-sidebar'><h3>Related</h3><ul>{}</ul></div>",
        related.iter()
            .filter(|e| e.id != event.entity_id)
            .map(|e| format!("<li>{}</li>", e.name))
            .collect::<Vec<_>>()
            .join("")
    );

    let response = PluginResponse {
        sidebar_html: html,
        notifications: vec![],
    };

    Ok(serde_json::to_string(&response)?)
}
```

Build: `cargo build --target wasm32-unknown-unknown --release`

#### Go (TinyGo) Plugin

```go
package main

import (
    "encoding/json"
    "github.com/extism/go-pdk"
)

// EntityViewEvent is sent by Chronicle when an entity page is viewed.
type EntityViewEvent struct {
    EntityID   string `json:"entity_id"`
    EntityName string `json:"entity_name"`
    TypeSlug   string `json:"type_slug"`
    CampaignID string `json:"campaign_id"`
}

type PluginResponse struct {
    SidebarHTML   string   `json:"sidebar_html"`
    Notifications []string `json:"notifications"`
}

//go:wasmimport extism:host/user chronicle_log
func _chronicle_log(offset uint64, level uint32)

func chronicleLog(msg string, level int) {
    mem := pdk.AllocateString(msg)
    _chronicle_log(mem.Offset(), uint32(level))
}

//go:wasmexport on_entity_view
func onEntityView() {
    input := pdk.Input()

    var event EntityViewEvent
    if err := json.Unmarshal(input, &event); err != nil {
        pdk.SetError(err)
        return
    }

    chronicleLog("Processing entity view: "+event.EntityName, 1)

    response := PluginResponse{
        SidebarHTML:   "<div>Hello from Go plugin!</div>",
        Notifications: []string{},
    }

    out, _ := json.Marshal(response)
    pdk.Output(out)
}

func main() {}
```

Build: `tinygo build -target wasip1 -o plugin.wasm main.go`

#### JavaScript Plugin

```javascript
// plugin.js

function on_entity_view() {
    const input = JSON.parse(Host.inputString());

    // Call host function to search related entities.
    const searchReq = JSON.stringify({
        campaign_id: input.campaign_id,
        query: input.entity_name,
        limit: 5
    });

    const related = JSON.parse(
        Host.getFunctions()["chronicle_entity_search"](searchReq)
    );

    const items = related
        .filter(e => e.id !== input.entity_id)
        .map(e => `<li>${e.name}</li>`)
        .join('');

    const response = {
        sidebar_html: `<div class="plugin-sidebar"><h3>Related</h3><ul>${items}</ul></div>`,
        notifications: []
    };

    Host.outputString(JSON.stringify(response));
}

module.exports = { on_entity_view };
```

Build: `extism-js plugin.js -o plugin.wasm`

### Local Testing

Chronicle should provide a CLI tool or test harness:

```bash
# Test a plugin locally with mock data.
chronicle-plugin test ./random-encounters/plugin.wasm \
    --function on_entity_view \
    --input '{"entity_id":"abc","entity_name":"Goblin Cave","type_slug":"location","campaign_id":"xyz"}'

# Validate a manifest.
chronicle-plugin validate ./random-encounters/manifest.json

# Run with the Extism CLI for quick smoke tests.
extism call ./plugin.wasm on_entity_view \
    --input '{"entity_id":"abc","entity_name":"Test","type_slug":"npc","campaign_id":"xyz"}'
```

### SDK/Tooling Chronicle Should Provide

1. **chronicle-plugin-sdk** -- A small library (Rust crate, Go module, npm
   package) that provides type definitions for all Chronicle host function
   request/response types. Auto-generated from the host function API spec.

2. **chronicle-plugin CLI** -- A development tool that:
   - Scaffolds new plugin projects (`chronicle-plugin init --lang rust`)
   - Validates manifest files
   - Runs plugins against mock Chronicle data
   - Packages plugins for distribution (creates the directory structure)

3. **Type definition files** -- JSON Schema or OpenAPI-style definitions of all
   host function contracts, so plugin developers know the exact shapes.

---

## 6. Performance Characteristics

### WASM Call Overhead vs Native Go

Based on wazero benchmarks and production reports:

| Operation | Native Go | WASM (wazero compiler) | Overhead |
|-----------|-----------|----------------------|----------|
| Simple function call | ~1 ns | ~100-500 ns | ~100-500x |
| JSON parse + business logic (1 KB) | ~1-5 us | ~5-20 us | ~3-5x |
| Complex computation (sorting, filtering) | baseline | 2-5x baseline | 2-5x |
| I/O-bound (host function calls) | baseline | ~baseline + marshaling | ~1.5-2x |

**Key insight:** For Chronicle's use case (plugins that mostly call host functions
to read/write data), the overhead is dominated by JSON serialization and database
I/O, not WASM execution speed. The WASM overhead is negligible compared to a
database round-trip (~1 ms).

### wazero AOT Compilation

wazero's compiler mode (the default) performs AOT compilation:

- **Compilation cost:** Several hundred milliseconds to a few seconds per module,
  depending on size. Paid once at startup or plugin load.
- **Compilation cache:** Use `wazero.NewCompilationCache()` to share compiled
  code across plugin instances and even persist it to disk across restarts.
- **Runtime performance:** AOT-compiled modules run 10x faster than the
  interpreter. The optimizing compiler (introduced in wazero 1.7+) adds another
  30-60% improvement.

```go
// Share compilation cache across all plugins for faster startup.
cache := wazero.NewCompilationCache()
defer cache.Close(ctx)

config := extism.PluginConfig{
    RuntimeConfig: wazero.NewRuntimeConfig().WithCompilationCache(cache),
}
```

### Memory Overhead per Plugin Instance

| Component | Approximate Size |
|-----------|-----------------|
| Compiled module (shared) | 1-10 MB depending on plugin size |
| Instance linear memory (initial) | 1-4 pages = 64-256 KB |
| Instance linear memory (max) | Up to `MaxPages * 64 KB` (16 MB at 256 pages) |
| wazero runtime metadata per instance | ~10-50 KB |
| Go-side bookkeeping | ~1-2 KB |

For a typical Chronicle deployment with 5-10 plugins, each with a pool of 4
instances: **~20-100 MB total** depending on plugin complexity. This is very
manageable.

### Instance Pooling and Reuse

Plugin instances ARE reusable across calls. Extism resets the plugin's memory
before each `Call` invocation. The recommended pattern:

```
Startup:
  1. Compile each plugin once (AOT) -> CompiledPlugin
  2. Pre-create N instances per plugin -> instance pool

Per-request:
  1. Acquire instance from pool (non-blocking)
  2. Call function (memory auto-reset by Extism)
  3. Release instance back to pool

Pool exhaustion:
  Create a new instance from CompiledPlugin (fast, ~1-5 ms)
  Return to pool or discard after use depending on pool capacity
```

### Recommended Pool Sizing

| Deployment Size | Concurrent Users | Pool Size per Plugin |
|-----------------|-----------------|---------------------|
| Small (self-hosted) | 1-5 | 2 |
| Medium | 5-20 | 4 |
| Large | 20-100 | 8-16 |

Since Chronicle is self-hosted and typically serves a small group, a pool of 2-4
instances per plugin is sufficient.

---

## 7. Real-World Examples & Lessons

### Navidrome (Go Music Server + Extism Plugins)

**Project:** [navidrome/navidrome](https://github.com/navidrome/navidrome)

Navidrome is a Go-based music streaming server that recently added an Extism-based
plugin system. Their design is the closest analogue to what Chronicle needs.

**Architecture decisions:**
- Plugins are distributed as `.ndp` files (zip archives) containing `manifest.json`
  and `plugin.wasm`.
- Host services (HTTP, Scheduler, Cache, KVStore, WebSocket, Library access) are
  exposed through host functions.
- Permissions are declared in the manifest and enforced at plugin load time.
- Plugin capabilities are auto-detected by inspecting exported function names
  (prefixed with `nd_`).
- Each plugin has isolated cache and KVStore namespaces.

**Lessons applicable to Chronicle:**
- The manifest-based permission model works well and is easy to audit.
- Auto-detecting capabilities from exports keeps the plugin API flexible.
- Providing a persistent KV store as a host function is valuable -- plugins need
  somewhere to store state.

### Moonrepo proto (Rust CLI + Extism Plugins)

**Project:** [moonrepo/proto](https://github.com/moonrepo/proto)

Proto is a toolchain version manager that uses Extism for its plugin system.

**Key insight from their experience:** "Extism is an amazing product that has
accelerated our development and iteration speeds. Without Extism, a lot of time
and resources would be spent understanding the intricacies of the WASM ecosystem."

**Lessons:**
- They found that the Extism abstraction layer saved significant development time
  compared to using wazero directly.
- Third-party plugin contributions grew rapidly once the plugin interface was
  documented.
- Rust PDK plugins produce the smallest and fastest binaries.

### Arcjet (Go API Security + wazero in Production)

**Project:** [Arcjet](https://blog.arcjet.com/lessons-from-running-webassembly-in-production-with-go-wazero/)

Arcjet runs wazero in production for their API security product, processing every
API request through WASM rules.

**Performance findings:**
- p50 latency: 10 ms, p99 target: 30 ms -- achievable with wazero.
- Pre-compilation with Wizer eliminates runtime compilation overhead.
- `wasm-opt` optimization passes reduce binary size and improve startup.
- Standard profiling tools cannot see inside WASM execution -- this is a known
  observability gap.

**Lessons for Chronicle:**
- Pre-compile WASM modules on startup, not on first request.
- Use `wasm-opt` in the plugin build pipeline to optimize binaries.
- Plan for limited WASM observability -- rely on host function logging.

### Lemmy (Rust/Actix + Extism Plugin Hooks)

**Project:** [LemmyNet/lemmy](https://github.com/LemmyNet/lemmy)

The Fediverse platform Lemmy added plugin hooks for content lifecycle events
(before/after post creation). Their approach uses event hooks rather than
service-level integration.

**Pattern applicable to Chronicle:**
- Define clear hook points: `on_entity_create`, `on_entity_view`,
  `on_calendar_advance`, `on_session_start`, etc.
- Plugins return structured responses (modifications, notifications, side effects)
  rather than making changes directly.
- This event-driven model fits naturally with Chronicle's request flow.

### Common Problems Encountered Across Projects

| Problem | Solution |
|---------|---------|
| Plugin compilation is slow on first load | Use `CompiledPlugin` + compilation cache; pre-compile at startup |
| Debugging WASM plugins is hard | Provide a `chronicle_log` host function; support `extism call` CLI testing |
| JSON serialization overhead | Acceptable for Chronicle's use case; consider MessagePack if it becomes a bottleneck |
| Plugin API versioning | Prefix host functions with version (`chronicle_v1_*`); maintain backward compatibility |
| Memory leaks in long-running instances | Extism resets memory between calls; pool instances and rotate periodically |
| Plugin installation UX | Provide a CLI tool and admin UI for installing/managing plugins |

---

## Integration with Chronicle's Architecture

### Where the WASM Runtime Fits

```
cmd/server/main.go
  -> internal/app/app.go
    -> internal/wasmrt/               # NEW: WASM runtime package
    |    ├── manager.go               # PluginManager (load/pool/lifecycle)
    |    ├── hostapi.go               # ChronicleHostAPI + host function defs
    |    ├── dto.go                   # Stable DTOs for WASM boundary
    |    ├── manifest.go              # Manifest parsing + validation
    |    ├── hooks.go                 # Hook point definitions + dispatch
    |    └── manager_test.go
    -> internal/plugins/*/handler.go  # Handlers call hook dispatch at key points
```

### Hook Dispatch Pattern

```go
// internal/wasmrt/hooks.go

// HookPoint identifies where in the request lifecycle a hook fires.
type HookPoint string

const (
    HookEntityView       HookPoint = "on_entity_view"
    HookEntityCreate     HookPoint = "on_entity_create"
    HookEntityUpdate     HookPoint = "on_entity_update"
    HookCalendarAdvance  HookPoint = "on_calendar_advance"
    HookSessionStart     HookPoint = "on_session_start"
)

// DispatchHook calls all plugins that registered for a given hook point.
func (pm *PluginManager) DispatchHook(ctx context.Context, hook HookPoint, input []byte) ([]HookResult, error) {
    pm.mu.RLock()
    var slugs []string
    for slug, manifest := range pm.manifests {
        for _, h := range manifest.Hooks {
            if HookPoint(h) == hook {
                slugs = append(slugs, slug)
                break
            }
        }
    }
    pm.mu.RUnlock()

    var results []HookResult
    for _, slug := range slugs {
        output, err := pm.SafeCall(ctx, slug, string(hook), input)
        if err != nil {
            slog.Error("hook dispatch failed", "hook", hook, "plugin", slug, "error", err)
            continue // Don't let one plugin break others.
        }
        var result HookResult
        if err := json.Unmarshal(output, &result); err != nil {
            slog.Error("invalid hook response", "hook", hook, "plugin", slug, "error", err)
            continue
        }
        results = append(results, result)
    }
    return results, nil
}

// HookResult is the standardized response from a plugin hook.
type HookResult struct {
    PluginSlug    string   `json:"plugin_slug"`
    SidebarHTML   string   `json:"sidebar_html,omitempty"`
    Notifications []string `json:"notifications,omitempty"`
    ModifiedData  any      `json:"modified_data,omitempty"`
}
```

### Integration in Entity Handler (example)

```go
// internal/plugins/entities/handler.go

func (h *Handler) ShowEntity(c echo.Context) error {
    // ... existing entity fetch logic ...
    entity, err := h.service.GetByID(ctx, campaignID, entityID)

    // Dispatch WASM hook for entity view.
    hookInput, _ := json.Marshal(map[string]string{
        "entity_id":   entity.ID,
        "entity_name": entity.Name,
        "type_slug":   entity.TypeSlug,
        "campaign_id": campaignID,
    })
    hookResults, _ := h.wasmManager.DispatchHook(ctx, wasmrt.HookEntityView, hookInput)

    // Pass hook results to template for rendering plugin sidebar panels.
    return render(c, templates.EntityShow(entity, hookResults))
}
```

---

## Recommended Implementation Order

1. **Phase 1:** Core runtime (`internal/wasmrt/`) -- manager, host API with
   `chronicle_log` + `chronicle_entity_get` only, manifest parsing.
2. **Phase 2:** Hook dispatch system + integration with entity handler.
3. **Phase 3:** Remaining host functions (search, tags, calendar).
4. **Phase 4:** Admin UI for plugin management (install, enable/disable, reload).
5. **Phase 5:** Plugin SDK packages (Rust crate, Go module) + documentation.
6. **Phase 6:** chronicle-plugin CLI tool.

---

## Sources

- [Extism Go SDK - GitHub](https://github.com/extism/go-sdk)
- [Extism Go SDK - API Reference](https://pkg.go.dev/github.com/extism/go-sdk)
- [Extism Go PDK - GitHub](https://github.com/extism/go-pdk)
- [Extism Host Functions Docs](https://extism.org/docs/concepts/host-functions/)
- [Extism Plugin Quickstart](https://extism.org/docs/quickstart/plugin-quickstart/)
- [Extism Host Quickstart](https://extism.org/docs/quickstart/host-quickstart/)
- [Extism Rust PDK](https://github.com/extism/rust-pdk)
- [Extism JS PDK](https://github.com/extism/js-pdk)
- [wazero Documentation](https://wazero.io/docs/)
- [wazero GitHub](https://github.com/tetratelabs/wazero)
- [Writing Host Functions in Go with Extism (k33g)](https://k33g.hashnode.dev/writing-host-functions-in-go-with-extism)
- [Lessons from Running WebAssembly in Production with Go & Wazero (Arcjet)](https://blog.arcjet.com/lessons-from-running-webassembly-in-production-with-go-wazero/)
- [Navidrome Plugin System PR](https://github.com/navidrome/navidrome/pull/4833)
- [Navidrome Plugin README](https://github.com/navidrome/navidrome/blob/master/plugins/README.md)
- [DX Spotlight: proto by moonrepo (Dylibso)](https://dylibso.com/blog/proto-extism/)
- [Projects Using Extism in the Wild - Discussion #684](https://github.com/extism/extism/discussions/684)
- [Extism Concurrency Discussion - Issue #791](https://github.com/extism/extism/issues/791)
- [Extism Memory Model](https://extism.org/docs/concepts/memory/)
- [Extism FAQs](https://extism.org/docs/questions/)
- [Sandboxing LLM Generated Code with Extism](https://extism.org/blog/sandboxing-llm-generated-code/)
- [WebAssembly Runtime Performance Benchmarks 2023](https://00f.net/2023/01/04/webassembly-benchmark-2023/)

# Build a WASM plugin

A WASM plugin is server-side logic, compiled to WebAssembly, that ships inside
a [content extension](content-extension.md). Chronicle runs it with Extism (on
wazero) in a sandbox: it sees only the host functions its declared
capabilities unlock. A widget or script calls its exported functions over HTTP.

## Files

```
hello-wasm/
  manifest.json
  dist/hello.wasm    # build output; ship it in the zip
  main.go, go.mod    # source (optional in the zip)
```

## Manifest

```json
{
  "manifest_version": 1,
  "id": "hello-wasm",
  "name": "Hello WASM",
  "version": "1.0.0",
  "description": "Says hello.",
  "contributes": {
    "wasm_plugins": [
      { "slug": "hello", "name": "Hello", "file": "dist/hello.wasm",
        "capabilities": ["log"] }
    ]
  }
}
```

- Required per plugin: `slug`, `name`, `file` (`.wasm`), at least one capability.
- Optional: `hooks`, `memory_limit_mb` (default 16, max 256), `timeout_secs`
  (default 30, max 300), `config` (declared only; not passed to the plugin).

## Minimal plugin (Go)

```go
package main

import "github.com/extism/go-pdk"

//go:wasmimport extism:host/user chronicle_log
func chronicleLog(offset uint64) // no result

type greetInput struct {
	Name string `json:"name"`
}

//go:wasmexport greet
func greet() int32 {
	var in greetInput
	if err := pdk.InputJSON(&in); err != nil {
		pdk.SetError(err)
		return 1
	}
	msg := pdk.AllocateString("greeting " + in.Name)
	chronicleLog(msg.Offset())
	_ = pdk.OutputJSON(map[string]string{"message": "Hello, " + in.Name})
	return 0
}

func main() {}
```

`go.mod` needs `require github.com/extism/go-pdk v1.1.3`. Build:

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o dist/hello.wasm .
# or: tinygo build -o dist/hello.wasm -target wasip1 .
```

Without `-buildmode=c-shared` the plugin loads but every call fails with
`runtime.notInitialized`. This example was built with Go 1.27 and called
through Chronicle's plugin manager. The repo's `extensions/example-wasm-go`
does not load as shipped (#1325).

Every other host function takes a pointer to JSON and returns a pointer to
JSON (`func getEntity(offset uint64) uint64`); read the result with
`pdk.FindMemory(ptr).ReadBytes()`.

## Capabilities

| Capability | Host functions |
|---|---|
| `log` | `chronicle_log` |
| `entity_read` | `get_entity`, `search_entities`, `list_entity_types` |
| `entity_write` | `update_entity_fields` |
| `tag_read` | `list_tags` |
| `tag_write` | `set_entity_tags`, `get_entity_tags` |
| `relation_write` | `create_relation` |
| `kv_store` | `kv_get`, `kv_set`, `kv_delete` (scoped to the campaign and extension) |
| `message` | `send_message` (calls another plugin's `on_message`) |
| `calendar_read`, `calendar_write` | `get_calendar`, `list_events`, `create_event`: always error until the calendar rebuild lands (#778) |

Input and output shapes per function: [`.ai/plugin-development.md`](../../.ai/plugin-development.md).

## Hooks

List event names in `hooks` and export `on_hook`; it receives
`{"type", "campaign_id", "payload", "timestamp"}`. Today only
`campaign.deleted` is ever sent; `entity.*` and `tag.*` are accepted but not
dispatched yet (#1325).

## Install, call and test

1. Install the extension zip and turn it on in a campaign
   ([content-extension.md](content-extension.md)). Turning it on loads the plugin.
2. Call it as a signed-in Scribe or higher (session cookie + CSRF, so from a
   widget with `Chronicle.apiFetch`):

   ```
   POST /campaigns/:id/extensions/wasm/hello-wasm/hello/call
   {"function": "greet", "input": {"name": "Ada"}}
   → {"output": {"message": "Hello, Ada"}}
   ```

   A plugin error comes back as `200 {"error": "…"}`; a plugin that isn't
   loaded is a 500, and a pack the campaign hasn't turned on is a 403.
3. Admin view: `GET /admin/extensions/wasm/plugins` lists loaded plugins.

After a server restart a plugin stays unloaded until the owner turns the pack
off and on again (#1325).

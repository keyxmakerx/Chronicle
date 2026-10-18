package extensions

import (
	"context"
	"sync"
	"testing"

	extism "github.com/extism/go-sdk"
)

// noopWasm is a hand-assembled module exporting memory and "f" (returns 0),
// enough for Extism to call without shipping a binary fixture.
var noopWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00,
	0x05, 0x03, 0x01, 0x00, 0x01,
	0x07, 0x0e, 0x02, 0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00, 0x01, 'f', 0x00, 0x00,
	0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x00, 0x0b,
}

// TestPluginManagerCallConcurrent pins that concurrent calls into one plugin
// are serialized; an Extism plugin is not thread-safe, so under -race an
// unlocked Call is reported.
func TestPluginManagerCallConcurrent(t *testing.T) {
	ctx := context.Background()
	p, err := extism.NewPlugin(ctx, extism.Manifest{Wasm: []extism.Wasm{extism.WasmData{Data: noopWasm}}}, extism.PluginConfig{}, nil)
	if err != nil {
		t.Fatalf("NewPlugin: %v", err)
	}
	defer func() { _ = p.Close(ctx) }()

	pm := NewPluginManager(t.TempDir(), NewHostEnvironment(nil, nil, nil, nil))
	pm.limits = WASMLimits{TimeoutMS: 5000}
	pm.plugins[pluginKey("ext", "p")] = &loadedPlugin{plugin: p}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := pm.Call(ctx, "ext", "p", "f", nil)
			if err != nil {
				t.Errorf("Call: %v", err)
				return
			}
			if resp.Error != "" {
				t.Errorf("call error: %s", resp.Error)
			}
		}()
	}
	wg.Wait()
}

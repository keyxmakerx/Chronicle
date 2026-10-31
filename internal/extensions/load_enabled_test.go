package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// loadRepo is an ExtensionRepository that answers only what
// LoadEnabledPlugins asks; any other call panics on the nil interface.
type loadRepo struct {
	ExtensionRepository
	exts  []Extension
	using map[string]int
}

func (r *loadRepo) List(context.Context) ([]Extension, error) { return r.exts, nil }
func (r *loadRepo) CountCampaignsUsing(_ context.Context, id string) (int, error) {
	return r.using[id], nil
}

// recordLoader records each plugin it was asked to load; slugs in fail error.
type recordLoader struct {
	loaded []string
	fail   map[string]bool
}

func (l *recordLoader) Load(_ context.Context, extID string, c WASMContribution) error {
	if l.fail[c.Slug] {
		return errors.New("broken")
	}
	l.loaded = append(l.loaded, extID+":"+c.Slug)
	return nil
}

func wasmManifest(slugs ...string) json.RawMessage {
	var plugins []WASMContribution
	for _, s := range slugs {
		plugins = append(plugins, WASMContribution{Slug: s, File: s + ".wasm"})
	}
	b, _ := json.Marshal(ExtensionManifest{Contributes: &ManifestContributes{WASMPlugins: plugins}})
	return b
}

func TestLoadEnabledPlugins(t *testing.T) {
	repo := &loadRepo{
		exts: []Extension{
			{ID: "1", ExtID: "used", Status: StatusActive, Manifest: wasmManifest("a", "b")},
			{ID: "2", ExtID: "unused", Status: StatusActive, Manifest: wasmManifest("c")},
			{ID: "3", ExtID: "off", Status: "disabled", Manifest: wasmManifest("d")},
			{ID: "4", ExtID: "content-only", Status: StatusActive, Manifest: json.RawMessage(`{}`)},
			{ID: "5", ExtID: "half-broken", Status: StatusActive, Manifest: wasmManifest("bad", "good")},
		},
		using: map[string]int{"1": 2, "3": 1, "4": 1, "5": 1},
	}
	loader := &recordLoader{fail: map[string]bool{"bad": true}}
	svc := NewExtensionService(repo, t.TempDir())

	n := svc.LoadEnabledPlugins(context.Background(), loader)

	want := []string{"used:a", "used:b", "half-broken:good"}
	if n != len(want) {
		t.Errorf("loaded count = %d, want %d", n, len(want))
	}
	if len(loader.loaded) != len(want) {
		t.Fatalf("loaded %v, want %v", loader.loaded, want)
	}
	for i := range want {
		if loader.loaded[i] != want[i] {
			t.Errorf("loaded[%d] = %q, want %q", i, loader.loaded[i], want[i])
		}
	}
}

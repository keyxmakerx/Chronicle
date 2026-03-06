package modules

import "fmt"

// GenericModule is a Module implementation that works for any game system
// without custom Go code. It loads data from JSON files and uses the
// manifest's field definitions for tooltip rendering. Drop a manifest.json
// + data/*.json files into a module directory and it Just Works.
type GenericModule struct {
	manifest *ModuleManifest
	provider *JSONProvider
	renderer *GenericTooltipRenderer
}

// NewGenericModule creates a module from its manifest and data directory
// using the generic JSON provider and manifest-driven tooltip renderer.
func NewGenericModule(manifest *ModuleManifest, dataDir string) (*GenericModule, error) {
	provider, err := NewJSONProvider(manifest.ID, dataDir)
	if err != nil {
		return nil, fmt.Errorf("generic module %s: loading data: %w", manifest.ID, err)
	}

	return &GenericModule{
		manifest: manifest,
		provider: provider,
		renderer: NewGenericTooltipRenderer(manifest),
	}, nil
}

// Info returns the module's manifest metadata.
func (m *GenericModule) Info() *ModuleManifest {
	return m.manifest
}

// DataProvider returns the JSON-file data provider.
func (m *GenericModule) DataProvider() DataProvider {
	return m.provider
}

// TooltipRenderer returns the manifest-driven generic tooltip renderer.
func (m *GenericModule) TooltipRenderer() TooltipRenderer {
	return m.renderer
}

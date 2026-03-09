package systems

import "fmt"

// GenericSystem is a System implementation that works for any game system
// without custom Go code. It loads data from JSON files and uses the
// manifest's field definitions for tooltip rendering. Drop a manifest.json
// + data/*.json files into a module directory and it Just Works.
type GenericSystem struct {
	manifest *SystemManifest
	provider *JSONProvider
	renderer *GenericTooltipRenderer
}

// NewGenericSystem creates a module from its manifest and data directory
// using the generic JSON provider and manifest-driven tooltip renderer.
func NewGenericSystem(manifest *SystemManifest, dataDir string) (*GenericSystem, error) {
	provider, err := NewJSONProvider(manifest.ID, dataDir)
	if err != nil {
		return nil, fmt.Errorf("generic module %s: loading data: %w", manifest.ID, err)
	}

	return &GenericSystem{
		manifest: manifest,
		provider: provider,
		renderer: NewGenericTooltipRenderer(manifest),
	}, nil
}

// Info returns the module's manifest metadata.
func (m *GenericSystem) Info() *SystemManifest {
	return m.manifest
}

// DataProvider returns the JSON-file data provider.
func (m *GenericSystem) DataProvider() DataProvider {
	return m.provider
}

// TooltipRenderer returns the manifest-driven generic tooltip renderer.
func (m *GenericSystem) TooltipRenderer() TooltipRenderer {
	return m.renderer
}

package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// JSONProvider loads reference data from JSON files in a module's data
// directory. Each file corresponds to one category (e.g., spells.json →
// "spells" category). Files contain arrays of ReferenceItem objects.
// Data is loaded into memory at construction time and served read-only.
type JSONProvider struct {
	moduleID string
	dataDir  string
	items    map[string][]ReferenceItem
}

// NewJSONProvider scans dataDir for *.json files, loading each as a
// category of ReferenceItem objects. The filename stem becomes the
// category slug (e.g., "spells.json" → "spells"). Returns an error
// if the directory cannot be read or a JSON file is malformed.
func NewJSONProvider(moduleID, dataDir string) (*JSONProvider, error) {
	p := &JSONProvider{
		moduleID: moduleID,
		dataDir:  dataDir,
		items:    make(map[string][]ReferenceItem),
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("reading data dir %s: %w", dataDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		category := strings.TrimSuffix(entry.Name(), ".json")
		filePath := filepath.Join(dataDir, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, err)
		}

		var items []ReferenceItem
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, err)
		}

		// Stamp each item with the module ID and category.
		for i := range items {
			items[i].ModuleID = moduleID
			items[i].Category = category
		}

		p.items[category] = items
	}

	return p, nil
}

// List returns all reference items in the given category.
// Returns an empty slice if the category does not exist.
func (p *JSONProvider) List(category string) ([]ReferenceItem, error) {
	items, ok := p.items[category]
	if !ok {
		return []ReferenceItem{}, nil
	}
	return items, nil
}

// Get returns a single reference item by category and ID (slug).
// Returns nil and no error if the item does not exist.
func (p *JSONProvider) Get(category string, id string) (*ReferenceItem, error) {
	items, ok := p.items[category]
	if !ok {
		return nil, nil
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, nil
}

// Search returns reference items matching the query string across all
// categories. Matches case-insensitively against Name, Summary, and Tags.
func (p *JSONProvider) Search(query string) ([]ReferenceItem, error) {
	if query == "" {
		return []ReferenceItem{}, nil
	}

	q := strings.ToLower(query)
	var results []ReferenceItem

	for _, items := range p.items {
		for _, item := range items {
			if matchesQuery(item, q) {
				results = append(results, item)
			}
		}
	}

	return results, nil
}

// Categories returns the list of available data category slugs, sorted.
func (p *JSONProvider) Categories() []string {
	cats := make([]string, 0, len(p.items))
	for k := range p.items {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	return cats
}

// matchesQuery checks if a reference item matches the query (lowercase)
// against name, summary, or any tag.
func matchesQuery(item ReferenceItem, query string) bool {
	if strings.Contains(strings.ToLower(item.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Summary), query) {
		return true
	}
	for _, tag := range item.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

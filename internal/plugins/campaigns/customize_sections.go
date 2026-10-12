// customize_sections.go — registry of the named parts of the Customize draft
// and the code that applies each.
//
// The campaigns plugin owns only the draft's storage, its API and the order
// sections are applied in. Each plugin that owns data the Customize page edits
// registers a section for it, so the campaigns plugin never imports it (T-B2)
// and a section's storage and validation stay with the plugin that owns it.

package campaigns

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
)

// customizeSectionKeyPattern is what a section key may look like. Keys appear
// in the draft JSON, the publish response and audit entries, so they stay
// short and URL-safe.
var customizeSectionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// CustomizeSection is one named part of the Customize draft and the code that
// applies it.
//
// A section's value may carry client-only keys that start with "_" (for
// example "_changes", the labels the page shows in its save bar). Apply must
// ignore keys it does not know, underscore ones included.
type CustomizeSection struct {
	// Key names the section in the draft: ^[a-z][a-z0-9_-]{0,31}$.
	Key string
	// Apply applies raw and returns what is still unapplied (nil when all of
	// it applied). On error, remaining is what to keep in the draft; nil
	// means keep raw unchanged.
	Apply func(ctx context.Context, cc *CampaignContext, userID string, raw json.RawMessage) (remaining json.RawMessage, err error)
}

// RegisterCustomizeSection adds a section. Registration order is apply order,
// so a section that depends on another (a category's menu entry needs the
// look's sidebar settings saved first) registers after it. A bad or repeated
// key is logged and ignored: a plugin wiring mistake must not stop the server
// from booting.
func (h *Handler) RegisterCustomizeSection(s CustomizeSection) {
	if !customizeSectionKeyPattern.MatchString(s.Key) {
		slog.Error("customize section not registered: invalid key", slog.String("key", s.Key))
		return
	}
	if s.Apply == nil {
		slog.Error("customize section not registered: no Apply", slog.String("key", s.Key))
		return
	}
	for _, have := range h.customizeSections {
		if have.Key == s.Key {
			slog.Error("customize section not registered: duplicate key", slog.String("key", s.Key))
			return
		}
	}
	h.customizeSections = append(h.customizeSections, s)
}

// CustomizeSectionKeys lists the registered section keys in apply order.
func (h *Handler) CustomizeSectionKeys() []string {
	keys := make([]string, len(h.customizeSections))
	for i, s := range h.customizeSections {
		keys[i] = s.Key
	}
	return keys
}

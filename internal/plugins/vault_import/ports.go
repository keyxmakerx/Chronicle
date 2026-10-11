package vault_import

import "context"

// PageKind is a page type a page can be created as.
type PageKind struct {
	ID      int
	Slug    string
	Enabled bool
}

// NewPage describes a page to create. Every imported page is created GM only:
// there is deliberately no field to say otherwise.
type NewPage struct {
	Name     string
	KindID   int
	Label    string
	ParentID string // empty for the top level
}

// PageStore is the part of the pages service the import writes through. The
// adapter in internal/app implements it over the entities service, so the
// normal rules (slug, version history, change events) apply.
type PageStore interface {
	Kinds(ctx context.Context, campaignID string) ([]PageKind, error)
	// NameTaken reports whether a live page already uses this name's address.
	NameTaken(ctx context.Context, campaignID, name string) (bool, error)
	// CreatePage creates a GM-only page and returns its id.
	CreatePage(ctx context.Context, campaignID, userID string, p NewPage) (string, error)
	// SetBody stores a page's text, as editor JSON plus the HTML shown to readers.
	SetBody(ctx context.Context, pageID, editorJSON, html string) error
	Rename(ctx context.Context, pageID, name string) error
}

// PictureStore stores a picture through the media service, with its type,
// size and quota checks, and returns the id the text refers to.
type PictureStore interface {
	StorePicture(ctx context.Context, campaignID, userID, name string, data []byte) (mediaID string, err error)
}

// FileAttacher puts a file in a page's Files section, GM only.
type FileAttacher interface {
	// CanAttach reports whether a file of this name is a type pages accept.
	CanAttach(name string) bool
	AttachGMOnly(ctx context.Context, campaignID, pageID, userID, name string, data []byte) error
}

// EditorJSON converts sanitised HTML to the editor's document format.
type EditorJSON func(html string) (string, error)

// AuditLogger records that an import ran. Optional.
type AuditLogger interface {
	LogCampaignEvent(ctx context.Context, campaignID, action string, details map[string]any)
}

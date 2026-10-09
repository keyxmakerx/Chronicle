package quests

import "context"

// The interfaces below are owned by this plugin and implemented in
// internal/app over the entities, maps and campaigns services, so quests
// never imports another plugin (plugin isolation).

// EntityInfo is the little this plugin needs to know about a page.
type EntityInfo struct {
	ID        string
	Name      string
	TypeName  string
	TypeSlug  string
	ImagePath string
}

// EntityDirectory answers questions about pages.
type EntityDirectory interface {
	// Entities returns the requested pages that exist IN the campaign; a
	// missing id or one in another campaign is simply absent.
	Entities(ctx context.Context, campaignID string, ids []string) (map[string]EntityInfo, error)
	// FilterViewable returns the subset of ids the viewer may open, using
	// the entities plugin's own visibility rules.
	FilterViewable(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]bool, error)
	// Search finds pages by name for the picker, already limited to what
	// the viewer may see.
	Search(ctx context.Context, campaignID, query string, role int, userID string, limit int) ([]EntityInfo, error)
}

// CharacterInfo is a character page and the display name of the member who
// claimed it ("" when unclaimed).
type CharacterInfo struct {
	ID     string
	Name   string
	Player string
}

// CharacterDirectory lists the campaign's characters for handing out rewards.
type CharacterDirectory interface {
	// ListCharacters returns the character pages the viewer may see.
	ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]CharacterInfo, error)
}

// MapInfo is a map's id and name.
type MapInfo struct {
	ID   string
	Name string
}

// MapDirectory answers questions about maps.
type MapDirectory interface {
	// Maps returns the requested maps that exist in the campaign.
	Maps(ctx context.Context, campaignID string, ids []string) (map[string]MapInfo, error)
	// ListMaps returns every map of the campaign.
	ListMaps(ctx context.Context, campaignID string) ([]MapInfo, error)
}

// MemberNames resolves campaign members' display names.
type MemberNames interface {
	DisplayNames(ctx context.Context, campaignID string, userIDs []string) (map[string]string, error)
}

// TypeDirectory answers whether a category (entity type) belongs to a campaign,
// so a category id from another campaign can never be used as a board home.
type TypeDirectory interface {
	// TypeInCampaign is false for a missing type and for one of another campaign.
	TypeInCampaign(ctx context.Context, campaignID string, typeID int) (bool, error)
}

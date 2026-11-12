package designs

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Origins say how a library row came to be. They are labels only: a copy
// never links back to what it was copied from.
const (
	OriginMade   = "made"
	OriginCode   = "code"
	OriginShared = "shared"
)

// Limits on the free text a design carries, which is all a reader sees that a
// kind's validator does not check.
const (
	MaxNameLen      = 60
	MaxNoteLen      = 140
	MaxMakerNameLen = 100
	// MaxCodeBytes caps a share code's decoded JSON. A room theme is well under
	// 2 KB; 16 KB leaves room for richer kinds without admitting a blob.
	MaxCodeBytes = 16 * 1024
	// MaxDataBytes caps one design's data on save, matching the code cap so a
	// design that saves can always be shared as a code.
	MaxDataBytes = MaxCodeBytes
	// MaxLibrary stops one campaign from filling the table.
	MaxLibrary = 500
)

// CodePrefix starts every share code; the version lets a later format coexist.
const CodePrefix = "chronicle-design:v1:"

// Design is one saved design as the Owner's library shows it.
type Design struct {
	ID          string          `json:"id"`
	CampaignID  string          `json:"campaignId"`
	Kind        string          `json:"kind"`
	SystemID    string          `json:"systemId"`
	Name        string          `json:"name"`
	Note        string          `json:"note"`
	MakerUserID string          `json:"-"`
	MakerName   string          `json:"makerName"`
	Data        json.RawMessage `json:"data"`
	Origin      string          `json:"origin"`
	OriginRef   string          `json:"originRef,omitempty"`
	// Shared is true while the design has a working link or any named owner.
	Shared    bool      `json:"shared"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SaveInput is what the Owner sends to put a design in the library.
type SaveInput struct {
	Kind string
	Name string
	Note string
	Data json.RawMessage
}

// UpdateDesignInput renames or re-notes a design. It is a partial update: an
// absent key keeps what is stored, so a rename never touches the note.
type UpdateDesignInput struct {
	Name patch.Field[string] `json:"name"`
	Note patch.Field[string] `json:"note"`
}

// Owner names a person a design is shared with.
type Owner struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
}

// ShareState is everything the Share sheet shows for one design.
type ShareState struct {
	Code    string  `json:"code"`
	LinkOn  bool    `json:"linkOn"`
	LinkURL string  `json:"linkUrl,omitempty"`
	Owners  []Owner `json:"owners"`
}

// CampaignInfo is the little the sharing rules need to know of a campaign.
type CampaignInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SystemID string `json:"systemId"`
}

// OwnerMatch is a search hit: an owner on this instance and the game systems
// they run a campaign on. Campaign names and ids stay private, because a name
// can give away a table the searcher was never invited to.
type OwnerMatch struct {
	UserID      string   `json:"userId"`
	DisplayName string   `json:"displayName"`
	Systems     []string `json:"systems"`
}

// Preview is what a reader sees before taking a design: who made it, what it
// is, whether it was made for the same game system, and what the receiving
// campaign lacks.
type Preview struct {
	Name       string   `json:"name"`
	Note       string   `json:"note,omitempty"`
	Kind       string   `json:"kind"`
	MakerName  string   `json:"makerName"`
	SystemID   string   `json:"systemId"`
	SameSystem bool     `json:"sameSystem"`
	Lacks      []string `json:"lacks"`
}

// LinkPreview is a link's preview plus the campaigns the reader may copy the
// design into: their own, on the design's game system.
type LinkPreview struct {
	Preview
	Targets []LinkTarget `json:"targets"`
}

// LinkTarget is one campaign a link's design can be copied into, with what
// that campaign lacks for it.
type LinkTarget struct {
	CampaignInfo
	Lacks []string `json:"lacks"`
}

// SharedDesign is a design shared to the current user by name.
type SharedDesign struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	MakerName    string `json:"makerName"`
	FromCampaign string `json:"fromCampaign"`
	// campaignID lets the service drop designs whose source campaign is gone;
	// it is never sent to the client.
	campaignID string
}

// codeBody is the JSON inside a share code. It is unsigned on purpose so a
// code works across instances; whatever reads one validates it again.
type codeBody struct {
	Kind      string          `json:"kind"`
	SystemID  string          `json:"system_id"`
	Name      string          `json:"name"`
	MakerName string          `json:"maker_name"`
	Data      json.RawMessage `json:"data"`
}

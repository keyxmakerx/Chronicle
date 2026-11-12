package designs

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// OriginGallery labels a library row copied from the server gallery. Like the
// other origins it is a label only; the copy never links back.
const OriginGallery = "gallery"

// Gallery entry states. Only StatusVisible is shown to anyone but the
// publisher and the instance admins.
const (
	StatusVisible  = "visible"
	StatusPending  = "pending"
	StatusRejected = "rejected"
	StatusRemoved  = "removed"
)

// Gallery limits.
const (
	// GalleryPageSize is how many cards one browse page holds.
	GalleryPageSize = 12
	// ReportThreshold is how many distinct reports send a visible entry back
	// to review.
	ReportThreshold = 3
	// MaxReportNoteLen caps the optional note on a report.
	MaxReportNoteLen = 140
	// maxSummaryLen caps the one-line parts summary an admin or a card reads.
	maxSummaryLen = 160
)

// Report reasons an Owner may give.
const (
	ReportAdult = "adult"
	ReportSpam  = "spam"
	ReportOther = "other"
)

// Kind filters of the browse sheet.
const (
	KindFilterAll      = "all"
	KindFilterRooms    = "rooms"
	KindFilterCrafting = "crafting"
)

// GalleryEntry is a published snapshot. Data is only ever sent out through a
// copy, never in a listing.
type GalleryEntry struct {
	ID              string
	PublisherUserID string
	SourceDesignID  string
	Kind            string
	SystemID        string
	Name            string
	Note            string
	MakerName       string
	Data            json.RawMessage
	DataHash        string
	Status          string
	Reasons         []string
	ReviewedBy      string
	PublishedAt     time.Time
	UpdatedAt       time.Time
}

// GalleryReport is one Owner's report of an entry.
type GalleryReport struct {
	ReporterUserID string    `json:"-"`
	Reason         string    `json:"reason"`
	Note           string    `json:"note,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// PublishInput is what the Share sheet sends. An empty name or shown-as name
// falls back to the library design's name and the Owner's display name.
type PublishInput struct {
	Name      string
	Note      string
	MakerName string
}

// RepublishInput is "Update in gallery": a partial update, so a key left out
// keeps what the entry already says. The design data is always re-read from the
// library.
type RepublishInput struct {
	Name      patch.Field[string] `json:"name"`
	Note      patch.Field[string] `json:"note"`
	MakerName patch.Field[string] `json:"makerName"`
}

// ReportInput is an Owner's report of a visible entry.
type ReportInput struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// BrowseInput is the browse sheet's filters.
type BrowseInput struct {
	AllSystems bool
	Kind       string
	Q          string
	Page       int
}

// GalleryCard is one visible entry as the browse sheet shows it.
type GalleryCard struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	SystemID    string    `json:"systemId"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	MakerName   string    `json:"makerName"`
	Summary     string    `json:"summary"`
	PublishedAt time.Time `json:"publishedAt"`
	Mine        bool      `json:"mine"`
	SameSystem  bool      `json:"sameSystem"`
	Lacks       []string  `json:"lacks"`
	Reported    bool      `json:"reported"`
}

// GalleryPage is one page of browse results.
type GalleryPage struct {
	Entries  []GalleryCard `json:"entries"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PerPage  int           `json:"perPage"`
	SystemID string        `json:"systemId"`
}

// GalleryMine is one of the publisher's own entries, with its state and, when
// it waits or was refused, the reasons in plain words (never the matched term).
type GalleryMine struct {
	ID             string    `json:"id"`
	SourceDesignID string    `json:"sourceDesignId,omitempty"`
	Kind           string    `json:"kind"`
	SystemID       string    `json:"systemId"`
	Name           string    `json:"name"`
	Note           string    `json:"note"`
	MakerName      string    `json:"makerName"`
	Status         string    `json:"status"`
	Reasons        []string  `json:"reasons"`
	PublishedAt    time.Time `json:"publishedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// AdminEntry is an entry as the admin queue shows it: who made it, why it is
// there and what has been reported.
type AdminEntry struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	SystemID    string          `json:"systemId"`
	Name        string          `json:"name"`
	Note        string          `json:"note"`
	MakerName   string          `json:"makerName"`
	Summary     string          `json:"summary"`
	Status      string          `json:"status"`
	ReasonCodes []string        `json:"reasonCodes"`
	Reasons     []string        `json:"reasons"`
	Reports     []GalleryReport `json:"reports"`
	PublishedAt time.Time       `json:"publishedAt"`
}

// AdminPage is one page of the admin queue.
type AdminPage struct {
	Entries []AdminEntry `json:"entries"`
	Total   int          `json:"total"`
	Page    int          `json:"page"`
	PerPage int          `json:"perPage"`
	Status  string       `json:"status"`
}

// Admin decisions.
const (
	DecisionApprove = "approve"
	DecisionReject  = "reject"
	DecisionRemove  = "remove"
)

// BrowseQuery is a browse after the service has turned the filters into what
// the repository can run.
type BrowseQuery struct {
	SystemID string // "" means every system
	AnySys   bool
	Kinds    []string
	Prefix   string // kind prefix, for "crafting-*"
	Text     string
	Limit    int
	Offset   int
}

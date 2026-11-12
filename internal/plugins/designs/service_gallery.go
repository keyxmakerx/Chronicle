package designs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Gallery is the server gallery: publishing a snapshot of a library design,
// browsing and copying what other Owners published, reporting, and the admin
// queue. Like Service, every campaign-scoped call checks the Owner itself; the
// Admin* calls trust their route gate (/admin, site admins only) and never
// reach the Owner rules.
type Gallery interface {
	Publish(ctx context.Context, campaignID, userID, designID string, in PublishInput) (*GalleryMine, error)
	Republish(ctx context.Context, campaignID, userID, designID string, in RepublishInput) (*GalleryMine, error)
	Unpublish(ctx context.Context, campaignID, userID, entryID string) error
	Mine(ctx context.Context, campaignID, userID string) ([]GalleryMine, error)
	Browse(ctx context.Context, campaignID, userID string, in BrowseInput) (*GalleryPage, error)
	Copy(ctx context.Context, campaignID, userID, entryID string) (*Design, error)
	Report(ctx context.Context, campaignID, userID, entryID string, in ReportInput) error

	AdminQueue(ctx context.Context, status string, page int) (*AdminPage, error)
	// AdminDecide applies approve, reject or remove and returns the entry as it
	// now stands. It records nothing itself; the handler logs the decision.
	AdminDecide(ctx context.Context, actorID, entryID, decision string) (*AdminEntry, error)
}

type gallery struct {
	lib      *service
	repo     GalleryRepository
	maturity *maturityList
	now      func() time.Time
}

// NewGalleryService builds the gallery over the same library, directory and
// kind registry the library service uses, so a published design is validated
// by the feature that owns its kind.
func NewGalleryService(lib Repository, repo GalleryRepository, dir CampaignDirectory, kinds KindRegistry) Gallery {
	return &gallery{lib: &service{repo: lib, dir: dir, kinds: kinds}, repo: repo, maturity: defaultMaturity, now: time.Now}
}

const (
	msgEntryGone   = "that gallery entry was not found"
	msgAlreadyPub  = "this design is already in the gallery; use Update in gallery"
	msgNotPub      = "this design is not in the gallery"
	msgOwnReport   = "you cannot report your own design"
	msgReportTwice = "you already reported this design"
)

// dataHash identifies a design's exact choices; the kind is part of it so two
// kinds with the same JSON never collide.
func dataHash(kind string, data []byte) string {
	h := sha256.Sum256(append([]byte(kind+"\n"), data...))
	return hex.EncodeToString(h[:])
}

// vetPublish runs every check on a candidate and returns the reasons it should
// wait for review; none means clean. The attempt counts toward the daily limit
// whatever the outcome, so a flagged retry loop still hits the wall.
func (g *gallery) vetPublish(ctx context.Context, userID, name, note, maker, hash, exceptID string) ([]string, error) {
	reasons := textReasons(g.maturity, name, note, maker)
	n, err := g.repo.CountEvents(ctx, userID, g.now().Add(-24*time.Hour))
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if n >= MaxGalleryPerDay {
		reasons = append(reasons, ReasonRate)
	}
	dup, err := g.repo.DuplicateExists(ctx, userID, hash, exceptID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if dup {
		reasons = append(reasons, ReasonDuplicate)
	}
	if err := g.repo.AddEvent(ctx, userID); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return reasons, nil
}

func statusFor(reasons []string) string {
	if len(reasons) == 0 {
		return StatusVisible
	}
	return StatusPending
}

// labels cleans the three free-text fields of a publish.
func labels(name, note, maker string) (string, string, string, error) {
	name, err := checkLabel("name", name, MaxNameLen, true)
	if err != nil {
		return "", "", "", err
	}
	if note, err = checkLabel("note", note, MaxNoteLen, false); err != nil {
		return "", "", "", err
	}
	maker, err = checkLabel("shown-as name", maker, MaxMakerNameLen, true)
	return name, note, maker, err
}

func (g *gallery) Publish(ctx context.Context, campaignID, userID, designID string, in PublishInput) (*GalleryMine, error) {
	_, display, err := g.lib.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	d, err := g.lib.own(ctx, campaignID, designID)
	if err != nil {
		return nil, err
	}
	name, note, maker, err := labels(orDefault(in.Name, d.Name), in.Note, orDefault(in.MakerName, display))
	if err != nil {
		return nil, err
	}
	if old, err := g.repo.BySource(ctx, userID, designID); err != nil {
		return nil, apperror.NewInternal(err)
	} else if old != nil {
		return nil, apperror.NewConflict(msgAlreadyPub)
	}
	// The kind validates the data again: the snapshot is only ever known parts.
	data, err := g.lib.validData(d.Kind, d.Data)
	if err != nil {
		return nil, err
	}
	hash := dataHash(d.Kind, data)
	reasons, err := g.vetPublish(ctx, userID, name, note, maker, hash, "")
	if err != nil {
		return nil, err
	}
	e := &GalleryEntry{
		ID: newID(), PublisherUserID: userID, SourceDesignID: designID, Kind: d.Kind, SystemID: d.SystemID,
		Name: name, Note: note, MakerName: maker, Data: data, DataHash: hash,
		Status: statusFor(reasons), Reasons: reasons,
	}
	if err := g.repo.Insert(ctx, e); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return g.mine(ctx, e.ID)
}

func orDefault(s, alt string) string {
	if cleanText(s) == "" {
		return alt
	}
	return s
}

// Republish takes a fresh snapshot of the library design and runs every check
// again. An entry an admin refused or removed never goes straight back on show,
// and neither does one the community reported: those wait for an admin.
func (g *gallery) Republish(ctx context.Context, campaignID, userID, designID string, in RepublishInput) (*GalleryMine, error) {
	if _, _, err := g.lib.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	d, err := g.lib.own(ctx, campaignID, designID)
	if err != nil {
		return nil, err
	}
	e, err := g.repo.BySource(ctx, userID, designID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if e == nil {
		return nil, apperror.NewNotFound(msgNotPub)
	}
	note := e.Note
	if in.Note.IsNull() {
		note = ""
	} else {
		note = in.Note.Val(note)
	}
	name, note, maker, err := labels(in.Name.Val(e.Name), note, in.MakerName.Val(e.MakerName))
	if err != nil {
		return nil, err
	}
	data, err := g.lib.validData(d.Kind, d.Data)
	if err != nil {
		return nil, err
	}
	hash := dataHash(d.Kind, data)
	reasons, err := g.vetPublish(ctx, userID, name, note, maker, hash, e.ID)
	if err != nil {
		return nil, err
	}
	prev := e.Status
	switch {
	case prev == StatusRejected || prev == StatusRemoved:
		reasons = append(reasons, ReasonResubmitted)
	case prev == StatusPending && slices.Contains(e.Reasons, ReasonReported):
		reasons = append(reasons, ReasonReported)
	}
	e.Name, e.Note, e.MakerName, e.Data, e.DataHash, e.SystemID = name, note, maker, data, hash, d.SystemID
	e.Kind = d.Kind
	e.Status, e.Reasons, e.ReviewedBy = statusFor(reasons), reasons, ""
	if err := g.repo.Save(ctx, e); err != nil {
		return nil, apperror.NewInternal(err)
	}
	// Reports were about the old text; a fresh clean entry starts without them.
	if e.Status == StatusVisible {
		if err := g.repo.ClearReports(ctx, e.ID); err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	return g.mine(ctx, e.ID)
}

// Unpublish removes the entry. Copies already taken are separate library rows
// and stay. An entry that is not the caller's is NotFound.
func (g *gallery) Unpublish(ctx context.Context, campaignID, userID, entryID string) error {
	if _, _, err := g.lib.requireOwner(ctx, campaignID, userID); err != nil {
		return err
	}
	e, err := g.repo.Get(ctx, entryID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if e == nil || e.PublisherUserID != userID {
		return apperror.NewNotFound(msgEntryGone)
	}
	if err := g.repo.Delete(ctx, entryID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func toMine(e *GalleryEntry) GalleryMine {
	words := []string{}
	if e.Status != StatusVisible {
		for _, r := range e.Reasons {
			words = append(words, ReasonText(r))
		}
		if e.Status == StatusRemoved && len(e.Reasons) == 0 {
			words = append(words, ReasonText(ReasonRemoved))
		}
	}
	return GalleryMine{
		ID: e.ID, SourceDesignID: e.SourceDesignID, Kind: e.Kind, SystemID: e.SystemID, Name: e.Name, Note: e.Note,
		MakerName: e.MakerName, Status: e.Status, Reasons: words, PublishedAt: e.PublishedAt, UpdatedAt: e.UpdatedAt,
	}
}

func (g *gallery) mine(ctx context.Context, id string) (*GalleryMine, error) {
	e, err := g.repo.Get(ctx, id)
	if err != nil || e == nil {
		return nil, apperror.NewInternal(err)
	}
	m := toMine(e)
	return &m, nil
}

func (g *gallery) Mine(ctx context.Context, campaignID, userID string) ([]GalleryMine, error) {
	if _, _, err := g.lib.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	list, err := g.repo.ByPublisher(ctx, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := make([]GalleryMine, 0, len(list))
	for i := range list {
		out = append(out, toMine(&list[i]))
	}
	return out, nil
}

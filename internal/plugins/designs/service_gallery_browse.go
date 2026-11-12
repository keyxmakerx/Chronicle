package designs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// summarize is a one-line reading of a design's parts for a card or the admin
// queue: top-level scalars as "key: value" and lists as a count. A kind's data
// is opaque here, so this stays generic; it is capped and only ever escaped
// text on the page.
func summarize(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "name" && k != "version" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := m[k].(type) {
		case string, float64, bool:
			parts = append(parts, fmt.Sprintf("%s: %v", k, v))
		case []any:
			parts = append(parts, fmt.Sprintf("%s: %d", k, len(v)))
		case map[string]any:
			parts = append(parts, fmt.Sprintf("%s: %d parts", k, len(v)))
		}
	}
	s := strings.Join(parts, " · ")
	if utf8.RuneCountInString(s) > maxSummaryLen {
		s = string([]rune(s)[:maxSummaryLen-1]) + "…"
	}
	return s
}

// clampPage keeps a page number inside a sane range, so an absurd offset never
// reaches the database.
func clampPage(p int) int {
	if p < 1 {
		return 1
	}
	if p > MaxPage {
		return MaxPage
	}
	return p
}

// browseQuery turns the sheet's filters into a repository query.
func browseQuery(c *CampaignInfo, in BrowseInput) (BrowseQuery, int, error) {
	q := BrowseQuery{SystemID: c.SystemID, AnySys: in.AllSystems, Limit: GalleryPageSize}
	switch in.Kind {
	case "", KindFilterAll:
	case KindFilterRooms:
		q.Kinds = []string{KindRoomTheme}
	case KindFilterCrafting:
		q.Prefix = "crafting-"
	default:
		return q, 0, apperror.NewBadRequest("kind must be all, rooms or crafting")
	}
	text := cleanText(in.Q)
	if utf8.RuneCountInString(text) > MaxNameLen {
		text = string([]rune(text)[:MaxNameLen])
	}
	q.Text = text
	page := clampPage(in.Page)
	q.Offset = (page - 1) * GalleryPageSize
	return q, page, nil
}

// Browse lists visible entries only, to any Owner on the server.
func (g *gallery) Browse(ctx context.Context, campaignID, userID string, in BrowseInput) (*GalleryPage, error) {
	c, _, err := g.lib.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	q, page, err := browseQuery(c, in)
	if err != nil {
		return nil, err
	}
	list, total, err := g.repo.Browse(ctx, q)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	reported, err := g.repo.ReportedBy(ctx, userID, ids)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := &GalleryPage{Entries: make([]GalleryCard, 0, len(list)), Total: total, Page: page, PerPage: GalleryPageSize, SystemID: c.SystemID}
	for i := range list {
		e := &list[i]
		card := GalleryCard{
			ID: e.ID, Kind: e.Kind, SystemID: e.SystemID, Name: e.Name, Note: e.Note, MakerName: e.MakerName,
			Summary: summarize(e.Data), PublishedAt: e.PublishedAt, Mine: e.PublisherUserID == userID,
			SameSystem: e.SystemID == c.SystemID, Lacks: []string{}, Reported: reported[e.ID],
		}
		// A kind this server no longer knows is shown without needs and refused
		// on copy, rather than hiding the entry from its maker's friends.
		if k, ok := g.lib.kinds.Kind(e.Kind); ok {
			lacks, err := k.Lacks(ctx, c.ID, e.Data)
			if err != nil {
				return nil, apperror.NewInternal(err)
			}
			if lacks != nil {
				card.Lacks = lacks
			}
		}
		out.Entries = append(out.Entries, card)
	}
	return out, nil
}

// visible loads an entry any Owner may see; one that is waiting, refused or
// removed is NotFound, the same as a missing one.
func (g *gallery) visible(ctx context.Context, id string) (*GalleryEntry, error) {
	e, err := g.repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if e == nil || e.Status != StatusVisible {
		return nil, apperror.NewNotFound(msgEntryGone)
	}
	return e, nil
}

// Copy takes an independent copy into the route's campaign. A design from
// another game system is allowed, as a share code is: the kind's check refuses
// a part that does not exist here, and the card already said what is lacking.
func (g *gallery) Copy(ctx context.Context, campaignID, userID, entryID string) (*Design, error) {
	c, _, err := g.lib.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	e, err := g.visible(ctx, entryID)
	if err != nil {
		return nil, err
	}
	in := incoming{kind: e.Kind, name: e.Name, note: e.Note, makerName: e.MakerName, makerUser: e.PublisherUserID, systemID: e.SystemID, data: e.Data}
	_, data, err := g.lib.preview(ctx, c, in)
	if err != nil {
		return nil, err
	}
	return g.lib.store(ctx, c, newDesign{
		kind: e.Kind, name: e.Name, note: e.Note, data: data,
		makerUser: e.PublisherUserID, makerName: e.MakerName, origin: OriginGallery, ref: e.MakerName,
	})
}

// Report records one Owner's report of a visible entry. The third distinct
// report sends it back to review; the reports stay for the admin to read.
func (g *gallery) Report(ctx context.Context, campaignID, userID, entryID string, in ReportInput) error {
	if _, _, err := g.lib.requireOwner(ctx, campaignID, userID); err != nil {
		return err
	}
	switch in.Reason {
	case ReportAdult, ReportSpam, ReportOther:
	default:
		return apperror.NewBadRequest("the reason must be adult, spam or other")
	}
	note, err := checkLabel("note", in.Note, MaxReportNoteLen, false)
	if err != nil {
		return err
	}
	e, err := g.visible(ctx, entryID)
	if err != nil {
		return err
	}
	if e.PublisherUserID == userID {
		return apperror.NewBadRequest(msgOwnReport)
	}
	added, err := g.repo.AddReport(ctx, e.ID, userID, in.Reason, note)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !added {
		return apperror.NewConflict(msgReportTwice)
	}
	reports, err := g.repo.Reports(ctx, e.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if len(reports) >= ReportThreshold {
		// A narrow update, not a save of the entry read above: an admin or the
		// publisher may have changed it since, and a stale save would undo that.
		if _, err := g.repo.FlagReported(ctx, e.ID, []string{ReasonReported}); err != nil {
			return apperror.NewInternal(err)
		}
	}
	return nil
}

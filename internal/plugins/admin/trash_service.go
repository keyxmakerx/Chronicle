package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
)

// The site Trash holds two kinds of thing for a few days before anything is
// removed for good: a deleted campaign, and the set of unused uploads an admin
// cleaned up in one go. Undo brings either back exactly as it was; the purge
// is the only step that deletes. This file is the rules; the SQL lives in
// campaigns/trash_repository.go, media/trash_repository.go and
// trash_repository.go (the batch table).

const (
	// trashPurgeInterval is how often the background purger looks for items
	// past their time. An item can wait up to this long beyond its day count.
	trashPurgeInterval = time.Hour
	// emptyBatchGrace is how long a batch row with no counted files is left
	// before it is recounted, so a clean-up still being written is not touched.
	emptyBatchGrace = 10 * time.Minute
)

// TrashKind says what a Trash row holds.
type TrashKind string

const (
	TrashCampaign TrashKind = "campaign"
	TrashFiles    TrashKind = "files"
)

// TrashActor is who did the deleting, for the "deleted by" copy. Empty for a
// system action.
type TrashActor struct {
	UserID string
	Name   string
}

// TrashEntry is one row of the Trash page, already phrased for display.
type TrashEntry struct {
	Kind TrashKind
	ID   string
	// Title is the row's heading, e.g. "Campaign: Shattered Coast" or
	// "Files clean-up: 42 unused pictures, 180 MB".
	Title string
	// Who is "deleted by alex" for a campaign; empty for a clean-up.
	Who string
	// When is "3 days ago"; Empties is "empties in 27 days".
	When    string
	Empties string
	// Emptying is true while the final delete is under way; Undo is hidden.
	Emptying  bool
	DeletedAt time.Time
}

// TrashOverview is everything the Trash page shows.
type TrashOverview struct {
	Entries       []TrashEntry
	RetentionDays int
}

// EmptyResult says what a purge removed.
type EmptyResult struct {
	Campaigns int
	Batches   int
	// Failed counts items that could not be finished this run; they stay in
	// the Trash and the next run tries again.
	Failed int
}

// TrashService runs the site Trash.
type TrashService interface {
	// Overview lists what is waiting, newest first, with phrased dates.
	Overview(ctx context.Context) (*TrashOverview, error)
	// TrashUnusedUploads moves every unused, campaignless upload into one
	// Trash batch. It returns nil when there was nothing to move.
	TrashUnusedUploads(ctx context.Context, actor TrashActor) (*TrashBatch, error)
	// UndoCampaign and UndoBatch bring one item back. They fail with Not
	// Found when the item is gone or its final delete has begun.
	UndoCampaign(ctx context.Context, campaignID string) (name string, err error)
	UndoBatch(ctx context.Context, batchID string) (*TrashBatch, error)
	// EmptyNow removes everything in the Trash for good, whatever its age.
	EmptyNow(ctx context.Context) (EmptyResult, error)
	// PurgeDue removes only what has waited out the retention, and finishes
	// anything a previous run left half done.
	PurgeDue(ctx context.Context) (EmptyResult, error)
	// RetentionDays is the setting the purger obeys.
	RetentionDays(ctx context.Context) int
	// StartPurger runs PurgeDue now and then every hour until ctx ends.
	StartPurger(ctx context.Context)
}

// TrashedCampaigns is the part of the campaigns service the Trash needs.
type TrashedCampaigns interface {
	ListTrashed(ctx context.Context) ([]campaigns.TrashedCampaign, error)
	RestoreFromTrash(ctx context.Context, campaignID string) error
	ListPurgeDue(ctx context.Context, cutoff time.Time, all bool) ([]string, error)
	PurgeTrashed(ctx context.Context, campaignID string) (bool, error)
}

// TrashedFiles is the part of the media service the Trash needs.
type TrashedFiles interface {
	TrashFiles(ctx context.Context, batchID string, ids []string) (count int, bytes int64, err error)
	RestoreTrashedFiles(ctx context.Context, batchID string) (int, error)
	ListTrashedFileIDs(ctx context.Context, batchID string) ([]string, error)
	PurgeTrashedFile(ctx context.Context, batchID, fileID string) error
}

// UnusedUploadFinder lists the uploads a clean-up would move; the data hygiene
// scanner is the one place that knows what counts as unused.
type UnusedUploadFinder interface {
	ScanOrphanedMedia(ctx context.Context) ([]OrphanedMediaItem, error)
}

// TrashRetention reads the retention setting.
type TrashRetention interface {
	SiteTrashRetentionDays(ctx context.Context) int
}

type trashService struct {
	campaigns TrashedCampaigns
	files     TrashedFiles
	finder    UnusedUploadFinder
	batches   TrashBatchRepository
	retention TrashRetention
	now       func() time.Time

	// purgeMu keeps two purges in this process from interleaving. Another
	// process is still safe: every step is a conditional claim or repeats
	// harmlessly.
	purgeMu sync.Mutex
}

// NewTrashService wires the Trash. retention may be nil, which means the
// default period.
func NewTrashService(c TrashedCampaigns, f TrashedFiles, finder UnusedUploadFinder, b TrashBatchRepository, r TrashRetention) TrashService {
	return &trashService{campaigns: c, files: f, finder: finder, batches: b, retention: r, now: func() time.Time { return time.Now().UTC() }}
}

func (s *trashService) RetentionDays(ctx context.Context) int {
	if s.retention == nil {
		return settings.DefaultSiteTrashRetentionDays
	}
	days := s.retention.SiteTrashRetentionDays(ctx)
	if !settings.IsValidSiteTrashRetention(days) {
		return settings.DefaultSiteTrashRetentionDays
	}
	return days
}

// --- Reading ---

func (s *trashService) Overview(ctx context.Context) (*TrashOverview, error) {
	now := s.now()
	days := s.RetentionDays(ctx)

	trashed, err := s.campaigns.ListTrashed(ctx)
	if err != nil {
		return nil, err
	}
	batches, err := s.batches.List(ctx)
	if err != nil {
		return nil, err
	}

	out := &TrashOverview{RetentionDays: days}
	for _, c := range trashed {
		e := TrashEntry{
			Kind: TrashCampaign, ID: c.ID, DeletedAt: c.DeletedAt,
			Title:    "Campaign: " + c.Name,
			When:     agoLabel(now, c.DeletedAt),
			Empties:  emptiesLabel(now, c.DeletedAt, days),
			Emptying: c.Emptying,
		}
		switch {
		case c.DeletedByName != "":
			e.Who = "deleted by " + c.DeletedByName
		case c.DeletedBy == nil:
			e.Who = "deleted"
		default:
			e.Who = "deleted by a former member"
		}
		out.Entries = append(out.Entries, e)
	}
	for _, b := range batches {
		// A batch still being restored is not shown: it is already back.
		if b.State == batchRestoring {
			continue
		}
		out.Entries = append(out.Entries, TrashEntry{
			Kind: TrashFiles, ID: b.ID, DeletedAt: b.CreatedAt,
			Title:    fmt.Sprintf("Files clean-up: %s", batchSummary(b)),
			When:     agoLabel(now, b.CreatedAt),
			Empties:  emptiesLabel(now, b.CreatedAt, days),
			Emptying: b.State == batchPurging,
		})
	}
	sort.SliceStable(out.Entries, func(i, j int) bool {
		return out.Entries[i].DeletedAt.After(out.Entries[j].DeletedAt)
	})
	return out, nil
}

// --- Trashing ---

func (s *trashService) TrashUnusedUploads(ctx context.Context, actor TrashActor) (*TrashBatch, error) {
	items, err := s.finder.ScanOrphanedMedia(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	allPictures := true
	for _, it := range items {
		// A file an entity still points at is not unused; leave it be.
		if it.Referenced {
			continue
		}
		ids = append(ids, it.ID)
		if !strings.HasPrefix(it.MimeType, "image/") {
			allPictures = false
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	label := "files"
	if allPictures {
		label = "pictures"
	}
	batch := &TrashBatch{
		ID: uuid.NewString(), Kind: trashKindFiles, Label: label,
		DeletedBy: actor.UserID, DeletedByName: actor.Name, CreatedAt: s.now(),
	}
	// The batch row goes in first so a file is never marked without a batch
	// to find it by; a crash before the totals are written is repaired by
	// reconcile, which recounts.
	if err := s.batches.Create(ctx, batch); err != nil {
		return nil, apperror.NewInternal(err)
	}
	count, size, err := s.files.TrashFiles(ctx, batch.ID, ids)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if count == 0 {
		_ = s.batches.Delete(ctx, batch.ID)
		return nil, nil
	}
	batch.Items, batch.Bytes = count, size
	if err := s.batches.SetTotals(ctx, batch.ID, count, size); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return batch, nil
}

// --- Undo ---

func (s *trashService) UndoCampaign(ctx context.Context, campaignID string) (string, error) {
	// The name is read first only for the confirmation message; Undo itself
	// is the single conditional update inside RestoreFromTrash.
	var name string
	if list, err := s.campaigns.ListTrashed(ctx); err == nil {
		for _, c := range list {
			if c.ID == campaignID {
				name = c.Name
			}
		}
	}
	if err := s.campaigns.RestoreFromTrash(ctx, campaignID); err != nil {
		return "", err
	}
	return name, nil
}

func (s *trashService) UndoBatch(ctx context.Context, batchID string) (*TrashBatch, error) {
	b, err := s.batches.Get(ctx, batchID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if b == nil {
		return nil, apperror.NewNotFound("that clean-up is not in the trash any more")
	}
	claimed, err := s.batches.Claim(ctx, batchID, batchTrashed, batchRestoring)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !claimed && b.State != batchRestoring {
		// The final delete got there first.
		return nil, apperror.NewNotFound("that clean-up is already being emptied")
	}
	if err := s.finishRestore(ctx, batchID); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return b, nil
}

// finishRestore completes an Undo that has claimed its batch. Both steps repeat
// harmlessly, so it also serves to finish one that stopped halfway.
func (s *trashService) finishRestore(ctx context.Context, batchID string) error {
	if _, err := s.files.RestoreTrashedFiles(ctx, batchID); err != nil {
		return err
	}
	return s.batches.Delete(ctx, batchID)
}

// --- Purging ---

func (s *trashService) EmptyNow(ctx context.Context) (EmptyResult, error) {
	return s.purge(ctx, true)
}

func (s *trashService) PurgeDue(ctx context.Context) (EmptyResult, error) {
	return s.purge(ctx, false)
}

// purge removes what is due (everything when all). Failures on one item are
// counted and logged, never allowed to stop the others.
func (s *trashService) purge(ctx context.Context, all bool) (EmptyResult, error) {
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()

	now := s.now()
	cutoff := now.AddDate(0, 0, -s.RetentionDays(ctx))
	var res EmptyResult

	ids, err := s.campaigns.ListPurgeDue(ctx, cutoff, all)
	if err != nil {
		return res, err
	}
	for _, id := range ids {
		purged, err := s.campaigns.PurgeTrashed(ctx, id)
		switch {
		case err != nil:
			res.Failed++
			slog.Error("trash: purging campaign failed", slog.String("campaign_id", id), slog.Any("error", err))
		case purged:
			res.Campaigns++
		}
	}

	batches, err := s.batches.List(ctx)
	if err != nil {
		return res, err
	}
	for _, b := range batches {
		switch b.State {
		case batchRestoring:
			// Undo claimed this and stopped; it is committed to coming back.
			if err := s.finishRestore(ctx, b.ID); err != nil {
				res.Failed++
				slog.Error("trash: finishing restore failed", slog.String("batch_id", b.ID), slog.Any("error", err))
			}
			continue
		case batchTrashed:
			if b.Items == 0 && now.Sub(b.CreatedAt) > emptyBatchGrace {
				s.reconcileEmpty(ctx, b)
				continue
			}
			if !all && !b.CreatedAt.Before(cutoff) {
				continue
			}
		}
		// batchPurging falls through: its final delete is under way and is
		// resumed whatever its age.
		purged, err := s.purgeBatch(ctx, b.ID)
		switch {
		case err != nil:
			res.Failed++
			slog.Error("trash: purging clean-up failed", slog.String("batch_id", b.ID), slog.Any("error", err))
		case purged:
			res.Batches++
		}
	}
	if res.Campaigns+res.Batches > 0 {
		slog.Info("trash emptied", slog.Int("campaigns", res.Campaigns), slog.Int("cleanups", res.Batches), slog.Bool("all", all))
	}
	return res, nil
}

// reconcileEmpty repairs a batch whose totals were never written (a crash
// between marking the files and saving the count): recount, then keep it with
// its real totals or drop the empty row.
func (s *trashService) reconcileEmpty(ctx context.Context, b TrashBatch) {
	count, size, err := s.files.TrashFiles(ctx, b.ID, nil)
	if err != nil {
		slog.Warn("trash: recounting batch failed", slog.String("batch_id", b.ID), slog.Any("error", err))
		return
	}
	if count == 0 {
		_ = s.batches.Delete(ctx, b.ID)
		return
	}
	_ = s.batches.SetTotals(ctx, b.ID, count, size)
}

// purgeBatch deletes one batch's files and then the batch. It claims the batch
// first; after that Undo is refused, so a file can never be deleted out from
// under a restore. It reports false, having done nothing, when the batch is
// gone or was claimed by Undo.
func (s *trashService) purgeBatch(ctx context.Context, batchID string) (bool, error) {
	claimed, err := s.batches.Claim(ctx, batchID, batchTrashed, batchPurging)
	if err != nil {
		return false, err
	}
	if !claimed {
		b, err := s.batches.Get(ctx, batchID)
		if err != nil {
			return false, err
		}
		if b == nil || b.State != batchPurging {
			return false, nil
		}
	}
	ids, err := s.files.ListTrashedFileIDs(ctx, batchID)
	if err != nil {
		return false, err
	}
	var firstErr error
	for _, id := range ids {
		if err := s.files.PurgeTrashedFile(ctx, batchID, id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		// Keep the batch, in the purging state, so the next run finishes it.
		return false, firstErr
	}
	if err := s.batches.Delete(ctx, batchID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *trashService) StartPurger(ctx context.Context) {
	run := func() {
		if _, err := s.PurgeDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("trash: automatic emptying failed", slog.Any("error", err))
		}
	}
	run()
	ticker := time.NewTicker(trashPurgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// --- Wording ---

// batchSummary reads "42 unused pictures, 180 MB".
func batchSummary(b TrashBatch) string {
	noun := b.Label
	if noun == "" {
		noun = "files"
	}
	if b.Items == 1 {
		noun = strings.TrimSuffix(noun, "s")
	}
	return fmt.Sprintf("%d unused %s, %s", b.Items, noun, formatBytes(b.Bytes))
}

// agoLabel phrases how long ago t was, in the coarsest unit that is still true.
func agoLabel(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return countUnit(int(d/time.Minute), "minute") + " ago"
	case d < 24*time.Hour:
		return countUnit(int(d/time.Hour), "hour") + " ago"
	case d < 48*time.Hour:
		return "yesterday"
	}
	return countUnit(int(d/(24*time.Hour)), "day") + " ago"
}

// emptiesLabel says when the purger will remove an item that was trashed at t.
func emptiesLabel(now, t time.Time, retentionDays int) string {
	left := t.AddDate(0, 0, retentionDays).Sub(now)
	switch {
	case left <= 0:
		return "empties soon"
	case left < 24*time.Hour:
		return "empties today"
	}
	// Whole days, rounded up, so "empties in 1 day" is never shown for an
	// item with a day and a half left.
	days := int((left + 24*time.Hour - 1) / (24 * time.Hour))
	return "empties in " + countUnit(days, "day")
}

func countUnit(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

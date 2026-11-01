package playerchanges

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Pages is what Undo needs from the pages a change was made on. It is
// implemented over the entities service in internal/app.
type Pages interface {
	// FieldValue is a sheet field's stored value; ok is false when the page
	// or field is gone.
	FieldValue(ctx context.Context, campaignID, entityID, key string) (value any, ok bool, err error)
	// SetField writes one sheet field back, as the owner.
	SetField(ctx context.Context, campaignID, entityID, key string, value any, ownerID string) error
	// RestorePage brings a removed page back from the Trash.
	RestorePage(ctx context.Context, campaignID, entityID string) error
	// TextDiff names the words a page's text lost and gained between two
	// plain-text versions, each run joined with " … ".
	TextDiff(older, newer string) (removed, added string)
}

// foldWindow is how long successive saves of the same thing by the same
// person count as one change, so a player typing into a page leaves one
// line, not one per autosave.
const foldWindow = 10 * time.Minute

// Service records and searches what players changed.
type Service interface {
	// Record keeps one change. Errors are logged by the caller and never
	// fail the write the change describes.
	Record(ctx context.Context, c Change) error
	// Search answers a query. withPacked also unpacks older months.
	Search(ctx context.Context, campaignID string, q Query, withPacked bool) (*Result, error)
	// Suggestions are the tags the search box offers.
	Suggestions(ctx context.Context, campaignID string) ([]Suggestion, error)
	// Undo puts a change back, as the owner, and marks it undone.
	Undo(ctx context.Context, campaignID, ownerID string, id int64) (*Change, error)
	// PackDue packs live rows older than LiveDays and removes packs whose
	// time is up.
	PackDue(ctx context.Context) error
	// StartPacker runs PackDue now and then every hour until ctx ends.
	StartPacker(ctx context.Context)
	// KeptPages lists the pages players removed that the campaign Trash
	// keeps past its days (Long-term trash).
	KeptPages(ctx context.Context) ([]string, error)
	// MonthlyBytes estimates one packed month's size, for the Site admin
	// setting.
	MonthlyBytes(ctx context.Context) (int64, bool, error)
}

type service struct {
	repo  Repository
	pages Pages
	// keepMonths is the site's Long-term trash length; nil means the default.
	keepMonths func(ctx context.Context) int
	now        func() time.Time
}

// NewService creates the service. keepMonths may be nil.
func NewService(repo Repository, pages Pages, keepMonths func(ctx context.Context) int) Service {
	return &service{repo: repo, pages: pages, keepMonths: keepMonths, now: time.Now}
}

func (s *service) Record(ctx context.Context, c Change) error {
	if c.CampaignID == "" || c.EntityID == "" || c.UserID == "" {
		return nil
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = s.now().UTC()
	}
	if len(c.OldJSON) > maxJSONBytes || len(c.NewJSON) > maxJSONBytes {
		c.OldJSON, c.NewJSON = "", ""
	}
	if c.Kind == KindField || c.Kind == KindText {
		folded, err := s.fold(ctx, c)
		if folded || err != nil {
			return err
		}
	}
	c.OldText, c.NewText = clip(c.OldText, maxValueChars), clip(c.NewText, maxValueChars)
	return s.repo.Insert(ctx, c)
}

// fold merges c into the same person's change to the same thing a moment
// ago: the older value stays, the newer one replaces it. A fold that comes
// back to where it started removes the line. A page text whose versions
// were too large to keep is never folded, since its words could not be
// worked out again.
func (s *service) fold(ctx context.Context, c Change) (bool, error) {
	prev, err := s.repo.LastFor(ctx, c, c.CreatedAt.Add(-foldWindow))
	if err != nil || prev == nil {
		return false, err
	}
	body := c.Kind == KindText && c.FieldKey != "name"
	if body && (prev.OldJSON == "" || c.NewJSON == "") {
		return false, nil
	}
	prev.PageName, prev.App, prev.CreatedAt = c.PageName, c.App, c.CreatedAt
	prev.NewJSON, prev.NewText = c.NewJSON, c.NewText
	if prev.OldJSON == "" || c.NewJSON == "" {
		// One side was too large to keep, so the value can no longer be put back.
		prev.OldJSON, prev.NewJSON = "", ""
	}
	if body {
		prev.OldText, prev.NewText = s.pages.TextDiff(prev.OldJSON, prev.NewJSON)
	}
	prev.OldText, prev.NewText = clip(prev.OldText, maxValueChars), clip(prev.NewText, maxValueChars)
	if prev.OldJSON != "" && prev.OldJSON == prev.NewJSON {
		return true, s.repo.Delete(ctx, prev.CampaignID, prev.ID)
	}
	return true, s.repo.UpdateNew(ctx, *prev)
}

func (s *service) Search(ctx context.Context, campaignID string, q Query, withPacked bool) (*Result, error) {
	live, err := s.repo.Live(ctx, campaignID, q)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	for _, c := range live {
		if q.matches(c) {
			res.Changes = append(res.Changes, c)
		}
	}
	if withPacked {
		packs, err := s.repo.Packs(ctx, campaignID, q.Since)
		if err != nil {
			return nil, err
		}
		res.Unpacked = len(packs)
		for _, p := range packs {
			for _, c := range p.Rows {
				if q.matches(c) {
					res.Changes = append(res.Changes, c)
				}
			}
		}
	}
	sort.SliceStable(res.Changes, func(i, j int) bool { return res.Changes[i].CreatedAt.After(res.Changes[j].CreatedAt) })
	if len(res.Changes) > MaxResults {
		res.Changes, res.Truncated = res.Changes[:MaxResults], true
	}
	return res, nil
}

func (s *service) Suggestions(ctx context.Context, campaignID string) ([]Suggestion, error) {
	pages, fields, err := s.repo.Recent(ctx, campaignID, s.now().AddDate(0, 0, -LiveDays))
	if err != nil {
		return nil, err
	}
	out := append(pages, fields...)
	return append(out, Suggestion{Kind: "ai", Value: "1", Label: "by an AI"}, Suggestion{Kind: "removed", Value: "1", Label: "removed things"}), nil
}

// ErrPageGone is what Undo says when the change's page no longer exists.
var ErrPageGone = apperror.NewNotFound("that page is gone, so the change can't be undone")

// Undo refusals, in the owner's words.
var (
	errUndone    = apperror.NewConflict("that change was already undone")
	errNoUndo    = apperror.NewBadRequest("that change is too large to undo from here; open the page's history instead")
	errTextUndo  = apperror.NewBadRequest("a change to a page's text is undone from the page's history")
	errNotFound  = apperror.NewNotFound("that change is no longer kept")
	errNoPages   = apperror.NewBadRequest("undoing isn't available on this Chronicle site")
	errPageGone  = ErrPageGone
	errDeepTrash = apperror.NewNotFound("that page is no longer in the Trash")
)

func (s *service) Undo(ctx context.Context, campaignID, ownerID string, id int64) (*Change, error) {
	if s.pages == nil {
		return nil, errNoPages
	}
	c, pk, err := s.find(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if c.UndoneAt != nil {
		return nil, errUndone
	}
	switch c.Kind {
	case KindText:
		return nil, errTextUndo
	case KindRemoved:
		if err := s.pages.RestorePage(ctx, campaignID, c.EntityID); err != nil {
			var ae *apperror.AppError
			if errors.As(err, &ae) && ae.Code == 404 {
				return nil, errDeepTrash
			}
			return nil, err
		}
		if err := s.repo.ForgetPage(ctx, campaignID, c.EntityID); err != nil {
			slog.Warn("player changes: forgetting a restored page", slog.Any("error", err))
		}
	case KindField:
		if c.OldJSON == "" || c.NewJSON == "" {
			return nil, errNoUndo
		}
		now, ok, err := s.pages.FieldValue(ctx, campaignID, c.EntityID, c.FieldKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errPageGone
		}
		if !sameJSON(now, c.NewJSON) {
			return nil, apperror.NewConflict(c.What() + " has changed since, so Undo would lose that. It now says " + quoteValue(textOf(now)) + ".")
		}
		var old any
		if err := json.Unmarshal([]byte(c.OldJSON), &old); err != nil {
			return nil, errNoUndo
		}
		if err := s.pages.SetField(ctx, campaignID, c.EntityID, c.FieldKey, old, ownerID); err != nil {
			return nil, err
		}
	default:
		return nil, errNotFound
	}
	at := s.now().UTC()
	if pk == nil {
		if _, err := s.repo.MarkUndone(ctx, campaignID, id, ownerID, at); err != nil {
			return nil, err
		}
	} else if err := s.markPacked(ctx, *pk, id, ownerID, at); err != nil {
		return nil, err
	}
	c.UndoneAt, c.UndoneBy = &at, ownerID
	return c, nil
}

// find reads a change from the live rows or, failing that, the packs. pk is
// the pack it came from.
func (s *service) find(ctx context.Context, campaignID string, id int64) (*Change, *PackKey, error) {
	c, err := s.repo.Get(ctx, campaignID, id)
	if err != nil || c != nil {
		return c, nil, err
	}
	packs, err := s.repo.Packs(ctx, campaignID, time.Time{})
	if err != nil {
		return nil, nil, err
	}
	for _, p := range packs {
		for i := range p.Rows {
			if p.Rows[i].ID == id {
				k := p.PackKey
				return &p.Rows[i], &k, nil
			}
		}
	}
	return nil, nil, errNotFound
}

func (s *service) markPacked(ctx context.Context, k PackKey, id int64, by string, at time.Time) error {
	packs, err := s.repo.Packs(ctx, k.CampaignID, k.Month)
	if err != nil {
		return err
	}
	for _, p := range packs {
		if !p.Month.Equal(k.Month) {
			continue
		}
		for i := range p.Rows {
			if p.Rows[i].ID == id {
				p.Rows[i].UndoneAt, p.Rows[i].UndoneBy = &at, by
			}
		}
		return s.repo.RewritePack(ctx, k, p.Rows)
	}
	return errNotFound
}

func (s *service) months(ctx context.Context) int {
	if s.keepMonths == nil {
		return DefaultKeepMonths
	}
	return s.keepMonths(ctx)
}

func (s *service) PackDue(ctx context.Context) error {
	now := s.now().UTC()
	cutoff := now.AddDate(0, 0, -LiveDays)
	keys, err := s.repo.DueMonths(ctx, cutoff)
	if err != nil {
		return err
	}
	var keep time.Time
	if m := s.months(ctx); m > 0 {
		keep = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, m, 0)
	}
	for _, k := range keys {
		if _, err := s.repo.PackMonth(ctx, k, cutoff, keep); err != nil {
			return err
		}
	}
	_, err = s.repo.PrunePacks(ctx, now)
	return err
}

func (s *service) KeptPages(ctx context.Context) ([]string, error) {
	return s.repo.KeptPages(ctx, s.now())
}

func (s *service) MonthlyBytes(ctx context.Context) (int64, bool, error) {
	return s.repo.MonthlyBytes(ctx)
}

func (s *service) StartPacker(ctx context.Context) {
	run := func() {
		if err := s.PackDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("player changes: packing", slog.Any("error", err))
		}
	}
	run()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// sameJSON reports whether a stored value equals the one a change wrote,
// compared as JSON so 3 and 3.0 or key order don't matter.
func sameJSON(v any, raw string) bool {
	a, err := json.Marshal(v)
	if err != nil {
		return false
	}
	var w any
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return false
	}
	b, err := json.Marshal(w)
	return err == nil && bytes.Equal(a, b)
}

// ValueJSON is a field value as Undo stores it; "" when it can't be kept.
func ValueJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// textOf is a field value as the page shows it.
func textOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case []any, map[string]any:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		b, _ := json.Marshal(x)
		return strings.Trim(string(b), `"`)
	}
}

// TextOf is textOf for the recorder in internal/app.
func TextOf(v any) string { return textOf(v) }

func quoteValue(s string) string {
	if s == "" {
		return "nothing"
	}
	// Straight quotes: the message reaches the page in a response header,
	// which carries only plain ASCII punctuation safely.
	return `"` + clip(s, 80) + `"`
}

// request_undo.go is Undo on an applied request in "What it did": what is kept
// when an AI's request is applied, what Undo checks before it puts anything
// back, and how each kind of change is put back.
//
// Undo never overwrites later work. Each kept change carries a fingerprint of
// what the request left; if the live thing no longer matches, nothing is put
// back and the owner is told what changed.

package ai_workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// kindPage names a page's before in ai_request_befores; every other kind uses
// its record kind's name.
const kindPage = "page"

// PageUndo is how Undo reaches pages: read one as it is now, put a title and
// text back through the page history's restore, and move pages to and from
// the Trash. It is the entities service's, so a put-back is versioned and
// broadcast like any other save. Nil leaves pages without Undo.
type PageUndo interface {
	// Read is the live page, or nil when it is gone or in the Trash.
	Read(ctx context.Context, campaignID, id string) (*entities.Entity, error)
	RestoreText(ctx context.Context, id, name string, entry, entryHTML *string) error
	// Trash removes a page the way a delete does: to the Trash, not for good.
	Trash(ctx context.Context, id string) error
	RestoreFromTrash(ctx context.Context, campaignID, id string) error
}

// SetPageUndo wires Undo for pages. Optional.
func (h *Handler) SetPageUndo(p PageUndo) { h.pageUndo = p }

// pageBefore is a page's kept title and text.
type pageBeforeJSON struct {
	Name      string  `json:"name"`
	Entry     *string `json:"entry"`
	EntryHTML *string `json:"entryHtml"`
}

// keptBefores turns what an Apply kept into rows for the connector's store.
func keptBefores(result importer.CommitResult) []connector.Before {
	var out []connector.Before
	for _, r := range result.Rows {
		switch {
		case r.Before != nil:
			b := connector.Before{Kind: kindPage, TargetID: r.Before.EntityID, TargetName: r.Before.Name,
				Action: r.Before.Action, AfterHash: r.Before.AfterHash}
			if r.Before.Action == importer.ActionUpdate {
				js, err := json.Marshal(pageBeforeJSON{Name: r.Before.Name, Entry: r.Before.Entry, EntryHTML: r.Before.EntryHTML})
				if err != nil {
					continue
				}
				s := string(js)
				b.BeforeJSON = &s
			}
			out = append(out, b)
		case r.Kept != nil:
			s := r.Kept.Snapshot.Data
			out = append(out, connector.Before{Kind: r.Kept.Kind, TargetID: r.Kept.Snapshot.Target,
				TargetName: r.Kept.Snapshot.Name, Action: r.Kept.Action, BeforeJSON: &s, AfterHash: r.Kept.AfterHash})
		}
	}
	return out
}

// undoCell is the Undo part of one applied request's row.
type undoCell struct {
	CampaignID string
	RequestID  int64
	// Confirm says what Undo will put back.
	Confirm  string
	UndoneAt *time.Time
	// Refusal is why the last press changed nothing.
	Refusal string
	Now     time.Time
	Loc     *time.Location
}

// Can reports whether the row offers Undo.
func (u undoCell) Can() bool { return u.UndoneAt == nil && u.Confirm != "" }

// undoneText is "Undone today 2:05 pm", with the day when it was not today.
func (u undoCell) undoneText() string {
	t := u.UndoneAt.In(u.Loc)
	clock := strings.ToLower(t.Format("3:04 PM"))
	now := u.Now.In(u.Loc)
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return "Undone today " + clock
	}
	return "Undone " + t.Format("2 January") + " " + clock
}

// undoConfirm is the plain line before Undo: what is put back and what is
// not. blocks is how many changes the request had.
func undoConfirm(left []connector.Before, blocks int) string {
	var upd, created, removed, other int
	var names []string
	for _, b := range left {
		switch {
		case b.Kind == kindPage && b.Action == importer.ActionUpdate:
			upd++
		case b.Kind == kindPage && b.Action == importer.ActionCreate:
			created++
		case b.Kind == kindPage:
			removed++
		default:
			other++
			names = append(names, b.TargetName)
		}
	}
	var parts []string
	if upd > 0 {
		parts = append(parts, fmt.Sprintf("Puts back %s as %s before.", plural(upd, "page"), thatWere(upd)))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("Brings back %s from the Trash.", plural(removed, "page")))
	}
	if other > 0 {
		sort.Strings(names)
		parts = append(parts, "Puts back "+joinNames(names)+" as it was before.")
	}
	if created > 0 {
		verb := "go"
		if created == 1 {
			verb = "goes"
		}
		parts = append(parts, fmt.Sprintf("%s the AI created %s to the Trash.", plural(created, "page"), verb))
	}
	if len(left) < blocks {
		parts = append(parts, "The request's other changes stay as they are.")
	}
	return strings.Join(parts, " ")
}

func thatWere(n int) string {
	if n == 1 {
		return "it was"
	}
	return "they were"
}

func joinNames(names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// fillUndo reads the Undo state of the owner's applied requests onto the view.
func (h *Handler) fillUndo(ctx context.Context, cc *campaigns.CampaignContext, userID string, v *ConnectView) error {
	if h.connector == nil || !h.connector.CanUndo() {
		return nil
	}
	applied, err := h.connector.Applied(ctx, cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	ids := make([]int64, len(applied))
	for i, q := range applied {
		ids[i] = q.ID
	}
	sums, err := h.connector.UndoSummaries(ctx, cc.Campaign.ID, userID, ids)
	if err != nil {
		return err
	}
	v.Applied = applied
	v.Undo = map[int64]undoCell{}
	for _, q := range applied {
		s := sums[q.ID]
		if len(s.Left) == 0 && s.UndoneAt == nil {
			continue
		}
		v.Undo[q.ID] = undoCell{CampaignID: cc.Campaign.ID, RequestID: q.ID, UndoneAt: s.UndoneAt,
			Confirm: undoConfirm(s.Left, q.Blocks), Now: v.Now, Loc: v.Loc}
	}
	return nil
}

// appliedFor is the applied request a logged ask_for_changes call made: sent
// within a minute of it, quoting the same line. The nearest wins.
func appliedFor(v ConnectView, c connector.Call) (int64, bool) {
	var best int64
	var bestGap time.Duration
	for _, q := range v.Applied {
		d := q.CreatedAt.Sub(c.At)
		if d < 0 {
			d = -d
		}
		if d > time.Minute || (q.Summary != "" && !strings.Contains(c.Summary, q.Summary)) {
			continue
		}
		if best == 0 || d < bestGap {
			best, bestGap = q.ID, d
		}
	}
	return best, best != 0
}

// UndoRequest puts back what an applied request changed.
//
// POST /campaigns/:id/ai/requests/:rid/undo
//
// The request is read through the owner's own key, so it can only be one of
// their own, and the answer is only the row's status: never the kept text.
func (h *Handler) UndoRequest(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil || !h.connector.CanUndo() {
		return apperror.NewBadRequest("Undo is not available on this Chronicle site")
	}
	id, err := strconv.ParseInt(c.Param("rid"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("that request isn't here any more")
	}
	ctx, userID := c.Request().Context(), auth.GetUserID(c)
	q, left, err := h.connector.ClaimUndo(ctx, cc.Campaign.ID, userID, id)
	if err != nil {
		return err
	}
	actor := actorFor(c, cc)
	rctx := entities.WithActor(ctx, userID)

	refusal := h.undoRefusal(ctx, cc.Campaign.ID, actor, left)
	restored := 0
	if refusal == "" {
		for _, b := range left {
			if err := h.restoreBefore(rctx, cc.Campaign.ID, actor, b); err != nil {
				slog.Warn("ai-workspace: undo step failed", slog.Int64("request_id", id), slog.String("kind", b.Kind), slog.Any("error", err))
				refusal = fmt.Sprintf("Put back %d of %d, then couldn't put back “%s”: %s. Press Undo again to try the rest.",
					restored, len(left), b.TargetName, strings.TrimRight(recordError(err), ". "))
				break
			}
			if err := h.connector.BeforeUndone(ctx, b.ID, userID); err != nil {
				slog.Error("ai-workspace: marking a change undone", slog.Any("error", err))
			}
			restored++
		}
	}
	if refusal != "" {
		// Nothing more will be put back by this press, so the claim is
		// given up and the owner may press again.
		h.connector.ReleaseUndo(ctx, cc.Campaign.ID, userID, id)
	}
	if h.audit != nil && restored > 0 {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, userID, "campaign.ai_request.undone",
			map[string]any{"request_id": id, "restored": restored, "kept": len(left)})
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	cell, ok := v.Undo[q.ID]
	if !ok {
		cell = undoCell{CampaignID: cc.Campaign.ID, RequestID: q.ID, Now: v.Now, Loc: v.Loc}
		if refusal == "" {
			now := time.Now()
			cell.UndoneAt = &now
		}
	}
	cell.Refusal = refusal
	return middleware.Render(c, http.StatusOK, aiUndoCell(cell))
}

// undoRefusal checks every change against the live state before any is put
// back, so a request is undone whole or not at all. It names what changed
// since, in the owner's own terms.
func (h *Handler) undoRefusal(ctx context.Context, campaignID string, actor aiplug.Actor, left []connector.Before) string {
	var changed, gone []string
	for _, b := range left {
		switch ok, why := h.stillAsLeft(ctx, campaignID, actor, b); {
		case ok:
		case why == "gone":
			gone = append(gone, b.TargetName)
		default:
			changed = append(changed, b.TargetName)
		}
	}
	var parts []string
	if len(changed) > 0 {
		parts = append(parts, joinNames(quoted(changed))+" "+haveHas(len(changed))+" been changed since the AI applied it.")
	}
	if len(gone) > 0 {
		parts = append(parts, joinNames(quoted(gone))+" can't be found or brought back any more.")
	}
	if len(parts) == 0 {
		return ""
	}
	return "Nothing was put back. " + strings.Join(parts, " ") +
		" Undo won't overwrite later work; for a page, open its History to choose an earlier version."
}

func haveHas(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

func quoted(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "“" + n + "”"
	}
	return out
}

// stillAsLeft reports whether a change is still as the request left it. why
// is "gone" when the thing can't be found at all.
func (h *Handler) stillAsLeft(ctx context.Context, campaignID string, actor aiplug.Actor, b connector.Before) (ok bool, why string) {
	if b.Kind == kindPage {
		if h.pageUndo == nil {
			return false, "gone"
		}
		if b.Action == importer.ActionDelete {
			// Nothing is compared: the page is still in the Trash or it is not,
			// which restoring will say.
			return true, ""
		}
		e, err := h.pageUndo.Read(ctx, campaignID, b.TargetID)
		if err != nil || e == nil || e.CampaignID != campaignID {
			// A page AI made that is already gone has nothing left to undo.
			return b.Action == importer.ActionCreate, "gone"
		}
		return importer.PageFingerprint(e) == b.AfterHash, "changed"
	}
	u, k := h.undoKind(b.Kind)
	if k == nil {
		return false, "gone"
	}
	hash, err := u.Fingerprint(ctx, campaignID, actor, snapshotOf(b))
	if err != nil {
		return false, "gone"
	}
	return hash == b.AfterHash, "changed"
}

func snapshotOf(b connector.Before) aiplug.Snapshot {
	s := aiplug.Snapshot{Target: b.TargetID, Name: b.TargetName}
	if b.BeforeJSON != nil {
		s.Data = *b.BeforeJSON
	}
	return s
}

func (h *Handler) undoKind(name string) (aiplug.Undoable, aiplug.Kind) {
	if h.records == nil {
		return nil, nil
	}
	k, ok := h.records.Get(name)
	if !ok {
		return nil, nil
	}
	u, ok := k.(aiplug.Undoable)
	if !ok {
		return nil, nil
	}
	return u, k
}

// restoreBefore puts one change back through the service that owns it.
func (h *Handler) restoreBefore(ctx context.Context, campaignID string, actor aiplug.Actor, b connector.Before) error {
	if b.Kind != kindPage {
		u, k := h.undoKind(b.Kind)
		if k == nil {
			return apperror.NewBadRequest("this kind of change can't be put back on this server")
		}
		return u.Restore(ctx, campaignID, actor, snapshotOf(b))
	}
	if h.pageUndo == nil {
		return apperror.NewBadRequest("pages can't be put back on this server")
	}
	switch b.Action {
	case importer.ActionCreate:
		if e, err := h.pageUndo.Read(ctx, campaignID, b.TargetID); err != nil || e == nil {
			return nil
		}
		return h.pageUndo.Trash(ctx, b.TargetID)
	case importer.ActionDelete:
		return h.pageUndo.RestoreFromTrash(ctx, campaignID, b.TargetID)
	}
	if b.BeforeJSON == nil {
		return apperror.NewBadRequest("nothing was kept for this page")
	}
	var p pageBeforeJSON
	if err := json.Unmarshal([]byte(*b.BeforeJSON), &p); err != nil {
		return err
	}
	return h.pageUndo.RestoreText(ctx, b.TargetID, p.Name, p.Entry, p.EntryHTML)
}

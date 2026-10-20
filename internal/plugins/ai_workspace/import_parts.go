package ai_workspace

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// An AI that splits its answer over several pastes labels each one with a
// `kind: part` block (importer.Part). A part pasted before the whole answer is
// in waits in the tray, shown under the paste box; when the last part lands,
// every part is put in order and reviewed as one paste. Without an
// ImportMemory the labels are still read, but nothing waits.

// SetImportMemory wires what AI Import keeps per campaign. Optional.
func (h *Handler) SetImportMemory(m ImportMemory) { h.memory = m }

// partNote is one labelled part of the reviewed paste.
type partNote struct {
	Part importer.Part
	// Missing are the parts it needs that are neither applied in the
	// campaign nor earlier in this paste.
	Missing []int
	// Known reports whether Chronicle can tell which parts were applied.
	Known bool
}

// MissingWords is "part 2" or "parts 1 and 2".
func (n partNote) MissingWords() string { return importer.PartList(n.Missing) }

// Heading warns that a part reviewed early lacks what it needs.
func (n partNote) Heading() string {
	return fmt.Sprintf("%s needs %s, which isn't applied or in this review, so rows that rely on it may fail.",
		capitalize(n.Part.Title()), n.MissingWords())
}

// Line is the quiet line for a part with nothing missing.
func (n partNote) Line() string {
	if n.Part.Of > 0 {
		return fmt.Sprintf("%q, part %d of %d.", n.Part.Name, n.Part.Number, n.Part.Of)
	}
	return fmt.Sprintf("%q, part %d.", n.Part.Name, n.Part.Number)
}

// appliedParts reads one answer's stored parts: which were applied, and which
// wait in the tray. ok is false when that can't be read.
func (h *Handler) appliedParts(ctx context.Context, campaignID, key string) (map[int]bool, []importer.StoredTray, bool) {
	if h.memory == nil {
		return nil, nil, false
	}
	stored, err := h.memory.Parts(ctx, campaignID, key)
	if err != nil {
		slog.Warn("ai-workspace: answer parts unreadable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil, nil, false
	}
	applied := map[int]bool{}
	var tray []importer.StoredTray
	for _, p := range stored {
		if p.Applied {
			applied[p.Number] = true
		}
		if p.Markdown != "" {
			tray = append(tray, importer.StoredTray{Part: storedToPart(p), Markdown: p.Markdown})
		}
	}
	return applied, tray, true
}

func storedToPart(p StoredPart) importer.Part {
	return importer.Part{Name: p.AnswerName, Key: p.AnswerKey, Number: p.Number, Of: p.Of, Needs: p.Needs}
}

// checkParts notes, for each labelled part being reviewed, the parts it needs
// that are neither applied nor earlier in the paste.
func (h *Handler) checkParts(ctx context.Context, campaignID string, lab importer.Labelled) []partNote {
	notes := make([]partNote, len(lab.Parts))
	inPaste := map[string]map[int]bool{}
	applied := map[string]map[int]bool{}
	known := map[string]bool{}
	for i, p := range lab.Parts {
		if inPaste[p.Key] == nil {
			inPaste[p.Key] = map[int]bool{}
			applied[p.Key], _, known[p.Key] = h.appliedParts(ctx, campaignID, p.Key)
		}
		notes[i] = partNote{Part: p, Known: known[p.Key]}
		if notes[i].Known {
			notes[i].Missing = importer.MissingNeeds(p, applied[p.Key], inPaste[p.Key])
		}
		inPaste[p.Key][p.Number] = true
	}
	return notes
}

// trayNotice tells the operator a pasted part went to the tray.
type trayNotice struct {
	Name    string
	Pasted  []int
	In      int // parts in the tray or applied
	Of      int // how many the AI said there are; 0 when it didn't say
	Missing []int
}

// Heading is the notice's title.
func (n trayNotice) Heading() string {
	pasted := capitalize(importer.PartList(n.Pasted))
	if n.Of > 0 {
		return fmt.Sprintf("%s of %q went to the tray: %d of %d in.", pasted, n.Name, n.In, n.Of)
	}
	return fmt.Sprintf("%s of %q went to the tray.", pasted, n.Name)
}

// Explain says what happens next.
func (n trayNotice) Explain() string {
	return fmt.Sprintf("Paste %s next. When the last part lands, Chronicle puts every part in order and opens one review of the whole answer.",
		importer.PartList(n.Missing))
}

// trayStep decides what a labelled paste does. A paste of one answer whose
// parts are not all in yet goes to the tray and gets a notice back. Once the
// answer is complete, the parts waiting in the tray join this paste, in
// order, and the whole is reviewed. Anything else is reviewed as it is.
func (h *Handler) trayStep(ctx context.Context, campaignID, userID, body string, all []importer.ParsedPage) (string, []importer.ParsedPage, *trayNotice, error) {
	lab := importer.TakeParts(all)
	if h.memory == nil || len(lab.Parts) == 0 {
		return body, all, nil, nil
	}
	key := lab.Parts[0].Key
	for _, p := range lab.Parts[1:] {
		if p.Key != key {
			return body, all, nil, nil // several answers at once: review as pasted
		}
	}
	applied, tray, ok := h.appliedParts(ctx, campaignID, key)
	if !ok {
		return body, all, nil, nil
	}
	pasted := map[int]bool{}
	for _, p := range lab.Parts {
		pasted[p.Number] = true
	}
	present := append([]importer.Part(nil), lab.Parts...)
	var waiting []string
	for _, t := range tray {
		if !pasted[t.Part.Number] {
			present = append(present, t.Part)
			waiting = append(waiting, t.Markdown)
		}
	}
	if missing := importer.AnswerMissing(present, applied); len(missing) > 0 {
		texts := importer.PartTexts(all)
		nums := make([]int, len(lab.Parts))
		for i, p := range lab.Parts {
			nums[i] = p.Number
			if err := h.memory.PutInTray(ctx, campaignID, StoredPart{
				AnswerKey: p.Key, AnswerName: p.Name, Number: p.Number, Of: p.Of,
				Needs: p.Needs, Markdown: texts[i], AddedBy: userID,
			}); err != nil {
				return "", nil, nil, err
			}
		}
		in := map[int]bool{}
		of := 0
		for _, p := range present {
			in[p.Number] = true
			of = max(of, p.Of)
		}
		for n := range applied {
			in[n] = true
		}
		return "", nil, &trayNotice{Name: lab.Parts[0].Name, Pasted: nums, In: len(in), Of: of, Missing: missing}, nil
	}
	if len(waiting) == 0 {
		return body, all, nil, nil
	}
	combined := strings.Join(append(waiting, body), "\n\n")
	if len(combined) > importBodyCap {
		return "", nil, nil, apperror.NewBadRequest("The parts of this answer add up to more than the 5 MB import limit. Empty the tray and ask your AI for fewer, smaller parts.")
	}
	parsed := importer.Parse(combined)
	if ordered, ok := importer.OrderParts(parsed); ok {
		combined, parsed = ordered, importer.Parse(ordered)
	}
	return combined, parsed, nil, nil
}

// trayText is every part of one answer waiting in the tray, in order, for
// reviewing what is there before the answer is complete.
func (h *Handler) trayText(ctx context.Context, campaignID, key string) (string, error) {
	_, tray, ok := h.appliedParts(ctx, campaignID, key)
	if !ok {
		return "", apperror.NewBadRequest("The tray could not be read just now. Try again in a moment.")
	}
	sort.SliceStable(tray, func(i, j int) bool { return tray[i].Part.Number < tray[j].Part.Number })
	texts := make([]string, len(tray))
	for i, t := range tray {
		texts[i] = t.Markdown
	}
	return strings.Join(texts, "\n\n"), nil
}

// trayAnswer is one answer waiting in the tray, as the paste box lists it.
type trayAnswer struct {
	Key     string
	Name    string
	Parts   []int
	In      int
	Of      int
	Missing []int
}

// Summary is the answer's line: what is in and what is still to paste.
func (a trayAnswer) Summary() string {
	s := capitalize(importer.PartList(a.Parts)) + " waiting"
	if a.Of > 0 {
		s = fmt.Sprintf("%d of %d in", a.In, a.Of)
	}
	if len(a.Missing) > 0 {
		s += ". Still to paste: " + importer.PartList(a.Missing) + "."
	}
	return s
}

// trayAnswers lists what waits in the campaign's tray, grouped by answer.
func (h *Handler) trayAnswers(ctx context.Context, campaignID string) []trayAnswer {
	if h.memory == nil {
		return nil
	}
	rows, err := h.memory.Tray(ctx, campaignID)
	if err != nil {
		slog.Warn("ai-workspace: tray unreadable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	var out []trayAnswer
	at := map[string]int{}
	present := map[string][]importer.Part{}
	for _, r := range rows {
		i, ok := at[r.AnswerKey]
		if !ok {
			i = len(out)
			at[r.AnswerKey] = i
			out = append(out, trayAnswer{Key: r.AnswerKey, Name: r.AnswerName})
		}
		out[i].Parts = append(out[i].Parts, r.Number)
		out[i].Of = max(out[i].Of, r.Of)
		present[r.AnswerKey] = append(present[r.AnswerKey], storedToPart(r))
	}
	for i := range out {
		applied, _, _ := h.appliedParts(ctx, campaignID, out[i].Key)
		sort.Ints(out[i].Parts)
		out[i].Missing = importer.AnswerMissing(present[out[i].Key], applied)
		in := map[int]bool{}
		for _, n := range out[i].Parts {
			in[n] = true
		}
		for n := range applied {
			in[n] = true
		}
		out[i].In = len(in)
	}
	return out
}

// markApplied records the labelled parts of a committed paste as applied,
// which also takes them out of the tray.
func (h *Handler) markApplied(ctx context.Context, campaignID, userID string, lab importer.Labelled, done map[int]bool) {
	if h.memory == nil {
		return
	}
	// A part counts as applied only once one of its blocks was, so a review
	// answered No throughout leaves its parts waiting in the tray.
	used := map[int]bool{}
	for src := range done {
		if pi, ok := lab.PartOf(src); ok {
			used[pi] = true
		}
	}
	for i, p := range lab.Parts {
		if !used[i] {
			continue
		}
		if err := h.memory.MarkApplied(ctx, campaignID, StoredPart{
			AnswerKey: p.Key, AnswerName: p.Name, Number: p.Number, Of: p.Of, Needs: p.Needs, AddedBy: userID,
		}); err != nil {
			slog.Warn("ai-workspace: applied part not recorded",
				slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
	}
}

// repeatedBlocks finds the blocks of a paste already applied in the campaign,
// by fingerprint, and says when, by SourceIndex. Pasting the same answer twice
// would otherwise make every page and event again.
func (h *Handler) repeatedBlocks(ctx context.Context, campaignID string, blocks []importer.ParsedPage) map[int]string {
	if h.memory == nil || len(blocks) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Status != importer.StatusParseError {
			hashes = append(hashes, importer.BlockHash(b.Raw))
		}
	}
	seen, err := h.memory.Seen(ctx, campaignID, hashes)
	if err != nil {
		slog.Warn("ai-workspace: applied blocks unreadable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	out := map[int]string{}
	for _, b := range blocks {
		if at, ok := seen[importer.BlockHash(b.Raw)]; ok && b.Status != importer.StatusParseError {
			out[b.SourceIndex] = at.UTC().Format("2 January 2006")
		}
	}
	return out
}

// rememberApplied records the fingerprint of every block the commit applied
// and returns their SourceIndexes. Page rows come first in the result,
// records after.
func (h *Handler) rememberApplied(ctx context.Context, campaignID string, result importer.CommitResult, pageRows int, pages, recs []importer.ParsedPage) map[int]bool {
	if h.memory == nil {
		return nil
	}
	var hashes []string
	done := map[int]bool{}
	add := func(row int, b importer.ParsedPage) {
		if row >= len(result.Rows) {
			return
		}
		switch result.Rows[row].Status {
		case importer.StatusCreated, importer.StatusRenamed, importer.StatusUpdated, importer.StatusDeleted:
			hashes = append(hashes, importer.BlockHash(b.Raw))
			done[b.SourceIndex] = true
		}
	}
	for i, p := range pages {
		if i < pageRows {
			add(i, p)
		}
	}
	for i, r := range recs {
		add(pageRows+i, r)
	}
	if err := h.memory.Remember(ctx, campaignID, hashes); err != nil {
		slog.Warn("ai-workspace: applied blocks not remembered",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	return done
}

// ImportTrayList renders the tray under the paste box.
//
// GET /campaigns/:id/ai-workspace/import/tray
func (h *Handler) ImportTrayList(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return middleware.Render(c, http.StatusOK, ImportTray(cc.Campaign.ID, h.trayAnswers(c.Request().Context(), cc.Campaign.ID), false))
}

// ClearImportTray takes one answer's waiting parts out of the tray. Nothing
// in the campaign changes; only the waiting text goes.
//
// POST /campaigns/:id/ai-workspace/import/tray/clear
func (h *Handler) ClearImportTray(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if key := strings.TrimSpace(c.FormValue("answer")); key != "" && h.memory != nil {
		if err := h.memory.ClearTray(c.Request().Context(), cc.Campaign.ID, key); err != nil {
			return err
		}
	}
	return middleware.Render(c, http.StatusOK, ImportTray(cc.Campaign.ID, h.trayAnswers(c.Request().Context(), cc.Campaign.ID), false))
}

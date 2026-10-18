package entities

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Two field types read more than their stored value: a link field stores a
// page id and shows that page's name, and a rule field shows what its
// homebrew rule gives unless someone typed a value by hand.
const (
	FieldTypeLink = "link"
	FieldTypeRule = "rule"
)

// FieldWords is what pages need from the homebrew rules and local words,
// which belong to the fieldrules plugin. It is wired in internal/app; unset,
// rule fields show only values typed by hand and no word is renamed.
type FieldWords interface {
	// Evaluate gives, per rule id, what the rule gives for these values.
	Evaluate(ctx context.Context, campaignID string, ruleIDs []int, data map[string]any) map[int]string
	// WordsFor is the local words this viewer sees on one page, keyed by the
	// lower-cased usual word.
	WordsFor(ctx context.Context, campaignID, entityID string, role int, userID string) map[string]string
	// Rename returns the local name for text, if one applies.
	Rename(words map[string]string, text string) (string, bool)
	// RuleExists reports whether the campaign has that rule.
	RuleExists(ctx context.Context, campaignID string, ruleID int) bool
}

// SetFieldWords wires the homebrew rules and local words into the fields API.
func (h *Handler) SetFieldWords(fw FieldWords) { h.fieldWords = fw }

// SetFieldWords lets category saves check that a rule field names a rule.
func (s *entityService) SetFieldWords(fw FieldWords) { s.fieldWords = fw }

// FieldShown is how one field reads to the viewer when that is not simply its
// stored value. Only fields that differ are sent.
type FieldShown struct {
	Text string `json:"text"`
	// Href is a link field's page; empty when the page is gone or hidden.
	Href string `json:"href,omitempty"`
	// Usual is the word a local word replaced, for a hover.
	Usual string `json:"usual,omitempty"`
	// ByHand marks a rule field whose value was typed in; RuleGives is what
	// the rule would show instead.
	ByHand    bool   `json:"byHand,omitempty"`
	RuleGives string `json:"ruleGives,omitempty"`
}

// linkedPage is a link field's target as the viewer may see it.
type linkedPage struct {
	Name string
	Href string
}

// shapeFields works out how each field reads: a link's page name, a rule's
// result unless typed by hand, and a local word for a value that has one.
// links holds only pages the viewer may see; a link to any other page shows
// nothing. Pure, so the precedence is pinned by table tests.
func shapeFields(defs []FieldDefinition, data map[string]any, links map[string]linkedPage,
	ruleGives map[int]string, words map[string]string, rename func(map[string]string, string) (string, bool)) map[string]FieldShown {
	out := map[string]FieldShown{}
	local := func(sh FieldShown) FieldShown {
		if rename == nil {
			return sh
		}
		if l, ok := rename(words, sh.Text); ok && l != sh.Text {
			sh.Usual, sh.Text = sh.Text, l
		}
		return sh
	}
	for _, d := range defs {
		raw, has := data[d.Key]
		text := strings.TrimSpace(fieldText(raw))
		switch d.Type {
		case FieldTypeLink:
			if text == "" {
				continue
			}
			if p, ok := links[text]; ok {
				out[d.Key] = FieldShown{Text: p.Name, Href: p.Href}
			} else {
				out[d.Key] = FieldShown{} // gone or hidden: show nothing, never the id
			}
		case FieldTypeRule:
			gives := ruleGives[d.RuleID]
			sh := FieldShown{Text: gives}
			if has && text != "" {
				sh = FieldShown{Text: text, ByHand: true}
				if gives != "" && !strings.EqualFold(gives, text) {
					sh.RuleGives = gives
				}
			}
			if sh.Text == "" {
				continue
			}
			out[d.Key] = local(sh)
		case "number", "checkbox", "url", "textarea":
			// Numbers, ticks, links and prose are never renamed.
		default:
			if text == "" {
				continue
			}
			if sh := local(FieldShown{Text: text}); sh.Usual != "" {
				out[d.Key] = sh
			}
		}
	}
	return out
}

// fieldText is a stored value as text.
func fieldText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

// restrictedKeys is every field whose value this viewer may not see, by the
// same flags FilterRestrictedFields strips on. A rule field is skipped too
// when restricted: it has no stored value to strip, so it would otherwise be
// worked out and shown anyway.
func restrictedKeys(canSeeGM, isOwner bool, defLists ...[]FieldDefinition) map[string]bool {
	out := map[string]bool{}
	if canSeeGM {
		return out
	}
	for _, defs := range defLists {
		for _, d := range defs {
			if d.GMOnly || (d.OwnerOnly && !isOwner) {
				out[d.Key] = true
			}
		}
	}
	return out
}

// shownFields is shapeFields with the lookups done: the linked pages this
// viewer may open, the rules' results and the page's local words. It never
// fails; a lookup that errs leaves its fields as stored.
func (h *Handler) shownFields(ctx context.Context, campaignID string, entity *Entity, allDefs []FieldDefinition,
	data map[string]any, hidden map[string]bool, role int, userID string) map[string]FieldShown {
	defs := make([]FieldDefinition, 0, len(allDefs))
	for _, d := range allDefs {
		if !hidden[d.Key] {
			defs = append(defs, d)
		}
	}
	links := map[string]linkedPage{}
	var ruleIDs []int
	hasText := false
	for _, d := range defs {
		switch d.Type {
		case FieldTypeLink:
			id := strings.TrimSpace(fieldText(data[d.Key]))
			if id == "" {
				continue
			}
			if _, done := links[id]; done {
				continue
			}
			if p, ok := h.visibleLink(ctx, campaignID, id, d.LinkTo, role, userID); ok {
				links[id] = p
			}
		case FieldTypeRule:
			ruleIDs = append(ruleIDs, d.RuleID)
		case "number", "checkbox", "url", "textarea":
		default:
			hasText = true
		}
	}
	var gives map[int]string
	var words map[string]string
	var rename func(map[string]string, string) (string, bool)
	if h.fieldWords != nil {
		if len(ruleIDs) > 0 {
			gives = h.fieldWords.Evaluate(ctx, campaignID, ruleIDs, data)
		}
		if hasText || len(ruleIDs) > 0 {
			words = h.fieldWords.WordsFor(ctx, campaignID, entity.ID, role, userID)
			rename = h.fieldWords.Rename
		}
	}
	return shapeFields(defs, data, links, gives, words, rename)
}

// visibleLink is a linked page the viewer may open, in this campaign and the
// field's category. Anything else is treated as no link.
func (h *Handler) visibleLink(ctx context.Context, campaignID, id string, typeID, role int, userID string) (linkedPage, bool) {
	target, err := h.service.GetByID(ctx, id)
	if err != nil || target == nil || target.CampaignID != campaignID {
		return linkedPage{}, false
	}
	if !linkTypeOK(target.EntityTypeID, typeID, func(id int) *EntityType {
		et, _ := h.service.GetEntityTypeByID(ctx, id)
		return et
	}) {
		return linkedPage{}, false
	}
	access, err := h.service.CheckEntityAccess(ctx, target.ID, role, userID)
	if err != nil || access == nil || !access.CanView {
		return linkedPage{}, false
	}
	return linkedPage{Name: target.Name, Href: fmt.Sprintf("/campaigns/%s/entities/%s", campaignID, target.ID)}, true
}

// linkTypeOK reports whether a page of category typeID may fill a link to
// linkTo: that category or one of its sub-categories, the same pages the
// page picker's search offers.
func linkTypeOK(typeID, linkTo int, typeOf func(int) *EntityType) bool {
	if linkTo == 0 || typeID == linkTo {
		return true
	}
	et := typeOf(typeID)
	return et != nil && et.ParentTypeID != nil && *et.ParentTypeID == linkTo
}

// linkChecker is checkLinkValues for handlers that save fields outside the
// service's own writes (an era version's fields).
type linkChecker interface {
	checkLinkValues(ctx context.Context, e *Entity, oldData, newData map[string]any) error
}

// checkFieldKinds checks a category's link and rule fields before they are
// saved: a link names a category of this campaign, a rule names one of its
// homebrew rules. Settings for another type are dropped so a field turned
// back into text carries nothing stale.
func (s *entityService) checkFieldKinds(ctx context.Context, campaignID string, fields []FieldDefinition) error {
	for i := range fields {
		f := &fields[i]
		switch f.Type {
		case FieldTypeLink:
			f.RuleID = 0
			if f.LinkTo == 0 {
				return apperror.NewBadRequest(fmt.Sprintf("say which category the %s field links to", f.Label))
			}
			et, err := s.types.FindByID(ctx, f.LinkTo)
			if err != nil || et == nil || et.CampaignID != campaignID {
				return apperror.NewBadRequest(fmt.Sprintf("the %s field links to a category this campaign doesn't have", f.Label))
			}
		case FieldTypeRule:
			f.LinkTo = 0
			if f.RuleID == 0 {
				return apperror.NewBadRequest(fmt.Sprintf("say which rule works out the %s field", f.Label))
			}
			if s.fieldWords != nil && !s.fieldWords.RuleExists(ctx, campaignID, f.RuleID) {
				return apperror.NewBadRequest(fmt.Sprintf("the %s field follows a rule this campaign doesn't have", f.Label))
			}
		default:
			f.LinkTo, f.RuleID = 0, 0
		}
	}
	return nil
}

// checkLinkValues refuses a link field value that is not a page of the
// field's category in this campaign. Only values that changed are checked,
// so a link whose page was later removed never blocks saving other fields.
func (s *entityService) checkLinkValues(ctx context.Context, e *Entity, oldData, newData map[string]any) error {
	if e == nil {
		return nil
	}
	et, err := s.types.FindByID(ctx, e.EntityTypeID)
	if err != nil || et == nil {
		return nil
	}
	defs := append([]FieldDefinition{}, et.Fields...)
	if e.FieldOverrides != nil {
		defs = append(defs, e.FieldOverrides.Added...)
	}
	for _, d := range defs {
		if d.Type != FieldTypeLink {
			continue
		}
		id := strings.TrimSpace(fieldText(newData[d.Key]))
		if id == "" || id == strings.TrimSpace(fieldText(oldData[d.Key])) {
			continue
		}
		target, err := s.entities.FindByID(ctx, id)
		if err != nil || target == nil || target.CampaignID != e.CampaignID || target.ID == e.ID ||
			!linkTypeOK(target.EntityTypeID, d.LinkTo, func(tid int) *EntityType {
				et, _ := s.types.FindByID(ctx, tid)
				return et
			}) {
			return apperror.NewBadRequest(fmt.Sprintf("%s must link to a page of the right category", d.Label))
		}
	}
	return nil
}

// GetLocalWordsAPI returns the local words a viewer sees on one page, for
// widgets that show words outside the fields box, such as a shop's coins.
// GET /campaigns/:id/entities/:eid/local-words
func (h *Handler) GetLocalWordsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entity, err := h.service.GetByID(c.Request().Context(), c.Param("eid"))
	if err != nil {
		return err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}
	userID := auth.GetUserID(c)
	access, err := h.service.CheckEntityAccess(c.Request().Context(), entity.ID, int(cc.VisibilityRole()), userID)
	if err != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}
	words := map[string]string{}
	if h.fieldWords != nil {
		if w := h.fieldWords.WordsFor(c.Request().Context(), cc.Campaign.ID, entity.ID, int(cc.VisibilityRole()), userID); w != nil {
			words = w
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"words": words})
}

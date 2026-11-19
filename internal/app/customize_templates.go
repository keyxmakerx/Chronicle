package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// customizeSectionTemplates is the draft key for the campaign's starting
// text (content templates).
const customizeSectionTemplates = "templates"

// maxCustomizeTemplates bounds the templates one Save touches.
const maxCustomizeTemplates = 100

var (
	templateExistingKey = regexp.MustCompile(`^[0-9]{1,9}$`)
	templateNewKey      = regexp.MustCompile(`^new-[a-z0-9-]{1,30}$`)
)

// templatesStore is the slice of the content template service the section needs.
type templatesStore interface {
	Create(ctx context.Context, campaignID string, in entities.CreateContentTemplateInput) (*entities.ContentTemplate, error)
	GetByID(ctx context.Context, id int) (*entities.ContentTemplate, error)
	Update(ctx context.Context, id int, in entities.UpdateContentTemplateInput) (*entities.ContentTemplate, error)
	Delete(ctx context.Context, id int) error
}

// customizeTemplateItem is one template's change. An existing template's item
// holds only what differs from what is saved; a new one holds all of it.
type customizeTemplateItem struct {
	New          bool    `json:"new,omitempty"`
	Delete       bool    `json:"delete,omitempty"`
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	Icon         *string `json:"icon,omitempty"`
	ContentJSON  *string `json:"content_json,omitempty"`
	ContentHTML  *string `json:"content_html,omitempty"`
	EntityTypeID int     `json:"entity_type_id,omitempty"`
}

type templatesSectionValue struct {
	Items   map[string]customizeTemplateItem `json:"items"`
	Changes json.RawMessage                  `json:"_changes,omitempty"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// newTemplatesSection builds the "templates" Customize section.
func newTemplatesSection(store templatesStore) campaigns.CustomizeSection {
	return campaigns.CustomizeSection{
		Key: customizeSectionTemplates,
		Apply: func(ctx context.Context, cc *campaigns.CampaignContext, _ string, raw json.RawMessage) (json.RawMessage, error) {
			if cc.MemberRole < campaigns.RoleOwner {
				return nil, apperror.NewForbidden("only campaign owners can change starting text")
			}
			return applyTemplatesSection(ctx, cc.Campaign.ID, store, raw)
		},
	}
}

// applyTemplatesSection checks every item first (a template that is another
// campaign's or built in is refused, as a direct edit refuses it), then
// applies them in key order and returns what is still unapplied.
func applyTemplatesSection(ctx context.Context, campaignID string, store templatesStore, raw json.RawMessage) (json.RawMessage, error) {
	var sec templatesSectionValue
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, apperror.NewBadRequest("the starting text in the draft is not valid")
	}
	if len(sec.Items) == 0 {
		return nil, nil
	}
	if len(sec.Items) > maxCustomizeTemplates {
		return nil, apperror.NewBadRequest("too many starting texts changed at once")
	}

	keys := make([]string, 0, len(sec.Items))
	for k, it := range sec.Items {
		switch {
		case templateNewKey.MatchString(k):
			if !it.New || it.Delete {
				return nil, apperror.NewBadRequest("the starting text in the draft is not valid")
			}
		case templateExistingKey.MatchString(k):
			if it.New {
				return nil, apperror.NewBadRequest("the starting text in the draft is not valid")
			}
			id := atoiKey(k)
			t, err := store.GetByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if t.CampaignID == nil || *t.CampaignID != campaignID || t.IsGlobal {
				return nil, apperror.NewNotFound("content template not found")
			}
		default:
			return nil, apperror.NewBadRequest("the starting text in the draft is not valid")
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	remaining := map[string]customizeTemplateItem{}
	var firstErr error
	for _, k := range keys {
		it := sec.Items[k]
		if firstErr != nil {
			remaining[k] = it
			continue
		}
		var err error
		switch {
		case it.New:
			_, err = store.Create(ctx, campaignID, entities.CreateContentTemplateInput{
				EntityTypeID: it.EntityTypeID, Name: str(it.Name), Description: str(it.Description),
				ContentJSON: str(it.ContentJSON), ContentHTML: str(it.ContentHTML), Icon: str(it.Icon),
			})
		case it.Delete:
			err = store.Delete(ctx, atoiKey(k))
		default:
			_, err = store.Update(ctx, atoiKey(k), templateUpdate(it))
		}
		if err != nil {
			firstErr = err
			remaining[k] = it
		}
	}

	if len(remaining) == 0 {
		return nil, firstErr
	}
	if len(remaining) == len(keys) {
		return nil, firstErr
	}
	out, mErr := json.Marshal(templatesSectionValue{Items: remaining, Changes: sec.Changes})
	if mErr != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marshaling the remaining starting text: %w", mErr))
	}
	return out, firstErr
}

func atoiKey(k string) int {
	n := 0
	for _, r := range k {
		n = n*10 + int(r-'0')
	}
	return n
}

func templateUpdate(it customizeTemplateItem) entities.UpdateContentTemplateInput {
	in := entities.UpdateContentTemplateInput{}
	in.Name = sent(it.Name)
	in.Description = sent(it.Description)
	in.Icon = sent(it.Icon)
	in.ContentJSON = sent(it.ContentJSON)
	in.ContentHTML = sent(it.ContentHTML)
	return in
}

// sent is a field the draft holds a value for; one it does not hold stays
// absent, so the stored value is kept.
func sent(p *string) patch.Field[string] {
	if p == nil {
		return patch.Absent[string]()
	}
	return patch.Of(*p)
}

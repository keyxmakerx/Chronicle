package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
)

// Quests and notice boards sit after the house rules and pick-list entries.
func init() {
	addAIKinds(75, func(d *aiDeps) []records.Kind {
		if d.Quests == nil || d.Boards == nil {
			return nil
		}
		api := aiQuestsAdapter{quests: d.Quests, boards: d.Boards, types: d.Entities}
		return []records.Kind{
			records.QuestKind{Svc: api, Entities: d.Entities},
			records.NoticeBoardKind{Svc: api, Entities: d.Entities},
		}
	})
}

// aiQuestsAdapter implements records.QuestsAPI over the quests plugin's
// services. The import routes are owner-gated, so the operator acts as the
// campaign's DM; the services still scope every id to the campaign and the
// home it names.
type aiQuestsAdapter struct {
	quests quests.QuestService
	boards quests.BoardService
	types  interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
}

var _ records.QuestsAPI = aiQuestsAdapter{}

func questViewer(a records.Actor) quests.Viewer {
	return quests.Viewer{UserID: a.UserID, MemberRole: permissions.RoleOwner, VisibilityRole: a.Role, IsDM: true}
}

func questHome(h records.BoardHome) quests.Home {
	if h.CategoryID != 0 {
		return quests.TypeHome(h.CategoryID)
	}
	return quests.PageHome(h.PageID)
}

func (q aiQuestsAdapter) Quests(ctx context.Context, campaignID string, a records.Actor) ([]records.QuestRef, error) {
	list, err := q.quests.List(ctx, campaignID, questViewer(a))
	if err != nil {
		return nil, err
	}
	out := make([]records.QuestRef, 0, len(list))
	for _, e := range list {
		ref := records.QuestRef{PageID: e.EntityID, PageName: e.Name, Status: e.Status,
			Title: e.Notice.Title, Kicker: e.Notice.Kicker, Blurb: e.Notice.Blurb, Reward: e.Notice.Reward, Due: e.Notice.Due, Body: e.Notice.Body}
		for _, s := range e.Steps {
			ref.Steps = append(ref.Steps, records.QuestStep{Text: s.Text, Done: s.Done, Shown: s.Shown})
		}
		out = append(out, ref)
	}
	return out, nil
}

// WriteQuest reads the sheet for its version and current notice, so a block
// that names only the reward keeps the rest of the notice. A step whose text
// matches a stored one keeps that step's id.
func (q aiQuestsAdapter) WriteQuest(ctx context.Context, campaignID, pageID string, a records.Actor, in records.QuestInput) error {
	v := questViewer(a)
	got, err := q.quests.Get(ctx, campaignID, pageID, v)
	if err != nil {
		return err
	}
	cur, ok := got.(*quests.DMQuestView)
	if !ok {
		return apperror.NewForbidden("only the DM can change this quest")
	}
	p := quests.QuestPatch{Version: patch.Of(cur.Version), Status: setIfGiven(in.Status)}
	if in.Title != nil || in.Kicker != nil || in.Blurb != nil || in.Reward != nil || in.Due != nil || in.Body != nil {
		n := cur.Notice
		for _, f := range []struct {
			src *string
			dst *string
		}{{in.Title, &n.Title}, {in.Kicker, &n.Kicker}, {in.Blurb, &n.Blurb}, {in.Reward, &n.Reward}, {in.Due, &n.Due}} {
			if f.src != nil {
				*f.dst = *f.src
			}
		}
		if in.Body != nil {
			n.Body = *in.Body
		}
		p.Notice = patch.Of(n)
	}
	if in.Steps != nil {
		ids := map[string]string{}
		for _, s := range cur.Steps {
			ids[s.Text] = s.ID
		}
		steps := make([]quests.Step, 0, len(*in.Steps))
		for _, s := range *in.Steps {
			steps = append(steps, quests.Step{ID: ids[s.Text], Text: s.Text, Done: s.Done, Shown: s.Shown})
			delete(ids, s.Text)
		}
		p.Steps = patch.Of(steps)
	}
	_, err = q.quests.Put(ctx, campaignID, pageID, v, p)
	return err
}

func (q aiQuestsAdapter) Categories(ctx context.Context, campaignID string) ([]records.FolderCategory, error) {
	types, err := q.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]records.FolderCategory, 0, len(types))
	for _, t := range types {
		out = append(out, records.FolderCategory{ID: t.ID, Name: t.Name, Plural: t.NamePlural, Slug: t.Slug})
	}
	return out, nil
}

func (q aiQuestsAdapter) Boards(ctx context.Context, campaignID string, a records.Actor) ([]records.BoardRef, error) {
	v := questViewer(a)
	homes, err := q.boards.Homes(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	cats, err := q.Categories(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	// A notice's title can differ from its page's name, and the kind names
	// quests by page, so the names come from the sheets.
	sheets, err := q.Quests(ctx, campaignID, a)
	if err != nil {
		return nil, err
	}
	pageName := map[string]string{}
	for _, s := range sheets {
		pageName[s.PageID] = s.PageName
	}
	catName := map[int]string{}
	for _, c := range cats {
		catName[c.ID] = c.Name
	}
	var out []records.BoardRef
	for _, hv := range homes {
		var h records.BoardHome
		name := hv.Name
		if hv.Kind == "category" {
			id, err := strconv.Atoi(hv.ID)
			if err != nil {
				continue
			}
			h, name = records.BoardHome{CategoryID: id}, catName[id]
		} else {
			h = records.BoardHome{PageID: hv.ID}
		}
		view, err := q.boards.View(ctx, campaignID, questHome(h), v)
		if err != nil {
			return nil, err
		}
		for _, b := range view.Boards {
			ref := records.BoardRef{ID: b.ID, Name: b.Name, Who: b.Who, Fill: b.Fill, Home: h, HomeName: name}
			for _, it := range b.Items {
				qn := pageName[it.QuestID]
				if qn == "" {
					qn = it.Title // no saved sheet: the title is still the page name
				}
				ref.Items = append(ref.Items, records.BoardItemRef{ID: it.ID, Kind: it.Kind, QuestID: it.QuestID,
					QuestName: qn, Text: it.Text, ByDM: it.ByDM, Auto: it.Auto})
			}
			out = append(out, ref)
		}
	}
	return out, nil
}

func (q aiQuestsAdapter) CreateBoard(ctx context.Context, campaignID string, h records.BoardHome, a records.Actor, name, who string) (string, error) {
	b, err := q.boards.CreateBoard(ctx, campaignID, questHome(h), questViewer(a), name, who)
	if err != nil {
		return "", err
	}
	return b.ID, nil
}

func (q aiQuestsAdapter) UpdateBoard(ctx context.Context, campaignID string, h records.BoardHome, boardID string, a records.Actor, in records.BoardInput) error {
	p := quests.BoardPatch{Name: setIfGiven(in.Name), Who: setIfGiven(in.Who)}
	if in.Fill != nil {
		p.Fill = patch.Of(*in.Fill)
	}
	_, err := q.boards.PatchBoard(ctx, campaignID, questHome(h), boardID, questViewer(a), p)
	return err
}

func (q aiQuestsAdapter) DeleteBoard(ctx context.Context, campaignID string, h records.BoardHome, boardID string, a records.Actor) error {
	return q.boards.DeleteBoard(ctx, campaignID, questHome(h), boardID, questViewer(a))
}

// AddItem lays new pieces out four to a row after what is already pinned, so
// an import never stacks them on one spot.
func (q aiQuestsAdapter) AddItem(ctx context.Context, campaignID string, h records.BoardHome, boardID string, a records.Actor, it records.BoardItemRef) error {
	v := questViewer(a)
	n := 0
	if view, err := q.boards.View(ctx, campaignID, questHome(h), v); err == nil {
		for _, b := range view.Boards {
			if b.ID == boardID {
				n = len(b.Items)
			}
		}
	}
	in := quests.ItemInput{Kind: it.Kind, X: 3 + float64(n%4)*24.5, Y: 5 + float64((n/4)%3)*31}
	switch it.Kind {
	case quests.KindNotice:
		in.RefID = it.QuestID
	case quests.KindNote:
		in.Text, in.W = it.Text, 14
	default:
		return apperror.NewBadRequest(fmt.Sprintf("the AI tool cannot pin a %s", it.Kind))
	}
	_, err := q.boards.CreateItem(ctx, campaignID, questHome(h), boardID, v, in)
	return err
}

func (q aiQuestsAdapter) RemoveItem(ctx context.Context, campaignID string, h records.BoardHome, boardID, itemID string, a records.Actor) error {
	return q.boards.DeleteItem(ctx, campaignID, questHome(h), boardID, itemID, questViewer(a))
}

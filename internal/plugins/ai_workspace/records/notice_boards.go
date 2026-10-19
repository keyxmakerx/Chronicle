package records

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// NoticeBoardKind makes, changes and removes the notice boards on a place
// page or a category's page, matched by `name` within that page. It writes
// only the DM's own pieces (quest notices and notes); what players pinned is
// left alone.
type NoticeBoardKind struct {
	Svc      QuestsAPI
	Entities EntityLookup
}

func (NoticeBoardKind) Name() string  { return "notice-board" }
func (NoticeBoardKind) Label() string { return "Notice board" }
func (NoticeBoardKind) Doc() string {
	return "A notice board, matched by `name` on its page. Keys: `page` (the place page it hangs on, such as a tavern) or `category` (a category whose page shows it, such as Quests), one of them; `who` (who may pin to it: dm, scribes or everyone; dm when left out); `fills_with` (quest statuses it shows by itself: open, taken, done, failed, or `nothing` to stop; new and changed quests then come and go on their own); `quests` (quest page names you pin by hand); `notes` (short notes you pin, up to 400 characters each); `rename_to`. On update `quests` and `notes` replace the ones you pinned before; players' own notes and pins are never touched. Removing a board removes everything on it.\n\n```\n---\nkind: notice-board\ncategory: Quests\nname: Open work\nwho: everyone\nfills_with: [open, taken]\nnotes:\n  - Ask Mara at the bar before you sign anything.\n---\n```\n\n```\n---\nkind: notice-board\npage: The Rusty Anchor\nname: Back wall\nquests:\n  - The Missing Goat\n---\n```"
}

// boardPlan is one board record, read and resolved.
type boardPlan struct {
	home     BoardHome
	homeName string
	board    *BoardRef
	in       BoardInput
	quests   []string // page ids; nil when the record does not set them
	questsOn bool
	notes    []string
	notesOn  bool
}

func (k NoticeBoardKind) resolve(ctx context.Context, campaignID string, a Actor, r Record) (boardPlan, error) {
	var p boardPlan
	if r.Name == "" {
		return p, badRequestf("a notice board needs `name:`")
	}
	page, cat := r.Str("page"), r.Str("category")
	switch {
	case page != "" && cat != "":
		return p, badRequestf("a board hangs on a `page` or a `category`, not both")
	case page != "":
		e, _ := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(page))
		if e == nil || e.CampaignID != campaignID {
			return p, badRequestf("no page called %s", quote(page))
		}
		p.home, p.homeName = BoardHome{PageID: e.ID}, e.Name
	case cat != "":
		cats, err := k.Svc.Categories(ctx, campaignID)
		if err != nil {
			return p, err
		}
		var found []FolderCategory
		for _, c := range cats {
			if sameName(c.Name, cat) || sameName(c.Plural, cat) || sameName(c.Slug, cat) {
				found = append(found, c)
			}
		}
		if len(found) != 1 {
			return p, badRequestf("no single category called %s", quote(cat))
		}
		p.home, p.homeName = BoardHome{CategoryID: found[0].ID}, found[0].Name
	default:
		return p, badRequestf("say where the board hangs: `page:` or `category:`")
	}
	boards, err := k.Svc.Boards(ctx, campaignID, a)
	if err != nil {
		return p, err
	}
	for i := range boards {
		if boards[i].Home == p.home && sameName(boards[i].Name, r.Name) {
			p.board = &boards[i]
		}
	}
	if r.Has("who") {
		w, ok := boardWho[strings.ToLower(r.Str("who"))]
		if !ok {
			return p, badRequestf("who: %q is not dm, scribes or everyone", r.Str("who"))
		}
		p.in.Who = &w
	}
	if r.Has("fills_with") {
		fill, err := fillsWith(r)
		if err != nil {
			return p, err
		}
		p.in.Fill = &fill
	}
	if s := r.Str("rename_to"); s != "" {
		p.in.Name = &s
	}
	if r.Has("quests") {
		p.questsOn = true
		for _, it := range r.List("quests") {
			name := strings.TrimSpace(fmt.Sprint(it))
			e, _ := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
			if e == nil || e.CampaignID != campaignID {
				return p, badRequestf("no quest page called %s", quote(name))
			}
			p.quests = append(p.quests, e.ID)
		}
	}
	if r.Has("notes") {
		p.notesOn = true
		for _, it := range r.List("notes") {
			if t := strings.TrimSpace(fmt.Sprint(it)); t != "" {
				p.notes = append(p.notes, t)
			}
		}
	}
	return p, nil
}

// fillsWith reads a list or a comma-separated line of status words.
func fillsWith(r Record) ([]string, error) {
	var words []string
	if line, ok := r.Fields["fills_with"].(string); ok {
		words = strings.Split(line, ",")
	} else {
		for _, it := range r.List("fills_with") {
			words = append(words, fmt.Sprint(it))
		}
	}
	out := []string{}
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" || strings.EqualFold(w, "nothing") || strings.EqualFold(w, "none") {
			continue
		}
		st, ok := statusOf(w)
		if !ok {
			return nil, badRequestf("fills_with: %q is not open, taken, done or failed", w)
		}
		out = append(out, st)
	}
	return out, nil
}

func (k NoticeBoardKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can change notice boards"}
	}
	p, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	where := " on " + quote(p.homeName)
	switch r.Action {
	case ActionCreate:
		if p.board != nil {
			return Plan{Error: "a board called " + quote(r.Name) + where + " already exists; use action: update"}
		}
		return Plan{Summary: "a new board" + where + boardParts(p)}
	case ActionUpdate:
		if p.board == nil {
			return Plan{Error: "no board called " + quote(r.Name) + where + " to change"}
		}
		if parts := boardParts(p); parts != "" {
			return Plan{Summary: "changes it" + parts}
		}
		return Plan{Error: "nothing to change"}
	default:
		if p.board == nil {
			return Plan{Error: "no board called " + quote(r.Name) + where + " to remove"}
		}
		return Plan{Summary: fmt.Sprintf("removes the board and the %d things pinned to it", len(p.board.Items))}
	}
}

func boardParts(p boardPlan) string {
	var parts []string
	if p.in.Fill != nil {
		if len(*p.in.Fill) == 0 {
			parts = append(parts, "stops filling itself")
		} else {
			var w []string
			for _, st := range *p.in.Fill {
				w = append(w, questWord[st])
			}
			parts = append(parts, "fills itself with "+strings.Join(w, ", ")+" quests")
		}
	}
	if p.in.Who != nil {
		parts = append(parts, whoWord[*p.in.Who]+" may pin")
	}
	if p.in.Name != nil {
		parts = append(parts, "renamed "+quote(*p.in.Name))
	}
	if p.questsOn {
		parts = append(parts, strconv.Itoa(len(p.quests))+" quests pinned by hand")
	}
	if p.notesOn {
		parts = append(parts, strconv.Itoa(len(p.notes))+" notes")
	}
	if len(parts) == 0 {
		return ""
	}
	return ": " + strings.Join(parts, ", ")
}

func (k NoticeBoardKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if pl := k.Plan(ctx, campaignID, a, r); pl.Error != "" {
		return apperror.NewBadRequest(pl.Error)
	}
	p, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		return k.Svc.DeleteBoard(ctx, campaignID, p.home, p.board.ID, a)
	}
	var have []BoardItemRef
	id := ""
	if r.Action == ActionCreate {
		who := "dm"
		if p.in.Who != nil {
			who = *p.in.Who
		}
		if id, err = k.Svc.CreateBoard(ctx, campaignID, p.home, a, r.Name, who); err != nil {
			return err
		}
		p.in.Who = nil
	} else {
		id, have = p.board.ID, p.board.Items
	}
	if p.in.Name != nil || p.in.Who != nil || p.in.Fill != nil {
		if err := k.Svc.UpdateBoard(ctx, campaignID, p.home, id, a, p.in); err != nil {
			return err
		}
	}
	if p.questsOn {
		want := map[string]bool{}
		for _, q := range p.quests {
			want[q] = true
		}
		for _, it := range have {
			// The board already shows a quest it fills itself with.
			if it.Kind == "notice" && it.Auto {
				delete(want, it.QuestID)
			}
		}
		for _, it := range have {
			if it.Kind == "notice" && !it.Auto {
				if !want[it.QuestID] {
					if err := k.Svc.RemoveItem(ctx, campaignID, p.home, id, it.ID, a); err != nil {
						return err
					}
				}
				delete(want, it.QuestID)
			}
		}
		for _, q := range p.quests {
			if want[q] {
				delete(want, q)
				if err := k.Svc.AddItem(ctx, campaignID, p.home, id, a, BoardItemRef{Kind: "notice", QuestID: q}); err != nil {
					return err
				}
			}
		}
	}
	if p.notesOn {
		want := map[string]int{}
		for _, n := range p.notes {
			want[n]++
		}
		for _, it := range have {
			if it.Kind == "note" && it.ByDM {
				if want[it.Text] > 0 {
					want[it.Text]--
					continue
				}
				if err := k.Svc.RemoveItem(ctx, campaignID, p.home, id, it.ID, a); err != nil {
					return err
				}
			}
		}
		for _, n := range p.notes {
			if want[n] > 0 {
				want[n]--
				if err := k.Svc.AddItem(ctx, campaignID, p.home, id, a, BoardItemRef{Kind: "note", Text: n}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (k NoticeBoardKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	boards, err := k.Svc.Boards(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, bd := range boards {
		where := "page"
		if bd.Home.CategoryID != 0 {
			where = "category"
		}
		b.WriteString("---\nkind: notice-board\naction: update\n" + where + ": " + yamlStr(bd.HomeName) + "\nname: " + yamlStr(bd.Name) + "\nwho: " + whoWord[bd.Who] + "\n")
		if len(bd.Fill) > 0 {
			var w []string
			for _, st := range bd.Fill {
				w = append(w, questWord[st])
			}
			b.WriteString("fills_with: [" + strings.Join(w, ", ") + "]\n")
		}
		var quests, notes []string
		players := 0
		for _, it := range bd.Items {
			switch {
			case it.Kind == "notice" && !it.Auto:
				quests = append(quests, it.QuestName)
			case it.Kind == "note" && it.ByDM:
				notes = append(notes, it.Text)
			case !it.ByDM:
				players++
			}
		}
		for _, l := range []struct {
			key  string
			vals []string
		}{{"quests", quests}, {"notes", notes}} {
			if len(l.vals) > 0 {
				b.WriteString(l.key + ":\n")
				for _, v := range l.vals {
					b.WriteString("  - " + yamlStr(v) + "\n")
				}
			}
		}
		b.WriteString("---\n")
		if players > 0 {
			b.WriteString(fmt.Sprintf("<!-- players pinned %d things here; they are theirs and stay as they are -->\n", players))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

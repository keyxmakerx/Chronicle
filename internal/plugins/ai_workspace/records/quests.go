package records

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// QuestsAPI is the quests plugin as the quest and notice-board kinds need it,
// adapted in app wiring so this package does not import the plugin. Every
// call goes through the plugin's own services, with the operator as a DM, so
// their checks (limits, campaign scoping, who may see what) still apply.
type QuestsAPI interface {
	// Quests lists every saved quest sheet, by page.
	Quests(ctx context.Context, campaignID string, a Actor) ([]QuestRef, error)
	// WriteQuest changes the given parts of a page's sheet, making the sheet
	// when the page has none.
	WriteQuest(ctx context.Context, campaignID, pageID string, a Actor, in QuestInput) error
	Categories(ctx context.Context, campaignID string) ([]FolderCategory, error)
	// Boards lists every notice board in the campaign with its items.
	Boards(ctx context.Context, campaignID string, a Actor) ([]BoardRef, error)
	CreateBoard(ctx context.Context, campaignID string, h BoardHome, a Actor, name, who string) (string, error)
	UpdateBoard(ctx context.Context, campaignID string, h BoardHome, boardID string, a Actor, in BoardInput) error
	DeleteBoard(ctx context.Context, campaignID string, h BoardHome, boardID string, a Actor) error
	// AddItem pins a quest notice (QuestID) or a note (Text).
	AddItem(ctx context.Context, campaignID string, h BoardHome, boardID string, a Actor, it BoardItemRef) error
	RemoveItem(ctx context.Context, campaignID string, h BoardHome, boardID, itemID string, a Actor) error
}

// QuestStep is one step of a quest's ledger.
type QuestStep struct {
	Text        string
	Done, Shown bool
}

// QuestRef is a saved quest sheet. Status is the stored word.
type QuestRef struct {
	PageID, PageName                          string
	Status, Title, Kicker, Blurb, Reward, Due string
	Body                                      []string
	Steps                                     []QuestStep
}

// QuestInput is what a quest block changes; nil keeps a part as it is.
type QuestInput struct {
	Status, Title, Kicker, Blurb, Reward, Due *string
	Body                                      *[]string
	Steps                                     *[]QuestStep
}

// BoardHome is where a board hangs: a page, or a category's page.
type BoardHome struct {
	PageID     string
	CategoryID int
}

// BoardRef is a stored board. HomeName is the page or category name.
type BoardRef struct {
	ID, Name, Who, HomeName string
	Fill                    []string
	Home                    BoardHome
	Items                   []BoardItemRef
}

// BoardItemRef is one notice or note on a board. Other pins (pages, maps,
// string) are listed only so a count is honest; the kind never writes them.
type BoardItemRef struct {
	ID, Kind, QuestID, QuestName, Text string
	ByDM, Auto                         bool
}

// BoardInput is what a board block changes; nil keeps a part as it is.
type BoardInput struct {
	Name, Who *string
	Fill      *[]string
}

// questWords maps what an AI may write onto the stored statuses, and back.
var (
	questStatus = map[string]string{
		"open": "not_started", "not started": "not_started", "not_started": "not_started", "available": "not_started",
		"taken": "active", "active": "active", "in progress": "active",
		"done": "done", "complete": "done", "completed": "done",
		"failed": "failed",
	}
	questWord  = map[string]string{"not_started": "open", "active": "taken", "done": "done", "failed": "failed"}
	boardWho   = map[string]string{"dm": "dm", "only me": "dm", "scribe": "scribe", "scribes": "scribe", "all": "all", "everyone": "all"}
	whoWord    = map[string]string{"dm": "dm", "scribe": "scribes", "all": "everyone"}
	stepLineRe = regexp.MustCompile(`^\[( |x|X)\]\s*(.+)$`)
)

func statusOf(word string) (string, bool) {
	s, ok := questStatus[strings.ToLower(strings.TrimSpace(word))]
	return s, ok
}

// QuestKind writes a quest page's sheet: the notice players read, its
// status and its steps. The page itself is an ordinary page block.
type QuestKind struct {
	Svc      QuestsAPI
	Entities EntityLookup
}

func (QuestKind) Name() string  { return "quest" }
func (QuestKind) Label() string { return "Quest" }
func (QuestKind) Doc() string {
	return "A quest on a quest page, matched by `name` (the page's name; the page must exist or be created in the same paste). Keys: `status` (open, taken, done or failed), `title` (the notice's heading; the page name when left out), `kicker` (the small line above it, such as Help wanted), `blurb` (one or two lines on the notice), `reward` (the line players see), `due` (free text such as \"Before the new moon\"), `steps` (a list; `[x] ` marks a step done, and ` (shown)` at the end lets players see it; steps are hidden from players otherwise). The body is the notice's text, one paragraph per blank-line block. On update only the keys you write change; `steps` replaces every step. A quest is part of its page: to remove one, remove the page. Notice boards that fill themselves pick quests up by status.\n\n```\n---\nkind: quest\nname: The Missing Goat\nstatus: open\nkicker: Help wanted\nreward: 20 gp and a wheel of cheese\nsteps:\n  - Ask the farmer (shown)\n  - Follow the tracks into the wood\n---\nOld Bessa's prize goat went missing three nights ago.\n```\n\n```\n---\nkind: quest\naction: update\nname: The Missing Goat\nstatus: taken\n---\n```"
}

func (k QuestKind) input(r Record) (QuestInput, error) {
	var in QuestInput
	if r.Has("status") {
		st, ok := statusOf(r.Str("status"))
		if !ok {
			return in, badRequestf("status: %q is not open, taken, done or failed", r.Str("status"))
		}
		in.Status = &st
	}
	for _, f := range []struct {
		key string
		dst **string
	}{{"title", &in.Title}, {"kicker", &in.Kicker}, {"blurb", &in.Blurb}, {"reward", &in.Reward}, {"due", &in.Due}} {
		if r.Has(f.key) {
			v := r.Str(f.key)
			*f.dst = &v
		}
	}
	if r.Has("steps") {
		steps := []QuestStep{}
		for _, it := range r.List("steps") {
			st, err := questStep(it)
			if err != nil {
				return in, err
			}
			steps = append(steps, st)
		}
		in.Steps = &steps
	}
	if b := strings.TrimSpace(r.Body); b != "" {
		var paras []string
		for _, p := range regexp.MustCompile(`\n\s*\n`).Split(b, -1) {
			if p = strings.Join(strings.Fields(p), " "); p != "" {
				paras = append(paras, p)
			}
		}
		in.Body = &paras
	}
	return in, nil
}

// questStep reads one step: "[x] text (shown)", or a map with text, done and
// shown.
func questStep(it any) (QuestStep, error) {
	if m, ok := it.(map[string]any); ok {
		st := QuestStep{Text: strings.TrimSpace(fmt.Sprint(m["text"]))}
		st.Done, _ = m["done"].(bool)
		st.Shown, _ = m["shown"].(bool)
		if st.Text == "" || m["text"] == nil {
			return st, badRequestf("a step needs text")
		}
		return st, nil
	}
	s := strings.TrimSpace(fmt.Sprint(it))
	var st QuestStep
	if m := stepLineRe.FindStringSubmatch(s); m != nil {
		st.Done, s = m[1] != " ", strings.TrimSpace(m[2])
	}
	if t, ok := strings.CutSuffix(s, "(shown)"); ok {
		st.Shown, s = true, strings.TrimSpace(t)
	}
	if s == "" {
		return st, badRequestf("a step needs text")
	}
	st.Text = s
	return st, nil
}

// target finds the page and its saved sheet. The page may be one this paste
// makes, which exists only by commit.
func (k QuestKind) target(ctx context.Context, campaignID string, a Actor, r Record) (pageID string, sheet *QuestRef, err error) {
	if r.Name == "" {
		return "", nil, badRequestf("a quest needs `name:`, the name of its page")
	}
	e, _ := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(r.Name))
	if e == nil || e.CampaignID != campaignID {
		if r.Made[entities.Slugify(r.Name)] {
			return "", nil, nil
		}
		return "", nil, badRequestf("no page called %s; add a page block for it first", quote(r.Name))
	}
	list, err := k.Svc.Quests(ctx, campaignID, a)
	if err != nil {
		return "", nil, err
	}
	for i := range list {
		if list[i].PageID == e.ID {
			return e.ID, &list[i], nil
		}
	}
	return e.ID, nil, nil
}

func (k QuestKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can change quests"}
	}
	if r.Action == ActionDelete {
		return Plan{Error: "a quest is part of its page; remove the page with a page block, or set status: failed"}
	}
	in, err := k.input(r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	_, sheet, err := k.target(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if r.Action == ActionCreate && sheet != nil {
		return Plan{Error: quote(r.Name) + " already has a quest; use action: update"}
	}
	if r.Action == ActionUpdate && sheet == nil {
		return Plan{Error: "no quest on " + quote(r.Name) + " to change; use action: create"}
	}
	var parts []string
	if in.Status != nil {
		parts = append(parts, "status "+questWord[*in.Status])
	}
	if in.Title != nil || in.Kicker != nil || in.Blurb != nil || in.Reward != nil || in.Due != nil || in.Body != nil {
		parts = append(parts, "the notice")
	}
	if in.Steps != nil {
		parts = append(parts, fmt.Sprintf("%d steps", len(*in.Steps)))
	}
	if len(parts) == 0 {
		if r.Action == ActionUpdate {
			return Plan{Error: "nothing to change"}
		}
		return Plan{Summary: "a new quest, open, with the page name as its title"}
	}
	return Plan{Summary: "sets " + strings.Join(parts, ", ")}
}

func (k QuestKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	pageID, _, err := k.target(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if pageID == "" {
		e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(r.Name))
		if err != nil || e == nil || e.CampaignID != campaignID {
			return apperror.NewBadRequest("the page " + quote(r.Name) + " was not made")
		}
		pageID = e.ID
	}
	in, _ := k.input(r)
	return k.Svc.WriteQuest(ctx, campaignID, pageID, a, in)
}

func (k QuestKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	list, err := k.Svc.Quests(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, q := range list {
		b.WriteString("---\nkind: quest\naction: update\nname: " + yamlStr(q.PageName) + "\nstatus: " + questWord[q.Status] + "\n")
		if q.Title != "" && q.Title != q.PageName {
			b.WriteString("title: " + yamlStr(q.Title) + "\n")
		}
		for _, f := range [][2]string{{"kicker", q.Kicker}, {"blurb", q.Blurb}, {"reward", q.Reward}, {"due", q.Due}} {
			if f[1] != "" {
				b.WriteString(f[0] + ": " + yamlStr(f[1]) + "\n")
			}
		}
		if len(q.Steps) > 0 {
			b.WriteString("steps:\n")
			for _, s := range q.Steps {
				line := s.Text
				if s.Done {
					line = "[x] " + line
				}
				if s.Shown {
					line += " (shown)"
				}
				b.WriteString("  - " + yamlStr(line) + "\n")
			}
		}
		b.WriteString("---\n")
		if len(q.Body) > 0 {
			b.WriteString(strings.Join(q.Body, "\n\n") + "\n")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// yamlStr quotes a value only when YAML would misread it bare.
func yamlStr(s string) string {
	if s == "" || strings.ContainsAny(s, ":#[]{}&*!|>'\"%@`,") || strings.TrimSpace(s) != s {
		return quote(s)
	}
	return s
}

package quests

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// AIQuests is the quests' part of the AI engine (aiplug): the `quests`
// lookup, every quest with its status and the notice board it is pinned on.
// It reads through the two services with a Viewer built from the asker, so
// each reader gets what the Quests screens give them: the Director team the
// whole list, anyone else (Safe privacy, a player's key) only the notices a
// board they may open shows, with the services' own rules for hidden items
// and pages they cannot open.
type AIQuests struct {
	quests QuestService
	boards BoardService
}

// NewAIQuests builds the lookup over the quests plugin's services.
func NewAIQuests(q QuestService, b BoardService) *AIQuests { return &AIQuests{quests: q, boards: b} }

var _ aiplug.Lookup = (*AIQuests)(nil)

// maxAIQuestHomes bounds the board reads one answer makes.
const maxAIQuestHomes = 60

// LookupInfo describes `what: quests` and its tool. It is not owner-only: a
// player's key gets the notices on boards a player can see.
func (*AIQuests) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "quests", Aliases: []string{"quest", "notice-boards"}, Keys: "none",
		Brief: "every quest with its status and the notice board it is on (the Director team); anyone else gets the notices on boards they can see",
		Tool: aiplug.Tool{
			Name: "quests", Title: "Quests", Did: "Looked at the quests",
			Desc:       "Every quest in the campaign with its status (not started, active, done or failed) and the notice board it is pinned on, if any.",
			PlayerDesc: "The quest notices on the boards you can see, each with its status.",
		},
	}
}

// questWords is the plain word for a stored status.
var questWords = map[string]string{StatusNotStarted: "not started", StatusActive: "active", StatusDone: "done", StatusFailed: "failed"}

func aiViewer(a aiplug.Actor) Viewer {
	if a.CanAuthorDmOnly() {
		return Viewer{UserID: a.UserID, MemberRole: permissions.RoleOwner, VisibilityRole: a.Role, IsDM: true}
	}
	return Viewer{UserID: a.UserID, MemberRole: permissions.RolePlayer, VisibilityRole: a.Role}
}

// Answer lists the quests the Actor may see.
func (l *AIQuests) Answer(ctx context.Context, campaignID string, a aiplug.Actor, _ aiplug.Record) (string, string, error) {
	chip := "Quests"
	v := aiViewer(a)
	homes, err := l.boards.Homes(ctx, campaignID, v)
	if err != nil {
		return chip, "", err
	}
	// Each quest's boards, and the notices a non-Director may read.
	onBoard := map[string][]string{}
	title := map[string]ItemView{}
	for i, h := range homes {
		if i >= maxAIQuestHomes {
			break
		}
		home := PageHome(h.ID)
		where := h.Name
		if h.Kind == "category" {
			id, _ := strconv.Atoi(h.ID)
			home, where = TypeHome(id), "a category home"
		}
		bv, err := l.boards.View(ctx, campaignID, home, v)
		if err != nil {
			return chip, "", err
		}
		for _, b := range bv.Boards {
			for _, it := range b.Items {
				if it.Kind == KindNotice && it.QuestID != "" {
					onBoard[it.QuestID] = append(onBoard[it.QuestID], fmt.Sprintf("%s (%s)", aiplug.OneLine(b.Name, 60), aiplug.OneLine(where, 60)))
					title[it.QuestID] = it
				}
			}
		}
	}

	var lines []string
	if v.IsDM {
		list, err := l.quests.List(ctx, campaignID, v)
		if err != nil {
			return chip, "", err
		}
		for _, q := range list {
			lines = append(lines, questLine(q.Name, q.Status, onBoard[q.EntityID]))
		}
	} else {
		for id, it := range title {
			lines = append(lines, questLine(aiplug.OneLine(it.Title, 120), it.Status, onBoard[id]))
		}
		sort.Strings(lines)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Quests (%d)\n\n", len(lines))
	if len(lines) == 0 {
		b.WriteString("None yet.\n")
	}
	aiplug.WriteLines(&b, lines)
	return fmt.Sprintf("Quests · %d", len(lines)), b.String(), nil
}

func questLine(name, status string, boards []string) string {
	line := "- " + aiplug.OneLine(name, 120)
	if w := questWords[status]; w != "" {
		line += ": " + w
	}
	if len(boards) == 0 {
		return line + "; not on a board"
	}
	return line + "; on " + strings.Join(boards, ", ")
}

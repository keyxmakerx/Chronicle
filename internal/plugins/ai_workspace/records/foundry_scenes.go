package records

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FoundryScenesAPI is the scenes the campaign's Foundry world last reported
// (the syncapi plugin's scene store).
type FoundryScenesAPI interface {
	// FoundryScenes returns nil when Foundry has never reported.
	FoundryScenes(ctx context.Context, campaignID string) (*FoundryScenes, error)
}

// FoundryScenes is one report: the scenes by name, whether the world had
// more than one report holds, and when it came.
type FoundryScenes struct {
	Scenes     []FoundrySceneRef
	Truncated  bool
	ReportedAt time.Time
}

// FoundrySceneRef is one Foundry scene.
type FoundrySceneRef struct {
	Name, Folder string
	Active, Nav  bool
}

// scenes answers `what: scenes`: which maps already exist as Foundry
// scenes, optionally only those whose name or folder holds match. Director
// only, since scene names can give away what is coming.
func (run *lookupRun) scenes(match string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Foundry scenes"}
	if run.l.Scenes == nil {
		return ans, badRequestf("Foundry scenes can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("Foundry scenes are left out in Safe privacy; choose Permitted to include them")
	}
	rep, err := run.l.Scenes.FoundryScenes(run.ctx, run.campaignID)
	if err != nil {
		return ans, err
	}
	var b strings.Builder
	if rep == nil {
		b.WriteString("## Foundry scenes\n\nFoundry hasn't sent its scenes yet. They arrive when the campaign's Foundry world is open with the Chronicle Sync module connected.\n")
		ans.Text = b.String()
		return ans, nil
	}
	needle := strings.ToLower(strings.TrimSpace(match))
	var lines []string
	for _, s := range rep.Scenes {
		if needle != "" && !strings.Contains(strings.ToLower(s.Name), needle) && !strings.Contains(strings.ToLower(s.Folder), needle) {
			continue
		}
		line := "- " + s.Name
		var notes []string
		if s.Folder != "" {
			notes = append(notes, "in "+s.Folder)
		}
		if s.Active {
			notes = append(notes, "the scene players are on")
		}
		if len(notes) > 0 {
			line += " (" + strings.Join(notes, ", ") + ")"
		}
		lines = append(lines, line)
	}
	if needle != "" {
		ans.Chip = fmt.Sprintf("Foundry scenes · %q · %d", oneLine(match, 40), len(lines))
		fmt.Fprintf(&b, "## Foundry scenes matching %q (%d of %d)\n\n", oneLine(match, 80), len(lines), len(rep.Scenes))
	} else {
		ans.Chip = fmt.Sprintf("Foundry scenes · %d", len(lines))
		fmt.Fprintf(&b, "## Foundry scenes (%d)\n\n", len(lines))
	}
	fmt.Fprintf(&b, "As Foundry last reported them, %s UTC. A scene listed here is a map that already exists in Foundry.\n\n", rep.ReportedAt.UTC().Format("2006-01-02 15:04"))
	if len(lines) == 0 {
		if needle != "" {
			b.WriteString("No scene matches, so Foundry has no map of that yet.\n")
		} else {
			b.WriteString("The Foundry world has no scenes.\n")
		}
	}
	writeLines(&b, lines)
	if rep.Truncated {
		b.WriteString("(The world has more scenes than Foundry sends at once; some are missing here.)\n")
	}
	ans.Text = b.String()
	return ans, nil
}

package encounters

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RunService owns the Fight tab's rules. Like the encounter service it is
// for the DM only: anyone below Owner visibility is refused before anything
// is read.
type RunService interface {
	// Current returns the fight that is on, or the Ready encounters that can
	// be started when none is.
	Current(ctx context.Context, campaignID string, v Viewer) (*RunView, error)
	// Start lays out a fight from an encounter. It refuses while a fight is on.
	Start(ctx context.Context, campaignID string, v Viewer, in RunStart) (*RunView, error)
	// Save replaces the fight; in.Version must be the stored version.
	Save(ctx context.Context, campaignID string, v Viewer, in RunSave) (*RunView, error)
	// End ends the fight and says what it left, as the fight listener
	// tells it; ending no fight reports nothing.
	End(ctx context.Context, campaignID string, v Viewer) (*EndReport, error)
	// Show shows one row of the running scene to its players.
	Show(ctx context.Context, campaignID string, v Viewer, in RunShowInput) (*RunView, error)
	// Shown lists what scenes showed since, as this viewer may see it: the
	// DM team sees every row, a player the rows for everyone or for them.
	// Unlike every other method it answers any member.
	Shown(ctx context.Context, campaignID string, v Viewer, since time.Time) ([]ShownItem, error)
}

// RunShowInput names the scene row to show.
type RunShowInput struct {
	ShowID string `json:"showId"`
}

// MaxShownList bounds one read of the shown log.
const MaxShownList = 200

type runService struct {
	repo   RunRepository
	enc    Service
	pages  PageDirectory
	listen FightListener
	scenes SceneDirectory
	shown  ShownRepository
	pub    ShowPublisher
	now    func() time.Time
	newID  func() string
}

// SetShowing wires what running scenes need: the scene directory, the shown
// log and the publisher. Until it is called a scene starts but every Show is
// refused. It is set once while the app is wired, before any request.
func (s *runService) SetShowing(d SceneDirectory, shown ShownRepository, pub ShowPublisher) {
	s.scenes, s.shown, s.pub = d, shown, pub
}

// SetFightListener sets who hears about fights; nil for nobody. It is set
// once while the app is wired, before any request.
func (s *runService) SetFightListener(l FightListener) { s.listen = l }

// NewRunService builds the run service over the encounter service, which
// decides which encounters the viewer may start.
func NewRunService(repo RunRepository, enc Service, pages PageDirectory) RunService {
	return &runService{repo: repo, enc: enc, pages: pages, now: time.Now, newID: uuid.NewString}
}

func errRunChanged() error {
	return apperror.NewConflict("this fight was changed elsewhere; reload it")
}

func decodeRun(r RunRow) (*Run, error) {
	var run Run
	if err := json.Unmarshal(r.Data, &run); err != nil {
		return nil, apperrorInternal(err)
	}
	return &run, nil
}

func (s *runService) Current(ctx context.Context, campaignID string, v Viewer) (*RunView, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	out := &RunView{Ready: []Summary{}, StatBlocks: []StatBlock{}}
	row, found, err := s.repo.GetRun(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if found {
		run, err := decodeRun(row)
		if err != nil {
			return nil, err
		}
		if err := s.unlinkGone(ctx, campaignID, run, v); err != nil {
			return nil, err
		}
		out.Run, out.Version = run, row.Version
		out.Leaves = s.leaves(ctx, campaignID, run, v)
		if run.Kind == KindScene && s.scenes != nil {
			if ps, err := s.scenes.Players(ctx, campaignID); err == nil {
				out.Players = ps
			}
		}
	} else {
		list, _, err := s.enc.Search(ctx, campaignID, "", MaxSearchLimit, v)
		if err != nil {
			return nil, err
		}
		// Search lists ready encounters first.
		for _, e := range list {
			if !e.Ready {
				break
			}
			out.Ready = append(out.Ready, e)
		}
	}
	// Best effort, as in the builder: without them a name folds out the
	// page's fields instead of the system's stat block.
	if pt, err := s.pages.PickerTypes(ctx, campaignID); err != nil {
		slog.Warn("encounters: stat blocks unavailable for the fight", slog.String("campaign_id", campaignID), slog.Any("error", err))
	} else if pt.StatBlocks != nil {
		out.StatBlocks = pt.StatBlocks
	}
	return out, nil
}

// unlinkGone clears a line's page when the page has gone or the viewer may
// no longer see it, the way an encounter row keeps its name.
func (s *runService) unlinkGone(ctx context.Context, campaignID string, run *Run, v Viewer) error {
	var ids []string
	for _, l := range run.Lines {
		if l.EntityID != "" {
			ids = append(ids, l.EntityID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := s.pages.Names(ctx, campaignID, ids, v)
	if err != nil {
		return apperrorInternal(err)
	}
	for i, l := range run.Lines {
		if _, ok := names[l.EntityID]; l.EntityID != "" && !ok {
			run.Lines[i].EntityID = ""
		}
	}
	return nil
}

func (s *runService) Start(ctx context.Context, campaignID string, v Viewer, in RunStart) (*RunView, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	// Get refuses an encounter on a page the viewer may not open.
	e, err := s.enc.Get(ctx, campaignID, in.EncounterID, v)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(newRun(e))
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ok, err := s.repo.CreateRun(ctx, campaignID, raw, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, apperror.NewConflict("a fight is already on; end it before starting another")
	}
	return s.Current(ctx, campaignID, v)
}

func (s *runService) Save(ctx context.Context, campaignID string, v Viewer, in RunSave) (*RunView, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	run, err := validateRun(in.Run)
	if err != nil {
		return nil, err
	}
	row, found, err := s.repo.GetRun(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found || in.Version != row.Version {
		return nil, errRunChanged()
	}
	stored, err := decodeRun(row)
	if err != nil {
		return nil, err
	}
	// What the fight is and which pages its lines are come from the stored
	// run, never from the save.
	pages := make(map[string]string, len(stored.Lines))
	for _, l := range stored.Lines {
		pages[l.ID] = l.EntityID
	}
	run.EncounterID, run.Name = stored.EncounterID, stored.Name
	run.Kind, run.Shows = stored.Kind, stored.Shows
	for i := range run.Lines {
		run.Lines[i].EntityID = pages[run.Lines[i].ID]
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ok, err := s.repo.UpdateRun(ctx, campaignID, raw, row.Version, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errRunChanged()
	}
	return s.Current(ctx, campaignID, v)
}

// End ends the fight, then tells the listener which creatures were down.
// The fight is over whatever the listener answers, so its failure is
// logged and reported as a line rather than undoing the end.
func (s *runService) End(ctx context.Context, campaignID string, v Viewer) (*EndReport, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	row, ended, err := s.repo.EndRun(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &EndReport{Lines: []string{}}
	if !ended {
		return out, nil
	}
	run, err := decodeRun(row)
	if err != nil {
		slog.Warn("encounters: an ended fight could not be read", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return out, nil
	}
	out.Name = run.Name
	if s.listen == nil || run.Kind == KindScene {
		return out, nil
	}
	rep, err := s.listen.FightEnded(context.WithoutCancel(ctx), campaignID, v, endedFight(run))
	if err != nil {
		slog.Error("encounters: the fight ended but its listener failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		out.Lines = append(out.Lines, "The fight is over, but what it left couldn’t be worked out. Check the tray before handing anything out.")
		return out, nil
	}
	if rep != nil {
		out.Lines, out.Link, out.LinkText, out.Note = rep.Lines, rep.Link, rep.LinkText, rep.Note
		if out.Lines == nil {
			out.Lines = []string{}
		}
	}
	return out, nil
}

// leaves asks the listener what each creature's page will leave, best
// effort: the fight shows without it.
func (s *runService) leaves(ctx context.Context, campaignID string, run *Run, v Viewer) map[string]string {
	if s.listen == nil {
		return nil
	}
	var ids []string
	for _, l := range run.Lines {
		if l.Kind == LineCreature && l.EntityID != "" {
			ids = append(ids, l.EntityID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	out, err := s.listen.Leaves(ctx, campaignID, v, ids)
	if err != nil {
		slog.Warn("encounters: what creatures leave is unavailable", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	return out
}

// Show shows one scene row. A page row needs a page every player it goes to
// may already open; a reveal row un-hides its NPC or creature page first. The
// item is logged before it is sent, and the row's time is saved last, so a
// lost race on the run never loses what players were shown.
func (s *runService) Show(ctx context.Context, campaignID string, v Viewer, in RunShowInput) (*RunView, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	if s.shown == nil || s.pub == nil {
		return nil, apperror.NewConflict("showing things to players isn’t available right now")
	}
	row, found, err := s.repo.GetRun(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, apperror.NewConflict("no scene is running; start one first")
	}
	run, err := decodeRun(row)
	if err != nil {
		return nil, err
	}
	if run.Kind != KindScene {
		return nil, apperror.NewConflict("a fight is running, not a scene")
	}
	at := -1
	for i, sh := range run.Shows {
		if sh.ID == in.ShowID {
			at = i
		}
	}
	if at < 0 {
		return nil, errNotFound("thing to show")
	}
	sh := run.Shows[at].Show
	item := ShownItem{ID: s.newID(), Kind: sh.Kind, Label: sh.Label, Text: sh.Text, To: sh.To, Scene: run.Name, ShownAt: s.now().UTC()}
	switch sh.Kind {
	case ShowPage, ShowReveal:
		if sh.EntityID == "" || s.scenes == nil {
			return nil, errInvalid("this row's page has gone; pick another in the builder")
		}
		names, err := s.pages.Names(ctx, campaignID, []string{sh.EntityID}, v)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		name, ok := names[sh.EntityID]
		if !ok {
			return nil, errInvalid("this row's page has gone; pick another in the builder")
		}
		if sh.Kind == ShowReveal {
			if name, err = s.scenes.Reveal(ctx, campaignID, sh.EntityID); err != nil {
				return nil, err
			}
		}
		can, err := s.scenes.PlayerCanView(ctx, campaignID, sh.EntityID, sh.To)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		if !can {
			return nil, errInvalid(name + " is hidden from the players. Show it to them on the page first, or make this a reveal row.")
		}
		item.EntityID = sh.EntityID
		if item.Label == "" {
			item.Label = name
		}
	}
	if err := s.shown.AddShown(ctx, campaignID, item, v.UserID); err != nil {
		return nil, apperrorInternal(err)
	}
	s.pub.Shown(campaignID, item)
	shownAt := item.ShownAt
	run.Shows[at].ShownAt = &shownAt
	if raw, err := json.Marshal(run); err == nil {
		if _, err := s.repo.UpdateRun(ctx, campaignID, raw, row.Version, v.UserID); err != nil {
			slog.Warn("encounters: a shown row's time wasn't saved", slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
	}
	return s.Current(ctx, campaignID, v)
}

func (s *runService) Shown(ctx context.Context, campaignID string, v Viewer, since time.Time) ([]ShownItem, error) {
	if s.shown == nil {
		return []ShownItem{}, nil
	}
	all, err := s.shown.ListShown(ctx, campaignID, since, MaxShownList)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if isDM(v) {
		return all, nil
	}
	out := make([]ShownItem, 0, len(all))
	for _, it := range all {
		if it.To == "" || it.To == v.UserID {
			out = append(out, it)
		}
	}
	return out, nil
}

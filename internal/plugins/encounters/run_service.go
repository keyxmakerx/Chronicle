package encounters

import (
	"context"
	"encoding/json"
	"log/slog"

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
	// End ends the fight.
	End(ctx context.Context, campaignID string, v Viewer) error
}

type runService struct {
	repo  RunRepository
	enc   Service
	pages PageDirectory
}

// NewRunService builds the run service over the encounter service, which
// decides which encounters the viewer may start.
func NewRunService(repo RunRepository, enc Service, pages PageDirectory) RunService {
	return &runService{repo: repo, enc: enc, pages: pages}
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

func (s *runService) End(ctx context.Context, campaignID string, v Viewer) error {
	if err := requireDM(v); err != nil {
		return err
	}
	if err := s.repo.DeleteRun(ctx, campaignID); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

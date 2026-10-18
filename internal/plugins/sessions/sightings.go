package sessions

import (
	"context"
	"sort"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Sighting states and sources. A stored row is "seen" or "not_seen" and came
// from the after-night check-in or was added by hand; "link" marks a
// session_entities link merged in at read time, never stored here.
const (
	SightingSeen    = "seen"
	SightingNotSeen = "not_seen"

	SightingSourceCheckin = "checkin"
	SightingSourceManual  = "manual"
	SightingSourceLink    = "link"
)

// Sighting is one night in a page's Last seen. Date is the night's
// occurrence date; it is empty only for a link on a repeating series, which
// names the series but no single night.
type Sighting struct {
	SessionID   string `json:"sessionId"`
	SessionName string `json:"sessionName"`
	Date        string `json:"date,omitempty"`
	Source      string `json:"source"`
	// Role is the session_entities role, set only when Source is "link".
	Role string `json:"role,omitempty"`
	// AddedBy is who recorded a stored row; only the DM is told.
	AddedBy string `json:"addedBy,omitempty"`
}

// Sightings is everything a viewer may know about when the party saw a page.
// Suggestions and NotSeen are filled only for a viewer who sees DM-only
// content: they are nights the page came up but nobody has said it was seen.
type Sightings struct {
	Seen        []Sighting `json:"seen"`
	Suggestions []Sighting `json:"suggestions,omitempty"`
	NotSeen     []Sighting `json:"notSeen,omitempty"`
}

// RecordSightingInput is one answer for one night and page.
type RecordSightingInput struct {
	SessionID string `json:"-"`
	Date      string `json:"date"`
	EntityID  string `json:"entityId"`
	State     string `json:"state"`
	Source    string `json:"source"`
}

// SightingService is the one place sightings are read and written, so the
// check-in and a page's Last seen share them.
type SightingService interface {
	// SightingsFor merges stored rows with the page's encountered/key links,
	// newest night first, and adds unanswered "mentioned" nights as
	// suggestions for a DM viewer. A viewer who cannot see the page gets
	// nothing.
	SightingsFor(ctx context.Context, campaignID, entityID string, v permissions.Viewer) (*Sightings, error)
	// RecordSighting writes one answer. DM only.
	RecordSighting(ctx context.Context, campaignID string, in RecordSightingInput, v permissions.Viewer) error
	// ClearSighting removes one answer, so the night is unanswered again. DM only.
	ClearSighting(ctx context.Context, campaignID, sessionID, date, entityID string, v permissions.Viewer) error
}

type sightingService struct {
	repo       SightingRepository
	sessions   SessionRepository
	entities   EntityCampaignChecker
	visibility EntityVisibilityFilter
}

// NewSightingService creates the sightings service. The entity checker and
// visibility filter are the same adapters the session service uses; with no
// visibility filter every read fails closed.
func NewSightingService(repo SightingRepository, sessions SessionRepository, ec EntityCampaignChecker, ev EntityVisibilityFilter) SightingService {
	return &sightingService{repo: repo, sessions: sessions, entities: ec, visibility: ev}
}

func seesDM(v permissions.Viewer) bool { return v.Role() >= permissions.RoleOwner }

func (s *sightingService) SightingsFor(ctx context.Context, campaignID, entityID string, v permissions.Viewer) (*Sightings, error) {
	out := &Sightings{Seen: []Sighting{}}
	if s.visibility == nil {
		return out, nil
	}
	ok, err := s.visibility.FilterViewableEntityIDs(ctx, campaignID, []string{entityID}, v.Role(), v.UserID())
	if err != nil {
		return nil, err
	}
	if !ok[entityID] {
		return out, nil
	}
	rows, err := s.repo.ListSightingsForEntity(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	links, err := s.repo.ListLinksForEntity(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	dm := seesDM(v)

	// A stored answer for a night outranks a link for the same night, and
	// once any night of a series is marked seen the series-wide link is
	// dropped so the series is not counted twice. A "not seen" night leaves
	// the series link alone: it says nothing about the other nights.
	answered := map[string]bool{}
	seenSession := map[string]bool{}
	for _, r := range rows {
		answered[r.SessionID+"|"+r.OccurrenceDate] = true
		if r.State == SightingSeen {
			seenSession[r.SessionID] = true
		}
		sg := Sighting{SessionID: r.SessionID, SessionName: r.SessionName, Date: r.OccurrenceDate, Source: r.Source}
		if dm {
			sg.AddedBy = r.AddedBy
		}
		switch {
		case r.State == SightingSeen:
			out.Seen = append(out.Seen, sg)
		case dm:
			out.NotSeen = append(out.NotSeen, sg)
		}
	}
	for _, l := range links {
		date := l.ScheduledDate
		if l.IsRecurring {
			date = ""
		}
		if answered[l.SessionID+"|"+date] || (date == "" && seenSession[l.SessionID]) {
			continue
		}
		sg := Sighting{SessionID: l.SessionID, SessionName: l.SessionName, Date: date, Source: SightingSourceLink, Role: l.Role}
		switch l.Role {
		case EntityRoleEncountered, EntityRoleKey:
			out.Seen = append(out.Seen, sg)
		case EntityRoleMentioned:
			if dm {
				out.Suggestions = append(out.Suggestions, sg)
			}
		}
	}
	newestFirst(out.Seen)
	newestFirst(out.Suggestions)
	newestFirst(out.NotSeen)
	return out, nil
}

// newestFirst orders by night, latest first; a series with no single night
// sorts last, then by name so the order is stable.
func newestFirst(list []Sighting) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Date != list[j].Date {
			return list[i].Date > list[j].Date
		}
		return list[i].SessionName < list[j].SessionName
	})
}

func (s *sightingService) RecordSighting(ctx context.Context, campaignID string, in RecordSightingInput, v permissions.Viewer) error {
	if !seesDM(v) {
		return apperror.NewForbidden("only the DM can record who the party saw")
	}
	switch in.State {
	case SightingSeen, SightingNotSeen:
	default:
		return apperror.NewBadRequest("state must be seen or not_seen")
	}
	switch in.Source {
	case "":
		in.Source = SightingSourceManual
	case SightingSourceCheckin, SightingSourceManual:
	default:
		return apperror.NewBadRequest("source must be checkin or manual")
	}
	if err := s.checkNight(ctx, campaignID, in.SessionID, in.Date, in.EntityID); err != nil {
		return err
	}
	return s.repo.UpsertSighting(ctx, SightingRow{
		SessionID: in.SessionID, OccurrenceDate: in.Date, EntityID: in.EntityID,
		State: in.State, Source: in.Source, AddedBy: v.UserID(),
	})
}

func (s *sightingService) ClearSighting(ctx context.Context, campaignID, sessionID, date, entityID string, v permissions.Viewer) error {
	if !seesDM(v) {
		return apperror.NewForbidden("only the DM can change who the party saw")
	}
	if err := s.checkNight(ctx, campaignID, sessionID, date, entityID); err != nil {
		return err
	}
	return s.repo.DeleteSighting(ctx, sessionID, date, entityID)
}

// checkNight confirms the session and page are this campaign's and that the
// date is one of the session's nights: its own date, or an occurrence of a
// repeating series.
func (s *sightingService) checkNight(ctx context.Context, campaignID, sessionID, date, entityID string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return apperror.NewBadRequest("date must be YYYY-MM-DD")
	}
	sess, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.CampaignID != campaignID {
		return apperror.NewNotFound("session not found")
	}
	if len(occurrenceDates(*sess, date, date)) == 0 {
		return apperror.NewBadRequest("that date is not one of this game night's dates")
	}
	if s.entities == nil {
		return apperror.NewNotFound("page not found")
	}
	belongs, err := s.entities.EntityBelongsToCampaign(ctx, entityID, campaignID)
	if err != nil {
		return err
	}
	if !belongs {
		return apperror.NewNotFound("page not found")
	}
	return nil
}

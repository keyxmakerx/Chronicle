package cutscenes

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Story is where a line the Director accepts joins the campaign's pages;
// implemented in internal/app over the entities service, so this plugin
// never reads or writes a page itself.
type Story interface {
	// Pages lists the pages the Director can add a line to.
	Pages(ctx context.Context, campaignID string) ([]PageRef, error)
	// AddLine appends the line to the page's words, marked as from the
	// named player.
	AddLine(ctx context.Context, campaignID, pageID, line, from string) error
}

// WithStory wires Add to the story; without it the panel can only turn
// answers down and go on.
func WithStory(st Story) Option { return func(s *service) { s.story = st } }

// cleanAnswer is a written answer as it is stored: control characters
// dropped, runs of space folded, trimmed, and refused when empty or longer
// than MaxAnswer.
func cleanAnswer(text, empty string) (string, error) {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "", apperror.NewBadRequest(empty)
	}
	if len([]rune(text)) > MaxAnswer {
		return "", apperror.NewBadRequest("keep it to 280 characters")
	}
	return text, nil
}

// viewerOf returns the showing when v is one of its viewers and it may
// still be answered: not stopped and inside its window. A Director who is
// not a viewer is not answering it, so they get the same 404 as anyone.
func (s *service) viewerOf(ctx context.Context, campaignID, showingID string, v Viewer) (*Showing, string, error) {
	if v.UserID == "" {
		return nil, "", notFound()
	}
	sh, err := s.repo.GetShowing(ctx, campaignID, showingID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return nil, "", notFound()
		}
		return nil, "", err
	}
	if !s.now().Before(sh.RewatchUntil) {
		return nil, "", notFound()
	}
	for _, vw := range sh.Viewers {
		if vw.UserID == v.UserID {
			s.name(ctx, campaignID, []*Showing{sh})
			for _, named := range sh.Viewers {
				if named.UserID == v.UserID {
					return sh, named.Name, nil
				}
			}
		}
	}
	return nil, "", notFound()
}

func (s *service) Answer(ctx context.Context, campaignID, showingID string, moment int, v Viewer, in AnswerInput) (*MyAnswer, error) {
	sh, name, err := s.viewerOf(ctx, campaignID, showingID, v)
	if err != nil {
		return nil, err
	}
	if sh.StoppedAt != nil {
		return nil, apperror.NewConflict("Your Director has ended this scene.")
	}
	if moment < 0 || moment >= len(sh.Scene.Moments) {
		return nil, apperror.NewBadRequest("this scene has no such moment")
	}
	if sh.IsReleased(moment) {
		return nil, apperror.NewConflict("The scene has moved on from this moment.")
	}
	m := sh.Scene.Moments[moment]
	offered := false
	for _, w := range m.Ways {
		if w.Kind == in.Way {
			offered = true
		}
	}
	if !offered {
		return nil, apperror.NewBadRequest("this moment can't be answered that way")
	}
	text := ""
	if in.Way == WayWrite {
		if text, err = cleanAnswer(in.Text, "write a few words first"); err != nil {
			return nil, err
		}
	}
	if m.To == ToOne {
		// One person answers this moment: whoever answers first.
		answers, err := s.repo.ListAnswers(ctx, campaignID, sh.ID, "")
		if err != nil {
			return nil, err
		}
		for _, a := range answers {
			if a.Moment == moment && a.UserID != v.UserID {
				return nil, apperror.NewConflict("Someone else is already answering this one.")
			}
		}
	}
	a := Answer{ID: uuid.NewString(), ShowingID: sh.ID, Moment: moment, UserID: v.UserID, Name: name, Way: in.Way, Text: text,
		Status: AnswerWaiting, CreatedAt: s.now().UTC().Truncate(time.Second)}
	if err := s.repo.SaveAnswer(ctx, campaignID, a); err != nil {
		return nil, err
	}
	if s.pub != nil {
		s.pub.Answered(campaignID, playerShowing(*sh), a)
	}
	return &MyAnswer{Moment: moment, Way: a.Way, Text: a.Text}, nil
}

func (s *service) LiveMoments(ctx context.Context, campaignID, sceneID string, v Viewer) ([]LiveMoment, error) {
	live, err := s.LiveFor(ctx, campaignID, sceneID, v)
	if err != nil {
		return nil, err
	}
	var pages []PageRef
	out := []LiveMoment{}
	for _, sh := range live {
		if len(sh.Scene.Moments) == 0 {
			continue
		}
		lm := LiveMoment{Showing: sh, Current: sh.Current()}
		if lm.Current >= 0 {
			lm.Moment = sh.Scene.Moments[lm.Current]
			all, err := s.repo.ListAnswers(ctx, campaignID, sh.ID, "")
			if err != nil {
				return nil, err
			}
			names := map[string]string{}
			for _, vw := range sh.Viewers {
				names[vw.UserID] = vw.Name
			}
			for _, a := range all {
				if a.Moment != lm.Current {
					continue
				}
				a.Name = names[a.UserID]
				if a.Name == "" {
					a.Name = "someone who left"
				}
				lm.Answers = append(lm.Answers, a)
			}
			if pages == nil && s.story != nil {
				// The page list only feeds the picker; without it the
				// Director types a page's name.
				if pages, err = s.story.Pages(ctx, campaignID); err != nil {
					slog.Warn("cutscenes: listing pages", slog.String("campaign_id", campaignID), slog.Any("error", err))
					pages = []PageRef{}
				}
			}
			lm.Pages = pages
		}
		out = append(out, lm)
	}
	return out, nil
}

// pageFor finds the page the Director named, by id or by its exact name.
func (s *service) pageFor(ctx context.Context, campaignID, page string) (PageRef, error) {
	page = strings.TrimSpace(page)
	if page == "" {
		return PageRef{}, apperror.NewBadRequest("pick the page the line goes on")
	}
	pages, err := s.story.Pages(ctx, campaignID)
	if err != nil {
		return PageRef{}, err
	}
	for _, p := range pages {
		if p.ID == page {
			return p, nil
		}
	}
	var found []PageRef
	for _, p := range pages {
		if strings.EqualFold(strings.TrimSpace(p.Name), page) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return PageRef{}, apperror.NewBadRequest("no page is called that; pick one from the list")
	}
	return PageRef{}, apperror.NewBadRequest("more than one page has that name; pick one from the list")
}

func (s *service) Resolve(ctx context.Context, campaignID, showingID, answerID string, v Viewer, in ResolveInput) (*Answer, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	sh, err := s.repo.GetShowing(ctx, campaignID, showingID)
	if err != nil {
		return nil, err
	}
	a, err := s.repo.GetAnswer(ctx, campaignID, sh.ID, answerID)
	if err != nil {
		return nil, err
	}
	if a.Status != AnswerWaiting {
		return nil, apperror.NewConflict("This answer was already decided.")
	}
	s.name(ctx, campaignID, []*Showing{sh})
	for _, vw := range sh.Viewers {
		if vw.UserID == a.UserID {
			a.Name = vw.Name
		}
	}
	status, line, pageID := "", "", ""
	switch in.Action {
	case ResolveDecline:
		status = AnswerDeclined
	case ResolveAdd, ResolveChange:
		if s.story == nil {
			return nil, apperror.NewConflict("Adding to the story isn't available right now.")
		}
		status, line = AnswerAdded, a.Text
		if in.Action == ResolveChange {
			status = AnswerChanged
			if line, err = cleanAnswer(in.Line, "write the line as it should read"); err != nil {
				return nil, err
			}
		} else if line == "" {
			// Left to the Director: there is nothing to add until they
			// write it.
			return nil, apperror.NewBadRequest("they left this to you: write the line with Change it")
		}
		page, err := s.pageFor(ctx, campaignID, in.Page)
		if err != nil {
			return nil, err
		}
		pageID = page.ID
		if err := s.story.AddLine(ctx, campaignID, pageID, line, a.Name); err != nil {
			return nil, err
		}
	default:
		return nil, apperror.NewBadRequest("choose Add to the story, Change it or Turn down")
	}
	now := s.now().UTC().Truncate(time.Second)
	if err := s.repo.ResolveAnswer(ctx, campaignID, a.ID, status, line, pageID, now); err != nil {
		return nil, err
	}
	a.Status, a.Line, a.PageID, a.ResolvedAt = status, line, pageID, &now
	return a, nil
}

func (s *service) Go(ctx context.Context, campaignID, showingID string, moment int, v Viewer) (*Showing, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	sh, err := s.repo.GetShowing(ctx, campaignID, showingID)
	if err != nil {
		return nil, err
	}
	if moment < 0 || moment >= len(sh.Scene.Moments) {
		return nil, apperror.NewBadRequest("this scene has no such moment")
	}
	if sh.IsReleased(moment) {
		return sh, nil
	}
	if err := s.repo.ReleaseMoment(ctx, campaignID, sh.ID, moment); err != nil {
		return nil, err
	}
	sh.Released |= 1 << uint(moment)
	answers, err := s.repo.ListAnswers(ctx, campaignID, sh.ID, "")
	if err != nil {
		return nil, err
	}
	s.name(ctx, campaignID, []*Showing{sh})
	names := map[string]string{}
	for _, vw := range sh.Viewers {
		names[vw.UserID] = vw.Name
	}
	lines := []ShownLine{}
	for _, a := range answers {
		if a.Moment == moment && a.InStory() {
			lines = append(lines, ShownLine{Line: a.Line, From: names[a.UserID]})
		}
	}
	if s.pub != nil {
		s.pub.Go(campaignID, playerShowing(*sh), moment, lines)
	}
	return sh, nil
}

// myAnswers is a viewer's own answers to a showing, for a screen that opens
// mid-hold. A failed read loses only the half-done answer, not the scene.
func (s *service) myAnswers(ctx context.Context, campaignID string, sh *Showing, userID string) []MyAnswer {
	if len(sh.Scene.Moments) == 0 {
		return nil
	}
	list, err := s.repo.ListAnswers(ctx, campaignID, sh.ID, userID)
	if err != nil {
		slog.Warn("cutscenes: reading a viewer's answers", slog.String("showing_id", sh.ID), slog.Any("error", err))
		return nil
	}
	var out []MyAnswer
	for _, a := range list {
		out = append(out, MyAnswer{Moment: a.Moment, Way: a.Way, Text: a.Text})
	}
	return out
}

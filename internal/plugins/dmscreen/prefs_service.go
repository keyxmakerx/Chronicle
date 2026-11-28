package dmscreen

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PrefsRepository stores each person's screen preferences, one document per
// campaign, user and screen. It never decides who may read what: the service
// gates every call and only ever passes the caller's own user id.
type PrefsRepository interface {
	// Get returns the stored document, or found false when there is none.
	Get(ctx context.Context, campaignID, userID, screen string) (doc []byte, found bool, err error)
	// Update runs fn on the stored document (nil when there is none) while
	// holding the row, and stores what fn returns, so two saves from two
	// tabs cannot each drop the other's keys.
	Update(ctx context.Context, campaignID, userID, screen string, fn func(cur []byte) ([]byte, error)) error
}

// KitInfo is which style kit a person's screen wears and whether they may
// pick their own from the Style drawer. The pick itself is a member look
// pick (campaigns "Your look"), so the owner's "who can change" setting
// governs it exactly as it does on that page.
type KitInfo struct {
	// Campaign is the kit the campaign gives this screen, "material" for none.
	Campaign string `json:"campaign"`
	// Mine is the viewer's own pick, empty when they follow the campaign.
	Mine string `json:"mine,omitempty"`
	// CanPick is true when the viewer may keep their own pick.
	CanPick bool `json:"canPick"`
	// Part is the look part a pick is saved under (PUT .../my-look/:part).
	Part string `json:"part,omitempty"`
	// Owner is true for the campaign owner, whose choice is the campaign's
	// own and is made in Customize.
	Owner bool `json:"owner,omitempty"`
}

// KitSource tells the Style drawer about the kit. It is implemented in
// internal/app over the campaigns service.
type KitSource interface {
	ScreenKit(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string) (KitInfo, error)
}

// PrefsView is what GET and PUT answer.
type PrefsView struct {
	Screen string `json:"screen"`
	// Stored is false until the person's first save, so the screen knows to
	// keep what this browser already remembers as Default's starting point.
	Stored bool        `json:"stored"`
	Prefs  ScreenPrefs `json:"prefs"`
	Kit    *KitInfo    `json:"kit,omitempty"`
}

// PrefsService reads and writes the caller's own screen preferences.
type PrefsService interface {
	Get(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string) (*PrefsView, error)
	Update(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string, in UpdateScreenPrefsInput) (*PrefsView, error)
	// Allowed reports whether the viewer may keep preferences for screen.
	Allowed(cc *campaigns.CampaignContext, screen string) error
}

type prefsService struct {
	repo PrefsRepository // nil while the plugin's tables are unavailable
	kits KitSource       // nil leaves the drawer's kit row out
}

// NewPrefsService returns the service. A nil repo (the plugin degraded)
// answers every read with nothing stored and refuses saves, so the screen
// keeps its layouts in the browser.
func NewPrefsService(repo PrefsRepository, kits KitSource) PrefsService {
	return &prefsService{repo: repo, kits: kits}
}

// ScreenAllowed is who may keep preferences for a screen: the DM Screen's
// for those who can open it, the players' screen's for every other member.
// The routes gate on the same rule; the service asks again so a wiring
// mistake cannot open one screen's rows to the other's viewers.
func ScreenAllowed(cc *campaigns.CampaignContext, screen string) error {
	member := cc != nil && cc.Campaign != nil && cc.IsMember && cc.MemberRole >= campaigns.RolePlayer
	switch screen {
	case ScreenDM:
		if !member || !CanOpen(cc) {
			return apperror.NewForbidden("the DM Screen is for the people running this campaign")
		}
	case ScreenPlayer:
		if !member || CanOpen(cc) {
			return apperror.NewForbidden("the players' screen is for the campaign's players")
		}
	default:
		return apperror.NewNotFound("unknown screen")
	}
	return nil
}

// Allowed implements PrefsService.
func (s *prefsService) Allowed(cc *campaigns.CampaignContext, screen string) error {
	return ScreenAllowed(cc, screen)
}

// Get implements PrefsService.
func (s *prefsService) Get(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string) (*PrefsView, error) {
	if err := ScreenAllowed(cc, screen); err != nil {
		return nil, err
	}
	if userID == "" {
		return nil, apperror.NewForbidden("sign in to keep screen layouts")
	}
	view := &PrefsView{Screen: screen, Kit: s.kit(ctx, cc, userID, screen)}
	if s.repo == nil {
		return view, nil
	}
	doc, found, err := s.repo.Get(ctx, cc.Campaign.ID, userID, screen)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !found {
		return view, nil
	}
	view.Stored = true
	view.Prefs = decodePrefs(doc)
	return view, nil
}

// Update implements PrefsService. The row written is addressed by the
// caller's own user id, never one from input.
func (s *prefsService) Update(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string, in UpdateScreenPrefsInput) (*PrefsView, error) {
	if err := ScreenAllowed(cc, screen); err != nil {
		return nil, err
	}
	if userID == "" {
		return nil, apperror.NewForbidden("sign in to keep screen layouts")
	}
	if s.repo == nil {
		return nil, apperror.NewInternalMessage("screen layouts can't be saved right now", errors.New("dmscreen prefs store unavailable"))
	}
	var saved ScreenPrefs
	err := s.repo.Update(ctx, cc.Campaign.ID, userID, screen, func(cur []byte) ([]byte, error) {
		merged, err := mergePrefs(decodePrefs(cur), in)
		if err != nil {
			return nil, err
		}
		saved = merged
		return json.Marshal(merged)
	})
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) {
			return nil, ae
		}
		return nil, apperror.NewInternal(err)
	}
	return &PrefsView{Screen: screen, Stored: true, Prefs: saved, Kit: s.kit(ctx, cc, userID, screen)}, nil
}

// decodePrefs reads a stored document. One that no longer validates (a
// later build tightened a rule) reads as nothing stored rather than locking
// the person out of their screen.
func decodePrefs(doc []byte) ScreenPrefs {
	var p ScreenPrefs
	if len(doc) == 0 {
		return p
	}
	if err := json.Unmarshal(doc, &p); err != nil {
		slog.Warn("reading screen prefs", slog.Any("error", err))
		return ScreenPrefs{}
	}
	clean, err := cleanPrefs(p)
	if err != nil {
		slog.Warn("stored screen prefs no longer valid", slog.Any("error", err))
		return ScreenPrefs{}
	}
	return clean
}

// kit asks the kit source; a failure only leaves the drawer's kit row out.
func (s *prefsService) kit(ctx context.Context, cc *campaigns.CampaignContext, userID, screen string) *KitInfo {
	if s.kits == nil {
		return nil
	}
	k, err := s.kits.ScreenKit(ctx, cc, userID, screen)
	if err != nil {
		slog.Warn("reading screen kit", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return nil
	}
	return &k
}

package auth

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
)

// GetNotifyPrefs returns the person's notification choices, defaults filled in.
func (s *authService) GetNotifyPrefs(ctx context.Context, userID string) (notifyprefs.Prefs, error) {
	raw, err := s.repo.GetNotifyPrefs(ctx, userID)
	if err != nil {
		return notifyprefs.Default(), err
	}
	return notifyprefs.Parse(raw), nil
}

// UpdateNotifyPrefs merges a partial change into the person's stored choices
// and returns the result.
func (s *authService) UpdateNotifyPrefs(ctx context.Context, userID string, in notifyprefs.Update) (notifyprefs.Prefs, error) {
	cur, err := s.GetNotifyPrefs(ctx, userID)
	if err != nil {
		return cur, err
	}
	next, err := in.ApplyTo(cur)
	if err != nil {
		return cur, err
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return cur, apperror.NewInternal(err)
	}
	if err := s.repo.SetNotifyPrefs(ctx, userID, raw); err != nil {
		return cur, err
	}
	return next, nil
}

// AllowedRecipients returns the users from ids who still want this category
// on this channel, in the same order. If the choices can't be read, everyone
// is kept: missing a game-night invite is worse than one unwanted message.
func (s *authService) AllowedRecipients(ctx context.Context, ids []string, category string, ch notifyprefs.Channel) []string {
	stored, err := s.repo.ListNotifyPrefs(ctx, ids)
	if err != nil {
		slog.Warn("reading notification choices; sending to everyone", slog.Any("error", err))
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if notifyprefs.Parse(stored[id]).Allows(category, ch) {
			out = append(out, id)
		}
	}
	return out
}
